//go:build js && wasm

package workers

import (
	"net/http"
	"syscall/js"
	"testing"
)

// S34's first fault shape: the dispatch result used to be built as a JS
// Response object inside Go, which held a reference into the request's I/O
// context and produced "Cannot perform I/O on behalf of a different request"
// once that context was gone. The fix returns plain values and lets the
// bootstrap build the Response in the request's own handler. These tests pin
// the contract that makes that possible -- a plain object carrying status,
// statusText, headers and a byte body, with no Response constructor involved.
//
// They also exist because this package was untestable until registerBinding
// stopped running unconditionally at init(): see its doc comment.

func recorderWith(status int, header http.Header, body []byte) *responseRecorder {
	rec := &responseRecorder{header: make(http.Header), status: http.StatusOK}
	for k, vs := range header {
		for _, v := range vs {
			rec.Header().Add(k, v)
		}
	}
	rec.WriteHeader(status)
	if len(body) > 0 {
		rec.Write(body)
	}
	return rec
}

func TestToJSResponseReturnsPlainValuesNotAResponse(t *testing.T) {
	rec := recorderWith(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, []byte(`{"ok":true}`))
	out := rec.toJSResponse()

	if got := out.Type(); got != js.TypeObject {
		t.Fatalf("toJSResponse type = %v, want object", got)
	}
	if ctor := out.Get("constructor"); !ctor.IsUndefined() && ctor.Get("name").String() == "Response" {
		t.Errorf("toJSResponse built a Response; it must return plain values so the bootstrap can construct one in the request's own context")
	}
	if got := out.Get("status").Int(); got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	if got := out.Get("statusText").String(); got != "OK" {
		t.Errorf("statusText = %q, want %q", got, "OK")
	}
	if got := out.Get("body"); got.Get("length").Int() != len(`{"ok":true}`) {
		t.Errorf("body length = %d, want %d", got.Get("length").Int(), len(`{"ok":true}`))
	}
}

func TestToJSResponseSendsNoBodyForBodilessStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusSwitchingProtocols,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusNotModified,
	} {
		rec := recorderWith(status, nil, []byte("this must not be sent"))
		body := rec.toJSResponse().Get("body")
		if !body.IsNull() {
			t.Errorf("status %d: body = %v, want null -- the Fetch spec forbids a body here and workerd throws if one is given", status, body)
		}
	}
}

func TestRegisterBindingIsANoOpWithoutTheHostObject(t *testing.T) {
	// The guard that makes this package testable at all. Without it, init()
	// dereferenced globalThis.context.binding and panicked before any test ran.
	if registerBinding() {
		t.Errorf("registerBinding reported success with no globalThis.context.binding present")
	}
}
