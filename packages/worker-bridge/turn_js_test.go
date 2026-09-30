//go:build js && wasm

package bridge

import (
	"net/http"
	"syscall/js"
	"testing"
	"time"
)

func serveForTest(t *testing.T, handler http.Handler) js.Value {
	t.Helper()
	ready := make(chan struct{})
	rt := js.ValueOf(map[string]any{"env": map[string]any{}, "binding": map[string]any{}})
	readyFn := js.FuncOf(func(js.Value, []js.Value) any {
		close(ready)
		return nil
	})
	rt.Set("ready", readyFn)
	js.Global().Set("context", rt)
	go Serve(handler)
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not call context.ready")
	}
	return rt.Get("binding")
}

func requestObject(id int, path string) js.Value {
	return js.ValueOf(map[string]any{
		"method":  "GET",
		"url":     "https://test.internal" + path,
		"headers": []any{},
		"body":    nil,
	})
}

func TestRunWokenOnAnotherRequestsEntryWaitsForItsOwnPump(t *testing.T) {
	gate := make(chan struct{})
	ran := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/wait", func(w http.ResponseWriter, r *http.Request) {
		<-gate
		if err := windowFrom(r.Context()).Run(func() { close(ran) }); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	mux.HandleFunc("/kick", func(w http.ResponseWriter, r *http.Request) {
		close(gate)
	})
	binding := serveForTest(t, mux)
	env := js.ValueOf(map[string]any{})

	binding.Call("handleRequest", requestObject(1, "/wait"), env, js.Undefined(), 1)
	time.Sleep(20 * time.Millisecond)
	binding.Call("handleRequest", requestObject(2, "/kick"), env, js.Undefined(), 2)

	select {
	case <-ran:
		t.Fatal("request 1's job ran on request 2's JS entry instead of waiting for request 1's pump")
	case <-time.After(100 * time.Millisecond):
	}

	binding.Call("pump", 1)
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("request 1's job did not run when request 1 pumped")
	}
}
