//go:build js && wasm

package bridge

import (
	"context"
	"io"
	"net/http"
	"syscall/js"
	"testing"
	"time"
)

func TestFetchWithoutAWindowUsesTheRequestWhoseEventWokeIt(t *testing.T) {
	type outcome struct {
		body string
		err  error
	}
	wake := make(chan js.Value, 1)
	events := make(chan struct{}, 1)
	fetched := make(chan outcome, 1)
	release := make(chan struct{})
	writing := make(chan struct{})
	finishWrite := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/watch", func(w http.ResponseWriter, r *http.Request) {
		id := register(struct{}{}, windowFrom(r.Context()))
		defer unregister(id)
		go func() {
			<-events
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, "https://hooks.internal/crdconvert", nil)
			if err != nil {
				fetched <- outcome{err: err}
				return
			}
			resp, err := BindingTransport{Name: "HOOK"}.RoundTrip(req)
			if err != nil {
				fetched <- outcome{err: err}
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			fetched <- outcome{body: string(body), err: err}
		}()
		wake <- bound("test-store-event", id, func(int, []js.Value) { events <- struct{}{} })
		<-release
	})
	mux.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		close(writing)
		<-finishWrite
	})
	binding := serveForTest(t, mux)
	env := js.Global().Call("eval", "({HOOK: {fetch: async () => new Response('converted')}})")
	defer close(release)

	binding.Call("handleRequest", requestObject(1, "/watch"), env, js.Undefined(), 1)
	storeEvent := <-wake
	binding.Call("handleRequest", requestObject(2, "/write"), env, js.Undefined(), 2)
	<-writing

	storeEvent.Invoke()
	time.Sleep(20 * time.Millisecond)
	close(finishWrite)

	select {
	case got := <-fetched:
		if got.err != nil || got.body != "converted" {
			t.Fatalf("fetch issued from the watch's event: body=%q err=%v", got.body, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the fetch was handed to the newer request, which answered and was never pumped again")
	}
}

func TestHandlerFetchWithoutAWindowStaysOnItsOwnRequestWhenANewerOneIsOpen(t *testing.T) {
	type outcome struct {
		body string
		err  error
	}
	storing := make(chan struct{})
	converted := make(chan outcome, 1)
	arrived := make(chan struct{})
	finishNewer := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/create", func(w http.ResponseWriter, r *http.Request) {
		store, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://cluster.internal/kv", nil)
		if err != nil {
			converted <- outcome{err: err}
			return
		}
		close(storing)
		stored, err := BindingTransport{Name: "STORE"}.RoundTrip(store)
		if err != nil {
			converted <- outcome{err: err}
			return
		}
		stored.Body.Close()
		convert, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, "https://hooks.internal/crdconvert", nil)
		if err != nil {
			converted <- outcome{err: err}
			return
		}
		resp, err := BindingTransport{Name: "HOOK"}.RoundTrip(convert)
		if err != nil {
			converted <- outcome{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		converted <- outcome{body: string(body), err: err}
	})
	mux.HandleFunc("/discovery", func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-finishNewer
	})
	binding := serveForTest(t, mux)
	env := js.Global().Call("eval", `(() => {
		let answer;
		const answered = new Promise((resolve) => { answer = resolve; });
		return {
			STORE: {fetch: () => answered.then(() => new Response('stored'))},
			HOOK: {fetch: async () => new Response('converted')},
			answerStore: () => answer(),
		};
	})()`)

	binding.Call("handleRequest", requestObject(1, "/create"), env, js.Undefined(), 1)
	<-storing
	time.Sleep(20 * time.Millisecond)
	binding.Call("handleRequest", requestObject(2, "/discovery"), env, js.Undefined(), 2)
	<-arrived

	env.Call("answerStore")
	time.Sleep(20 * time.Millisecond)
	close(finishNewer)

	select {
	case got := <-converted:
		if got.err != nil || got.body != "converted" {
			t.Fatalf("fetch issued by the handler: body=%q err=%v", got.body, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the handler's fetch was handed to the newer request, which answered and was never pumped again")
	}
}
