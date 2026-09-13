//go:build js && wasm

// Package wasmhttp bridges a Go http.Handler to the Worker Loader bootstrap
// (worker/src/loader.ts). The bootstrap instantiates the Go program once per
// isolate, waits for context.ready(), then calls context.binding.handleRequest
// (request, env) for every dispatch. Bodies are fully buffered in both
// directions.
package wasmhttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"syscall/js"
)

type envKey struct{}

func Env(ctx context.Context) js.Value {
	if ctx != nil {
		if v, ok := ctx.Value(envKey{}).(js.Value); ok {
			return v
		}
	}
	return js.Global().Get("context").Get("env")
}

func Binding(ctx context.Context, name string) js.Value {
	return Env(ctx).Get(name)
}

// Getenv reads a string var from the env the program was instantiated with.
func Getenv(name string) string {
	v := js.Global().Get("context").Get("env").Get(name)
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

func Serve(handler http.Handler) {
	rt := js.Global().Get("context")
	binding := rt.Get("binding")
	binding.Set("handleRequest", js.FuncOf(func(_ js.Value, args []js.Value) any {
		reqObj := args[0]
		env := args[1]
		var executor js.Func
		executor = js.FuncOf(func(_ js.Value, p []js.Value) any {
			defer executor.Release()
			resolve, reject := p[0], p[1]
			go func() {
				if err := dispatch(handler, reqObj, env, func(v js.Value) { resolve.Invoke(v) }); err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}
				yieldToEventLoop()
			}()
			return js.Undefined()
		})
		return js.Global().Get("Promise").New(executor)
	}))
	rt.Call("ready")
	select {}
}

func yieldToEventLoop() {
	done := make(chan struct{})
	var cb js.Func
	cb = js.FuncOf(func(js.Value, []js.Value) any {
		defer cb.Release()
		close(done)
		return nil
	})
	js.Global().Call("setTimeout", cb, 0)
	<-done
}

// responseWriter buffers until the handler flushes; from the first Flush
// on, the response is delivered early with a ReadableStream body that the
// rest of the handler's writes are enqueued into. That is what lets an
// upstream watch handler stream from a resident instance.
type responseWriter struct {
	header     http.Header
	status     int
	buf        bytes.Buffer
	streaming  bool
	controller js.Value
	started    func(js.Value)
	cancel     context.CancelFunc
	closed     chan bool
}

func (r *responseWriter) Header() http.Header { return r.header }

// CloseNotify marks this writer as HTTP/1 for k8s.io/apiserver's
// ResponseWriterDelegator, which otherwise hides Flush from the watch
// handler. The channel fires when the client cancels the stream.
func (r *responseWriter) CloseNotify() <-chan bool { return r.closed }
func (r *responseWriter) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *responseWriter) Write(b []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	if !r.streaming {
		return r.buf.Write(b)
	}
	r.controller.Call("enqueue", toUint8Array(b))
	return len(b), nil
}

func (r *responseWriter) Flush() {
	r.WriteHeader(http.StatusOK)
	if r.streaming {
		return
	}
	src := js.Global().Get("Object").New()
	start := js.FuncOf(func(_ js.Value, args []js.Value) any {
		r.controller = args[0]
		return nil
	})
	defer start.Release()
	src.Set("start", start)
	var cancelFn js.Func
	cancelFn = js.FuncOf(func(js.Value, []js.Value) any {
		defer cancelFn.Release()
		select {
		case r.closed <- true:
		default:
		}
		r.cancel()
		return nil
	})
	src.Set("cancel", cancelFn)
	stream := js.Global().Get("ReadableStream").New(src)
	r.streaming = true
	if r.buf.Len() > 0 {
		r.controller.Call("enqueue", toUint8Array(r.buf.Bytes()))
		r.buf.Reset()
	}
	r.started(r.response(stream))
}

func (r *responseWriter) finish() {
	if r.streaming {
		func() {
			defer func() { recover() }()
			r.controller.Call("close")
		}()
		return
	}
	r.WriteHeader(http.StatusOK)
	r.started(r.response(toUint8Array(r.buf.Bytes())))
}

func (r *responseWriter) response(body js.Value) js.Value {
	out := js.Global().Get("Object").New()
	out.Set("status", r.status)
	out.Set("headers", headerToPairs(r.header))
	out.Set("body", body)
	return out
}

func toUint8Array(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

func dispatch(handler http.Handler, reqObj, env js.Value, started func(js.Value)) (err error) {
	u, err := url.Parse(reqObj.Get("url").String())
	if err != nil {
		return err
	}
	var body []byte
	if raw := reqObj.Get("body"); !raw.IsNull() && !raw.IsUndefined() {
		body = make([]byte, raw.Get("byteLength").Int())
		js.CopyBytesToGo(body, raw)
	}
	req := &http.Request{
		Method:        reqObj.Get("method").String(),
		URL:           u,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        headerFromPairs(reqObj.Get("headers")),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Host:          u.Host,
		RequestURI:    u.RequestURI(),
	}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), envKey{}, env))
	defer cancel()
	req = req.WithContext(ctx)
	rw := &responseWriter{header: http.Header{}, cancel: cancel, closed: make(chan bool, 1)}
	once := false
	rw.started = func(v js.Value) {
		if !once {
			once = true
			started(v)
		}
	}
	handler.ServeHTTP(rw, req)
	rw.finish()
	return nil
}

func headerFromPairs(pairs js.Value) http.Header {
	h := http.Header{}
	for i := 0; i < pairs.Length(); i++ {
		p := pairs.Index(i)
		h.Add(p.Index(0).String(), p.Index(1).String())
	}
	return h
}

func headerToPairs(h http.Header) js.Value {
	arr := js.Global().Get("Array").New()
	for k, vs := range h {
		for _, v := range vs {
			p := js.Global().Get("Array").New()
			p.Call("push", k, v)
			arr.Call("push", p)
		}
	}
	return arr
}

// BindingTransport is an http.RoundTripper that sends each request through
// the named Fetcher on the current request's env (a service or DO binding).
type BindingTransport struct {
	Name string
}

func (t BindingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	binding := Binding(req.Context(), t.Name)
	if binding.IsUndefined() || binding.IsNull() {
		return nil, fmt.Errorf("wasmhttp: binding %q is not in env", t.Name)
	}
	opts := js.Global().Get("Object").New()
	opts.Set("method", req.Method)
	opts.Set("headers", headerToPairs(req.Header))
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(body) > 0 {
			jsBody := js.Global().Get("Uint8Array").New(len(body))
			js.CopyBytesToJS(jsBody, body)
			opts.Set("body", jsBody)
		}
	}
	jsReq := js.Global().Get("Request").New(req.URL.String(), opts)
	jsResp, err := await(binding.Call("fetch", jsReq))
	if err != nil {
		return nil, fmt.Errorf("wasmhttp: fetch %s: %w", req.URL, err)
	}
	buf, err := await(jsResp.Call("arrayBuffer"))
	if err != nil {
		return nil, err
	}
	u8 := js.Global().Get("Uint8Array").New(buf)
	body := make([]byte, u8.Get("byteLength").Int())
	js.CopyBytesToGo(body, u8)
	header := http.Header{}
	entries := js.Global().Get("Array").Call("from", jsResp.Get("headers").Call("entries"))
	for i := 0; i < entries.Length(); i++ {
		p := entries.Index(i)
		header.Add(p.Index(0).String(), p.Index(1).String())
	}
	status := jsResp.Get("status").Int()
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

func await(promise js.Value) (js.Value, error) {
	done := make(chan struct{})
	var result js.Value
	var err error
	var onOK, onErr js.Func
	onOK = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer onOK.Release()
		if len(args) > 0 {
			result = args[0]
		}
		close(done)
		return nil
	})
	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer onErr.Release()
		msg := "rejected"
		if len(args) > 0 {
			if m := args[0].Get("message"); m.Type() == js.TypeString {
				msg = m.String()
			} else {
				msg = args[0].String()
			}
		}
		err = errors.New(strings.TrimSpace(msg))
		close(done)
		return nil
	})
	promise.Call("then", onOK, onErr)
	<-done
	return result, err
}

// WebSocket is a client connection opened through a binding's fetch with
// an Upgrade header, the only way a Worker dials a WebSocket.
type WebSocket struct {
	ws       js.Value
	Messages <-chan []byte
	closed   chan struct{}
}

func DialWebSocket(ctx context.Context, bindingName, rawURL string) (*WebSocket, error) {
	binding := Binding(ctx, bindingName)
	opts := js.Global().Get("Object").New()
	headers := js.Global().Get("Object").New()
	headers.Set("Upgrade", "websocket")
	opts.Set("headers", headers)
	resp, err := await(binding.Call("fetch", js.Global().Get("Request").New(rawURL, opts)))
	if err != nil {
		return nil, fmt.Errorf("wasmhttp: websocket %s: %w", rawURL, err)
	}
	ws := resp.Get("webSocket")
	if ws.IsNull() || ws.IsUndefined() {
		return nil, fmt.Errorf("wasmhttp: websocket %s: status %d", rawURL, resp.Get("status").Int())
	}
	msgs := make(chan []byte, 256)
	c := &WebSocket{ws: ws, Messages: msgs, closed: make(chan struct{})}
	var closeOnce func()
	closeOnce = func() {
		select {
		case <-c.closed:
		default:
			close(c.closed)
			close(msgs)
		}
	}
	ws.Call("addEventListener", "message", js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data")
		var b []byte
		if data.Type() == js.TypeString {
			b = []byte(data.String())
		} else {
			u8 := js.Global().Get("Uint8Array").New(data)
			b = make([]byte, u8.Get("byteLength").Int())
			js.CopyBytesToGo(b, u8)
		}
		select {
		case msgs <- b:
		case <-c.closed:
		}
		return nil
	}))
	ws.Call("addEventListener", "close", js.FuncOf(func(js.Value, []js.Value) any { closeOnce(); return nil }))
	ws.Call("addEventListener", "error", js.FuncOf(func(js.Value, []js.Value) any { closeOnce(); return nil }))
	ws.Call("accept")
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-c.closed:
		}
	}()
	return c, nil
}

func (c *WebSocket) Close() {
	func() {
		defer func() { recover() }()
		c.ws.Call("close", 1000, "done")
	}()
}
