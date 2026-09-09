//go:build js && wasm

package workers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"syscall/js"

	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
)

var httpHandler http.Handler

func init() {
	binding := js.Global().Get("context").Get("binding")
	var handleRequestFn js.Func
	handleRequestFn = js.FuncOf(func(this js.Value, args []js.Value) any {
		reqObj := args[0]
		// The JS bootstrap forwards this request's env as a second arg for
		// resident binaries (loader/bootstrap.ts). Threading it per request
		// -- instead of relying on the module-global env from Go.run time --
		// lets a resident handler resolve request-scoped bindings, which a
		// long-lived instance must do or it reuses the instantiating
		// request's Fetcher and hits "Cannot perform I/O on behalf of a
		// different request" (S24). Absent (controllers' pokes pass only the
		// request), env stays undefined and dispatch leaves the context
		// alone.
		var env js.Value
		hasEnv := len(args) > 1 && !args[1].IsUndefined() && !args[1].IsNull()
		if hasEnv {
			env = args[1]
		}
		var executor js.Func
		executor = js.FuncOf(func(_ js.Value, promiseArgs []js.Value) any {
			defer executor.Release()
			resolve, reject := promiseArgs[0], promiseArgs[1]
			go func() {
				// This goroutine must not let the instance be considered
				// done until JS has actually finished delivering the
				// resolved/rejected value to whoever is awaiting this
				// promise (bootstrap.ts's `return binding.handleRequest(
				// request)`, in turn awaited by apiserver.ts's
				// `ep.fetch(...)`). resolve/reject.Invoke only *schedules*
				// that delivery as a microtask; it does not run it
				// synchronously, so returning right after Invoke races the
				// still-pending continuation -- observed live (in the old
				// per-request shape) as a JS-side "Cannot read properties
				// of undefined (reading 'exports')" crash, 100%
				// reproducible for a zero-I/O handler like GET /version.
				// yieldToEventLoop forces a macrotask boundary, which the
				// JS spec guarantees fully drains the microtask queue
				// first -- verified fixed live.
				respObj, err := dispatch(reqObj, env, hasEnv)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
					yieldToEventLoop()
					return
				}
				resolve.Invoke(respObj)
				yieldToEventLoop()
			}()
			return js.Undefined()
		})
		return js.Global().Get("Promise").New(executor)
	})
	binding.Set("handleRequest", handleRequestFn)

	// The pump-window pair the resident bootstrap brackets every dispatch
	// with. It is what lets background goroutines find a request whose
	// IoContext is still open when they need to issue outbound I/O --
	// see cloudflare.Window.
	binding.Set("openPumpWindow", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return cloudflare.OpenPumpWindow(args[0])
	}))
	binding.Set("closePumpWindow", js.FuncOf(func(_ js.Value, args []js.Value) any {
		cloudflare.ClosePumpWindow(args[0].Int())
		return js.Undefined()
	}))
}

// yieldToEventLoop blocks the calling goroutine until a fresh JS
// macrotask (setTimeout(..., 0), not a microtask/Promise callback) runs.
// The JS spec guarantees the entire microtask queue -- including however
// many .then() hops it takes to deliver a resolved/rejected value up
// through bootstrap.ts's await and apiserver.ts's await -- fully drains
// before any macrotask fires, so this is a reliable "I'm sure the caller
// has the value now" barrier regardless of the exact number of promise
// hops involved.
func yieldToEventLoop() {
	done := make(chan struct{})
	var cb js.Func
	cb = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer cb.Release()
		close(done)
		return nil
	})
	js.Global().Call("setTimeout", cb, 0)
	<-done
}

// dispatch converts a JS Request into an *http.Request, runs it through
// httpHandler, and converts the result into a JS Response. Fully
// buffered end to end (no ReadableStream/io.Pipe bridging): every
// pkg/apiserver handler either io.ReadAll's its request body or writes
// its response in one Encode+Write, and the one place a chunked response
// would matter (watch) is a stub that returns immediately -- real watch
// streaming happens over WebSocket in TypeScript, never through this Go
// WASM entrypoint.
func dispatch(reqObj js.Value, env js.Value, hasEnv bool) (js.Value, error) {
	if httpHandler == nil {
		return js.Value{}, fmt.Errorf("workers: Serve/ResidentService must be called before a request is dispatched")
	}

	req, err := requestFromJS(reqObj)
	if err != nil {
		return js.Value{}, fmt.Errorf("workers: decode request: %w", err)
	}
	if hasEnv {
		req = req.WithContext(cloudflare.WithEnv(req.Context(), env))
	}

	rec := &responseRecorder{header: make(http.Header), status: http.StatusOK}
	httpHandler.ServeHTTP(rec, req)
	return rec.toJSResponse(), nil
}

func requestFromJS(reqObj js.Value) (*http.Request, error) {
	reqURL, err := url.Parse(reqObj.Get("url").String())
	if err != nil {
		return nil, err
	}
	header := headerFromJS(reqObj.Get("headers"))
	body, err := readBody(reqObj)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	return &http.Request{
		Method:        reqObj.Get("method").String(),
		URL:           reqURL,
		Header:        header,
		Body:          nopCloser{bytes.NewReader(body)},
		ContentLength: int64(len(body)),
		Host:          header.Get("Host"),
		RemoteAddr:    header.Get("Cf-Connecting-Ip"),
	}, nil
}

// readBody reads a JS Request/Response body (a nullable ReadableStream)
// in one shot via arrayBuffer() rather than pulling a ReadableStream
// reader chunk by chunk -- see dispatch's doc comment for why every
// caller in this repo can tolerate that.
func readBody(reqObj js.Value) ([]byte, error) {
	if reqObj.Get("body").IsNull() {
		return nil, nil
	}
	buf, err := awaitPromise(reqObj.Call("arrayBuffer"))
	if err != nil {
		return nil, err
	}
	data := make([]byte, buf.Get("byteLength").Int())
	js.CopyBytesToGo(data, js.Global().Get("Uint8Array").New(buf))
	return data, nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// headerFromJS converts a JS Headers object into http.Header. Headers'
// own entries() iterator already comma-joins repeated header names per
// the Fetch spec, so splitting each value back out on "," reconstructs
// the original multi-value header.
func headerFromJS(headers js.Value) http.Header {
	entries := js.Global().Get("Array").Call("from", headers.Call("entries"))
	n := entries.Length()
	h := make(http.Header, n)
	for i := 0; i < n; i++ {
		entry := entries.Index(i)
		key, values := entry.Index(0).String(), entry.Index(1).String()
		for _, v := range strings.Split(values, ",") {
			h.Add(key, v)
		}
	}
	return h
}

func headerToJS(header http.Header) js.Value {
	h := js.Global().Get("Headers").New()
	for key, values := range header {
		for _, v := range values {
			h.Call("append", key, v)
		}
	}
	return h
}

// responseRecorder is a minimal buffered http.ResponseWriter -- see
// dispatch's doc comment for why buffering the whole response is safe
// here.
type responseRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *responseRecorder) Header() http.Header         { return w.header }
func (w *responseRecorder) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *responseRecorder) WriteHeader(status int)      { w.status = status }

// toJSResponse builds a JS Response. The four statuses the Fetch spec
// forbids a body on (https://fetch.spec.whatwg.org/#null-body-status)
// get Response(null, ...) even if something was written to them --
// matches the Fetch API's own contract, not a choice made here.
func (w *responseRecorder) toJSResponse() js.Value {
	respInit := js.Global().Get("Object").New()
	respInit.Set("status", w.status)
	respInit.Set("statusText", http.StatusText(w.status))
	respInit.Set("headers", headerToJS(w.header))

	switch w.status {
	case http.StatusSwitchingProtocols, http.StatusNoContent, http.StatusResetContent, http.StatusNotModified:
		return js.Global().Get("Response").New(js.Null(), respInit)
	}

	body := w.body.Bytes()
	jsBody := js.Global().Get("Uint8Array").New(len(body))
	js.CopyBytesToJS(jsBody, body)
	return js.Global().Get("Response").New(jsBody, respInit)
}

// awaitPromise blocks the calling goroutine until promise settles,
// returning its resolved value or converting a rejection into a Go
// error. Safe to call from any goroutine -- the then/catch callbacks run
// on the single JS thread and hand off through a channel.
func awaitPromise(promise js.Value) (js.Value, error) {
	resultCh := make(chan js.Value, 1)
	errCh := make(chan error, 1)
	var then, catch js.Func
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer then.Release()
		resultCh <- args[0]
		return js.Undefined()
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer catch.Release()
		errCh <- fmt.Errorf("js promise rejected: %s", args[0].Call("toString").String())
		return js.Undefined()
	})
	promise.Call("then", then).Call("catch", catch)
	select {
	case v := <-resultCh:
		return v, nil
	case err := <-errCh:
		return js.Value{}, err
	}
}

// ServeNonBlock registers handler to serve every dispatched request but
// does not block or signal readiness -- pair with Ready(). Both execution
// shapes (the resident apiserver's parked main() and ResidentService)
// use this; the old blocking Serve() went away with the per-request
// shape (S24).
func ServeNonBlock(handler http.Handler) {
	if handler == nil {
		handler = http.DefaultServeMux
	}
	httpHandler = handler
}

//go:wasmimport workers ready
func readyImport()

// Ready signals the JS bootstrap (bootstrap.ts's `workers: { ready: ...
// }` WASM import) that handleRequest registration (this file's init) has
// completed and the JS-side `binding` object it populated is now safe to
// call.
func Ready() {
	readyImport()
}
