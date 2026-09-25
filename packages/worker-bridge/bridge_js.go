//go:build js && wasm

// Package bridge connects a Go http.Handler to the Worker Loader bootstrap
// (packages/control-plane-worker/src/loader.ts). The bootstrap instantiates the Go program once per
// isolate, waits for context.ready(), then calls context.binding.handleRequest
// (request, env) for every dispatch. Request bodies are buffered; responses
// and binding fetches stream.
package bridge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"
)

type envKey struct{}

// Callbacks handed to JS are shared functions bound to an id, so nothing
// ever calls a released js.Func: a released function reached from JS (a
// close event after an error event, a stream cancel racing its close)
// ends the Go program.
var (
	callbacksMu sync.Mutex
	callbacks   = map[int]any{}
	nextID      int
	shared      = map[string]js.Func{}
)

func register(v any) int {
	callbacksMu.Lock()
	defer callbacksMu.Unlock()
	nextID++
	callbacks[nextID] = v
	return nextID
}

func lookup(id int) any {
	callbacksMu.Lock()
	defer callbacksMu.Unlock()
	return callbacks[id]
}

func unregister(id int) {
	callbacksMu.Lock()
	defer callbacksMu.Unlock()
	delete(callbacks, id)
}

// bound returns the shared JS function `name` bound to id as its first
// argument.
func bound(name string, id int, fn func(id int, args []js.Value)) js.Value {
	callbacksMu.Lock()
	f, ok := shared[name]
	if !ok {
		f = js.FuncOf(func(_ js.Value, args []js.Value) any {
			fn(args[0].Int(), args[1:])
			return nil
		})
		shared[name] = f
	}
	callbacksMu.Unlock()
	return f.Call("bind", js.Null(), id)
}

func Env(ctx context.Context) js.Value {
	if ctx != nil {
		if v, ok := ctx.Value(envKey{}).(js.Value); ok {
			return v
		}
	}
	if env, ok := currentWindowEnv(); ok {
		println("bridge: env borrowed from another request")
		return env
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
	binding.Set("tick", js.FuncOf(func(js.Value, []js.Value) any {
		lastTick.Store(time.Now().UnixNano())
		return nil
	}))
	binding.Set("pump", js.FuncOf(func(_ js.Value, args []js.Value) any {
		lastTick.Store(time.Now().UnixNano())
		if len(args) == 0 || args[0].Type() != js.TypeNumber {
			return nil
		}
		w := windowByID(args[0].Int())
		if w == nil {
			return nil
		}
		leave := EnterTurn(w)
		defer leave()
		w.pump()
		return nil
	}))
	binding.Set("handleRequest", js.FuncOf(func(_ js.Value, args []js.Value) any {
		reqObj := args[0]
		env := args[1]
		var execCtx js.Value
		if len(args) > 2 {
			execCtx = args[2]
		}
		requestID := 0
		if len(args) > 3 && args[3].Type() == js.TypeNumber {
			requestID = args[3].Int()
		}
		var executor js.Func
		executor = js.FuncOf(func(_ js.Value, p []js.Value) any {
			defer executor.Release()
			resolve, reject := p[0], p[1]
			go func() {
				defer func() {
					if rec := recover(); rec != nil {
						println("bridge: panic in dispatch:", fmt.Sprint(rec))
						reject.Invoke(js.Global().Get("Error").New(fmt.Sprint(rec)))
					}
					yieldToEventLoop()
				}()
				if err := dispatch(handler, reqObj, env, execCtx, requestID, func(v js.Value) { resolve.Invoke(v) }); err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}
			}()
			return js.Undefined()
		})
		return js.Global().Get("Promise").New(executor)
	}))
	rt.Call("ready")
	select {}
}

// extend keeps the request's I/O context alive past the response so writes
// started near the end of a handler can still reach their binding.
func extend(execCtx js.Value) func() {
	if execCtx.IsUndefined() || execCtx.IsNull() || execCtx.Get("waitUntil").Type() != js.TypeFunction {
		return func() {}
	}
	done := make(chan struct{})
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, p []js.Value) any {
		defer executor.Release()
		resolve := p[0]
		go func() {
			<-done
			resolve.Invoke(js.Undefined())
		}()
		return js.Undefined()
	})
	execCtx.Call("waitUntil", js.Global().Get("Promise").New(executor))
	return func() { close(done) }
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
// upstream watch handler stream from a resident instance. A streamed
// response also declares Content-Encoding: identity, because the runtime
// otherwise gzips compressible content types for clients that accept it
// and holds the whole stream back until it closes.
type responseWriter struct {
	header     http.Header
	status     int
	buf        bytes.Buffer
	streaming  bool
	controller js.Value
	started    func(js.Value)
	cancel     context.CancelFunc
	closed     chan bool
	id         int
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

func (r *responseWriter) Write(b []byte) (n int, err error) {
	r.WriteHeader(http.StatusOK)
	if !r.streaming {
		return r.buf.Write(b)
	}
	defer func() {
		if rec := recover(); rec != nil {
			n, err = 0, fmt.Errorf("bridge: stream closed: %v", rec)
		}
	}()
	r.controller.Call("enqueue", toUint8Array(b))
	return len(b), nil
}

func bodyless(status int) bool {
	return status == http.StatusSwitchingProtocols || status == http.StatusNoContent || status == http.StatusResetContent || status == http.StatusNotModified
}

func (r *responseWriter) Flush() {
	r.WriteHeader(http.StatusOK)
	if r.streaming || bodyless(r.status) {
		return
	}
	src := js.Global().Get("Object").New()
	start := js.FuncOf(func(_ js.Value, args []js.Value) any {
		r.controller = args[0]
		return nil
	})
	defer start.Release()
	src.Set("start", start)
	r.id = register(r)
	src.Set("cancel", bound("stream-cancel", r.id, func(id int, _ []js.Value) {
		if w, ok := lookup(id).(*responseWriter); ok {
			select {
			case w.closed <- true:
			default:
			}
			w.cancel()
		}
	}))
	stream := js.Global().Get("ReadableStream").New(src)
	r.streaming = true
	if r.header.Get("Content-Encoding") == "" {
		r.header.Set("Content-Encoding", "identity")
	}
	pending := r.buf.Bytes()
	r.buf = bytes.Buffer{}
	r.started(r.response(stream))
	if len(pending) > 0 {
		_, _ = r.Write(pending)
	}
}

func (r *responseWriter) finish() {
	if r.streaming {
		func() {
			defer func() { recover() }()
			r.controller.Call("close")
		}()
		unregister(r.id)
		return
	}
	r.WriteHeader(http.StatusOK)
	if bodyless(r.status) {
		r.started(r.response(js.Null()))
		return
	}
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

func fromUint8Array(v js.Value) []byte {
	b := make([]byte, v.Get("byteLength").Int())
	js.CopyBytesToGo(b, v)
	return b
}

func dispatch(handler http.Handler, reqObj, env, execCtx js.Value, requestID int, started func(js.Value)) (err error) {
	u, err := url.Parse(reqObj.Get("url").String())
	if err != nil {
		return err
	}
	var body []byte
	if raw := reqObj.Get("body"); !raw.IsNull() && !raw.IsUndefined() {
		body = fromUint8Array(raw)
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
	window := openWindowFor(requestID, env)
	defer forgetWindow(requestID)
	leaveTurn := EnterTurn(window)
	defer leaveTurn()
	finished := extend(execCtx)
	defer func() {
		window.drain()
		window.close()
		finished()
	}()
	ctx, cancel := context.WithCancel(context.WithValue(context.WithValue(context.Background(), envKey{}, env), windowKey{}, window))
	defer cancel()
	req = req.WithContext(ctx)
	rw := &responseWriter{header: http.Header{}, cancel: cancel, closed: make(chan bool, 1), started: started}
	if signal := reqObj.Get("signal"); !signal.IsUndefined() && !signal.IsNull() {
		id := register(rw)
		defer unregister(id)
		signal.Call("addEventListener", "abort", bound("request-abort", id, func(id int, _ []js.Value) {
			if w, ok := lookup(id).(*responseWriter); ok {
				w.cancel()
			}
		}))
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
		if !validHeaderName(k) {
			continue
		}
		for _, v := range vs {
			if !validHeaderValue(v) {
				continue
			}
			p := js.Global().Get("Array").New()
			p.Call("push", k, v)
			arr.Call("push", p)
		}
	}
	return arr
}

func validHeaderName(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c <= 32 || c >= 127 || c == ':' {
			return false
		}
	}
	return true
}

func validHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\r' || c == '\n' || (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}

// BindingTransport is an http.RoundTripper that sends each request through
// the named Fetcher on the current request's env (a service or DO binding).
// Plain HTTP needs no transport of its own: net/http's default transport is
// fetch-based on GOOS=js and streams response bodies.
var lastTick atomic.Int64

func MarkTick() { lastTick.Store(time.Now().UnixNano()) }

func SinceTick() time.Duration {
	last := lastTick.Load()
	if last == 0 {
		return time.Duration(1<<63 - 1)
	}
	return time.Since(time.Unix(0, last))
}

type BindingTransport struct {
	Name        string
	AbortOnWake bool
}

func (t BindingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	window, err := CurrentWindow(req.Context())
	if err != nil {
		return nil, fmt.Errorf("bridge: waiting for a window: %w", err)
	}
	binding := window.Env().Get(t.Name)
	if binding.IsUndefined() || binding.IsNull() {
		return nil, fmt.Errorf("bridge: binding %q is not in env", t.Name)
	}
	var body []byte
	if req.Body != nil {
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
	}
	endFetch := beginFetch(window)
	headerTimeout := unaryHeaderTimeout
	if req.URL.Host == "openapi.internal" {
		headerTimeout = openAPIHeaderTimeout
	} else if req.URL.Query().Get("watch") == "true" || req.URL.Query().Get("watch") == "1" || strings.HasSuffix(req.URL.Path, "/watch") {
		headerTimeout = watchHeaderTimeout
	}
	var pending, controller js.Value
	if err := window.Run(func() {
		opts := js.Global().Get("Object").New()
		opts.Set("method", req.Method)
		opts.Set("headers", headerToPairs(req.Header))
		if len(body) > 0 {
			opts.Set("body", toUint8Array(body))
		}
		controller = js.Global().Get("AbortController").New()
		opts.Set("signal", controller.Get("signal"))
		pending = binding.Call("fetch", js.Global().Get("Request").New(req.URL.String(), opts))
	}); err != nil {
		endFetch()
		return nil, fmt.Errorf("bridge: fetch %s: %w", req.URL, err)
	}
	abort := func() {
		if !controller.IsUndefined() && !controller.IsNull() {
			_ = window.Run(func() { controller.Call("abort") })
		}
	}
	stop := context.AfterFunc(req.Context(), abort)
	jsResp, err := awaitInCtx(req.Context(), window, pending, headerTimeout)
	endFetch()
	if err != nil {
		stop()
		if errors.Is(err, ErrFetchTimeout) {
			abort()
			println("bridge: fetch timed out kind="+window.kind()+" age="+time.Since(window.opened).Round(time.Second).String()+":", req.Method, req.URL.Path)
		}
		if errors.Is(err, ErrWindowClosed) {
			println("bridge: fetch lost to window close kind="+window.kind()+" age="+time.Since(window.opened).Round(time.Second).String()+":", req.Method, req.URL.Path)
		}
		return nil, fmt.Errorf("bridge: fetch %s: %w", req.URL, err)
	}
	header := headerFromPairs(js.Global().Get("Array").Call("from", jsResp.Get("headers").Call("entries")))
	status := jsResp.Get("status").Int()
	respBody, end := streamBody(jsResp.Get("body"), abort)
	untrack := func() {}
	if t.AbortOnWake {
		_, untrack = trackStream(window, end)
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          abortingBody{respBody, func() { untrack(); stop(); abort() }},
		ContentLength: -1,
		Request:       req,
	}, nil
}

type abortingBody struct {
	io.ReadCloser
	abort func()
}

func (b abortingBody) Close() error {
	err := b.ReadCloser.Close()
	b.abort()
	return err
}

func streamBody(stream js.Value, abort func()) (io.ReadCloser, func()) {
	if stream.IsNull() || stream.IsUndefined() {
		return io.NopCloser(bytes.NewReader(nil)), func() {}
	}
	reader := stream.Call("getReader")
	pr, pw := io.Pipe()
	end := func() {
		pw.Close()
		abort()
	}
	go func() {
		for {
			chunk, err := await(reader.Call("read"))
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			if chunk.Get("done").Bool() {
				pw.Close()
				return
			}
			if _, err := pw.Write(fromUint8Array(chunk.Get("value"))); err != nil {
				abort()
				return
			}
		}
	}()
	return pr, end
}

type settled struct {
	value js.Value
	err   error
}

func settle(promise js.Value) <-chan settled {
	ch := make(chan settled, 1)
	var onOK, onErr js.Func
	onOK = js.FuncOf(func(_ js.Value, args []js.Value) any {
		onOK.Release()
		onErr.Release()
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- settled{value: v}
		return nil
	})
	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		onOK.Release()
		onErr.Release()
		msg := "rejected"
		if len(args) > 0 {
			if m := args[0].Get("message"); m.Type() == js.TypeString {
				msg = m.String()
			} else {
				msg = args[0].String()
			}
		}
		ch <- settled{err: errors.New(strings.TrimSpace(msg))}
		return nil
	})
	promise.Call("then", onOK, onErr)
	return ch
}

func await(promise js.Value) (js.Value, error) {
	o := <-settle(promise)
	return o.value, o.err
}

// WebSocket is a client connection opened through a binding's fetch with
// an Upgrade header, the only way a Worker dials a WebSocket.
type WebSocket struct {
	ws       js.Value
	Messages <-chan []byte
	msgs     chan []byte
	closed   chan struct{}
	notify   chan struct{}
	untrack  func()
	mu       sync.Mutex
	queue    [][]byte
	queued   int
	id       int
}

const maxQueuedWebSocketBytes = 1 << 20

func (c *WebSocket) enqueue(b []byte) {
	c.mu.Lock()
	c.queue = append(c.queue, b)
	c.queued += len(b)
	over := c.queued > maxQueuedWebSocketBytes
	c.mu.Unlock()
	if over {
		c.Close()
		return
	}
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

func (c *WebSocket) drain() {
	defer close(c.msgs)
	for {
		select {
		case <-c.notify:
		case <-c.closed:
			return
		}
		for {
			c.mu.Lock()
			if len(c.queue) == 0 {
				c.mu.Unlock()
				break
			}
			b := c.queue[0]
			c.queue = c.queue[1:]
			c.queued -= len(b)
			c.mu.Unlock()
			select {
			case c.msgs <- b:
			case <-c.closed:
				return
			}
		}
	}
}

func DialWebSocket(ctx context.Context, bindingName, rawURL string) (*WebSocket, error) {
	window, err := CurrentWindow(ctx)
	if err != nil {
		return nil, fmt.Errorf("bridge: waiting for a window: %w", err)
	}
	binding := window.Env().Get(bindingName)
	endFetch := beginFetch(window)
	started := time.Now()
	live := liveSockets.Add(1)
	println("bridge: ws dial start live="+strconv.FormatInt(live, 10)+" kind="+window.kind()+" turn="+turnName()+":", rawURL)
	var pending js.Value
	if err := window.Run(func() {
		opts := js.Global().Get("Object").New()
		headers := js.Global().Get("Object").New()
		headers.Set("Upgrade", "websocket")
		opts.Set("headers", headers)
		pending = binding.Call("fetch", js.Global().Get("Request").New(rawURL, opts))
	}); err != nil {
		endFetch()
		liveSockets.Add(-1)
		return nil, fmt.Errorf("bridge: websocket %s: %w", rawURL, err)
	}
	resp, err := awaitInCtx(ctx, window, pending, wsDialTimeout)
	endFetch()
	if err != nil {
		liveSockets.Add(-1)
		if errors.Is(err, ErrFetchTimeout) {
			println("bridge: ws dial timed out after "+time.Since(started).Round(time.Millisecond).String()+":", rawURL)
		}
		if errors.Is(err, ErrWindowClosed) {
			println("bridge: websocket lost to window close kind="+window.kind()+" age="+time.Since(window.opened).Round(time.Second).String()+":", rawURL)
		}
		return nil, fmt.Errorf("bridge: websocket %s: %w", rawURL, err)
	}
	ws := resp.Get("webSocket")
	if ws.IsNull() || ws.IsUndefined() {
		liveSockets.Add(-1)
		return nil, fmt.Errorf("bridge: websocket %s: status %d", rawURL, resp.Get("status").Int())
	}
	println("bridge: ws dial ok in "+time.Since(started).Round(time.Millisecond).String()+":", rawURL)
	msgs := make(chan []byte)
	c := &WebSocket{ws: ws, Messages: msgs, msgs: msgs, closed: make(chan struct{}), notify: make(chan struct{}, 1)}
	c.id = register(c)
	go c.drain()
	if ownedBy(ctx, window) {
		_, c.untrack = trackStream(window, c.Close)
	}
	ws.Call("addEventListener", "message", bound("ws-message", c.id, func(id int, args []js.Value) {
		w, ok := lookup(id).(*WebSocket)
		if !ok {
			return
		}
		data := args[0].Get("data")
		var b []byte
		if data.Type() == js.TypeString {
			b = []byte(data.String())
		} else {
			b = fromUint8Array(js.Global().Get("Uint8Array").New(data))
		}
		w.enqueue(b)
	}))
	onClose := bound("ws-close", c.id, func(id int, _ []js.Value) {
		if w, ok := lookup(id).(*WebSocket); ok {
			w.finish()
		}
	})
	ws.Call("addEventListener", "close", onClose)
	ws.Call("addEventListener", "error", onClose)
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

func (c *WebSocket) finish() {
	select {
	case <-c.closed:
	default:
		close(c.closed)
		liveSockets.Add(-1)
		if c.untrack != nil {
			c.untrack()
		}
		unregister(c.id)
	}
}

func (c *WebSocket) Close() {
	func() {
		defer func() { recover() }()
		c.ws.Call("close", 1000, "done")
	}()
	c.finish()
}

// Call invokes an RPC method on a service binding in the request's env and
// returns the resolved value. []byte arguments cross as Uint8Array.
func Call(ctx context.Context, bindingName, method string, args ...any) (js.Value, error) {
	binding := Binding(ctx, bindingName)
	if binding.IsUndefined() || binding.IsNull() {
		return js.Value{}, fmt.Errorf("bridge: binding %q is not in env", bindingName)
	}
	jsArgs := make([]any, len(args))
	for i, a := range args {
		if b, ok := a.([]byte); ok {
			jsArgs[i] = toUint8Array(b)
		} else {
			jsArgs[i] = a
		}
	}
	return await(binding.Call(method, jsArgs...))
}

// CallBytes is Call for methods that return bytes.
func CallBytes(ctx context.Context, bindingName, method string, args ...any) ([]byte, error) {
	v, err := Call(ctx, bindingName, method, args...)
	if err != nil {
		return nil, err
	}
	return fromUint8Array(js.Global().Get("Uint8Array").New(v)), nil
}

// HasBinding reports whether the request's env carries a binding.
func HasBinding(ctx context.Context, name string) bool {
	b := Binding(ctx, name)
	return !b.IsUndefined() && !b.IsNull()
}
