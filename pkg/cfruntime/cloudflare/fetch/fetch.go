//go:build js && wasm

// Package fetch issues outbound HTTP requests through a Cloudflare
// binding's fetch() method (a service binding, or -- via WithBinding
// omitted -- the global fetch()). Used for exactly two things in this
// repo: cmd/apiserver-wasm's STORAGE self-binding client, and
// pkg/controllers/restconfig.go's client-go Transport for the same
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
	binding js.Value
}

type ClientOption func(*Client)

// WithBinding routes fetch() through binding (e.g. a Workers service
// binding) instead of the global fetch (js.Global()).
func WithBinding(binding js.Value) ClientOption {
	return func(c *Client) { c.binding = binding }
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
	return &http.Client{Transport: &roundTripper{binding: c.binding, redirect: redirect}}
}

type roundTripper struct {
	binding  js.Value
	redirect RedirectMode
}

func (t *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	jsReq, err := requestToJS(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: encode request: %w", err)
	}
	init := js.Global().Get("Object").New()
	init.Set("redirect", string(t.redirect))

	jsResp, err := awaitPromise(t.binding.Call("fetch", jsReq, init))
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	return responseFromJS(jsResp)
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

// responseFromJS reads the JS Response body in one shot via
// arrayBuffer() -- every caller of this client in this repo
// (pkg/apiserver/storage.go, cmd/apiserver-wasm's vault token read,
// client-go's own REST decoding via RestConfig) does
// json.NewDecoder(resp.Body).Decode(...), which works identically over a
// pre-buffered reader.
func responseFromJS(resp js.Value) (*http.Response, error) {
	buf, err := awaitPromise(resp.Call("arrayBuffer"))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	body := make([]byte, buf.Get("byteLength").Int())
	js.CopyBytesToGo(body, js.Global().Get("Uint8Array").New(buf))

	status := resp.Get("status").Int()
	header := headerFromJS(resp.Get("headers"))
	return &http.Response{
		Status:        strconv.Itoa(status) + " " + resp.Get("statusText").String(),
		StatusCode:    status,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}, nil
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

// awaitPromise blocks the calling goroutine until promise settles.
// Duplicated from pkg/cfruntime's own copy rather than shared through an
// internal package -- see pkg/cfruntime/README.md for why.
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
