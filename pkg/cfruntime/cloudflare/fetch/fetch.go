//go:build js && wasm

// Package fetch issues outbound HTTP requests through a Cloudflare
// binding's fetch() method (a service binding, or -- via WithBinding
// omitted -- the global fetch()). Used for exactly two things in this
// repo: pkg/apiserver/cmd/apiserver-wasm's STORAGE self-binding client,
// and pkg/controllers/restconfig.go's client-go Transport for the same
// service-binding-backed request pattern.
// https://developers.cloudflare.com/workers/runtime-apis/fetch/
package fetch

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
)

// RedirectMode is the redirect mode of an outbound fetch() request.
// https://developers.cloudflare.com/workers/runtime-apis/request/#the-cf-property-requestinitcfproperties
type RedirectMode string

const (
	RedirectModeFollow RedirectMode = "follow"
	RedirectModeError  RedirectMode = "error"
	RedirectModeManual RedirectMode = "manual"
)

type Client struct {
	binding  js.Value
	liveName string
}

type ClientOption func(*Client)

// WithBinding routes fetch() through binding (e.g. a Workers service
// binding) instead of the global fetch (js.Global()).
func WithBinding(binding js.Value) ClientOption {
	return func(c *Client) { c.binding = binding }
}

// WithLiveBinding routes fetch() through the named binding taken from
// whichever pump window is open when each individual request is issued,
// waiting for one to open if none is.
//
// Resident binaries whose background goroutines outlive any single
// dispatch (the controllers) must use this rather than WithBinding: a
// binding captured once belongs to the request that supplied it, and
// after that request's IoContext is torn down its calls neither succeed
// nor fail -- the promises simply never settle, wedging the caller for
// the isolate's lifetime (S31).
func WithLiveBinding(name string) ClientOption {
	return func(c *Client) { c.liveName = name }
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{binding: js.Global()}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HTTPClient returns an *http.Client whose RoundTripper calls the
// client's binding.fetch() for every request.
func (c *Client) HTTPClient(redirect RedirectMode) *http.Client {
	return &http.Client{Transport: &roundTripper{binding: c.binding, liveName: c.liveName, redirect: redirect}}
}

type roundTripper struct {
	binding  js.Value
	liveName string
	redirect RedirectMode
}

// jsCall invokes a JS method, turning the JS exceptions syscall/js raises
// as Go panics into ordinary errors. Every I/O object this package
// touches belongs to a request context that the runtime can tear down
// underneath it, and a panic on that boundary takes the whole resident
// instance down (exit code 2) instead of failing the one call that the
// caller is already prepared to retry.
func jsCall(v js.Value, method string, args ...any) (result js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = js.Value{}, fmt.Errorf("js %s: %v", method, r)
		}
	}()
	return v.Call(method, args...), nil
}

func (t *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	binding := t.binding
	var window *cloudflare.Window
	if t.liveName != "" {
		w, err := cloudflare.CurrentWindow(req.Context())
		if err != nil {
			return nil, fmt.Errorf("fetch: waiting for a pump window: %w", err)
		}
		window, binding = w, w.Env().Get(t.liveName)
	}

	jsReq, err := requestToJS(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: encode request: %w", err)
	}
	init := js.Global().Get("Object").New()
	init.Set("redirect", string(t.redirect))

	promise, err := jsCall(binding, "fetch", jsReq, init)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	jsResp, err := awaitPromise(window, promise)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	return responseFromJS(window, jsResp)
}

func requestToJS(req *http.Request) (js.Value, error) {
	opts := js.Global().Get("Object").New()
	opts.Set("method", req.Method)
	opts.Set("headers", headerToJS(req.Header))
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return js.Value{}, err
		}
		jsBody := js.Global().Get("Uint8Array").New(len(body))
		js.CopyBytesToJS(jsBody, body)
		opts.Set("body", jsBody)
	}
	return js.Global().Get("Request").New(req.URL.String(), opts), nil
}

// responseFromJS wraps the JS Response body's ReadableStream in a
// streaming io.ReadCloser (streamBody below), chunk by chunk as the
// stream produces them. It MUST NOT buffer the whole body up front: an
// earlier version read the body in one shot via arrayBuffer(), which
// was correct for every caller checked at the time (kine GETs, vault
// reads -- all bounded JSON) but silently deadlocked every Kubernetes
// WATCH stream (an unbounded response whose arrayBuffer() promise never
// settles), and client-go v1.35+ enables WatchListClient by default, so
// even the reflector's INITIAL sync is a watch -- with the buffering
// version, kube-controller-manager's and the garbage collector's
// informers never completed a single sync (found live 2026-07-10, see
// docs/platform-verification.md's cfruntime-rewrite correction).
func responseFromJS(window *cloudflare.Window, resp js.Value) (*http.Response, error) {
	status := resp.Get("status").Int()
	header := headerFromJS(resp.Get("headers"))

	var body io.ReadCloser = http.NoBody
	if stream := resp.Get("body"); !stream.IsNull() && !stream.IsUndefined() {
		body = &streamBody{stream: stream, window: window}
	}

	contentLength := int64(-1) // unknown (e.g. a streaming watch)
	if cl := header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			contentLength = n
		}
	}
	return &http.Response{
		Status:        strconv.Itoa(status) + " " + resp.Get("statusText").String(),
		StatusCode:    status,
		Header:        header,
		Body:          body,
		ContentLength: contentLength,
	}, nil
}

// streamBody adapts a JS ReadableStream to io.ReadCloser: each Read
// pulls at most one chunk from the stream's reader when the local
// buffer is empty (same shape as the reference implementation this
// repo's earlier vendored syumai/workers fork carried in
// internal/jsutil/stream.go, which the original cfruntime absorbed --
// see spikes/s8-wasm-resident/vendor/syumai-workers-fork).
type streamBody struct {
	stream js.Value
	reader js.Value // lazily: stream.getReader()
	window *cloudflare.Window
	buf    bytes.Buffer
	eof    bool
}

func (b *streamBody) Read(p []byte) (int, error) {
	if b.buf.Len() == 0 {
		if b.eof {
			return 0, io.EOF
		}
		if b.reader.IsUndefined() {
			reader, err := jsCall(b.stream, "getReader")
			if err != nil {
				b.eof = true
				return 0, fmt.Errorf("read response stream: %w", err)
			}
			b.reader = reader
		}
		pending, err := jsCall(b.reader, "read")
		if err != nil {
			b.eof = true
			return 0, fmt.Errorf("read response stream: %w", err)
		}
		result, err := awaitPromise(b.window, pending)
		if err != nil {
			b.eof = true
			return 0, fmt.Errorf("read response stream: %w", err)
		}
		if result.Get("done").Bool() {
			b.eof = true
			return 0, io.EOF
		}
		value := result.Get("value") // a Uint8Array chunk
		chunk := make([]byte, value.Get("byteLength").Int())
		js.CopyBytesToGo(chunk, value)
		b.buf.Write(chunk)
	}
	return b.buf.Read(p)
}

func (b *streamBody) windowOpen() bool {
	if b.window == nil {
		return true
	}
	select {
	case <-b.window.Done():
		return false
	default:
		return true
	}
}

func (b *streamBody) Close() error {
	if !b.eof && b.windowOpen() {
		if !b.reader.IsUndefined() {
			_, _ = jsCall(b.reader, "cancel")
		} else if !b.stream.IsUndefined() && !b.stream.IsNull() {
			_, _ = jsCall(b.stream, "cancel")
		}
	}
	b.eof = true
	return nil
}

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

// awaitPromise blocks the calling goroutine until promise settles, or
// until window closes -- a promise created under a torn-down IoContext is
// abandoned by the runtime rather than rejected, so without that second
// arm the caller would block for the isolate's lifetime.
// Duplicated from pkg/cfruntime's own copy rather than shared through an
// internal package -- see pkg/cfruntime/README.md for why.
func awaitPromise(window *cloudflare.Window, promise js.Value) (js.Value, error) {
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
	settled, err := jsCall(promise, "then", then)
	if err != nil {
		then.Release()
		catch.Release()
		return js.Value{}, err
	}
	if _, err := jsCall(settled, "catch", catch); err != nil {
		return js.Value{}, err
	}
	var closed <-chan struct{}
	if window != nil {
		closed = window.Done()
	}
	select {
	case v := <-resultCh:
		return v, nil
	case err := <-errCh:
		return js.Value{}, err
	case <-closed:
		cloudflare.AbandonFunc(then, catch)
		return js.Value{}, cloudflare.ErrPumpWindowClosed
	}
}
