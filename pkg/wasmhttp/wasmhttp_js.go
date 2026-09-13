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
				resp, err := dispatch(handler, reqObj, env)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				} else {
					resolve.Invoke(resp)
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

type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *recorder) WriteHeader(code int)        { r.status = code }
func (r *recorder) Flush()                      {}

func dispatch(handler http.Handler, reqObj, env js.Value) (js.Value, error) {
	u, err := url.Parse(reqObj.Get("url").String())
	if err != nil {
		return js.Value{}, err
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
	req = req.WithContext(context.WithValue(context.Background(), envKey{}, env))
	rec := &recorder{header: http.Header{}, status: http.StatusOK}
	handler.ServeHTTP(rec, req)
	out := js.Global().Get("Object").New()
	out.Set("status", rec.status)
	out.Set("headers", headerToPairs(rec.header))
	jsBody := js.Global().Get("Uint8Array").New(rec.body.Len())
	js.CopyBytesToJS(jsBody, rec.body.Bytes())
	out.Set("body", jsBody)
	return out, nil
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
