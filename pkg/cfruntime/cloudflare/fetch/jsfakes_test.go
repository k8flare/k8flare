//go:build js && wasm

// JS fakes for the fetch tests. node gives these tests a real JS runtime
// and real promise semantics, which is what the code under test actually
// talks to; what node does not have is Cloudflare's request-scoped
// IoContext. So a torn-down request is modelled the way S31 measured it in
// production and reproduced it in `wrangler dev` (E2): the promise a
// binding returned is neither resolved nor rejected, it is simply
// abandoned.
package fetch

import (
	"strings"
	"sync"
	"syscall/js"
	"testing"
	"time"
)

func newObject() js.Value { return js.Global().Get("Object").New() }

// resolved wraps v in an already-settled promise.
func resolved(v js.Value) js.Value { return js.Global().Get("Promise").Call("resolve", v) }

type pendingPromise struct {
	promise js.Value
	resolve js.Value
	reject  js.Value
}

// newPendingPromise returns a promise nothing has settled yet.
func newPendingPromise() *pendingPromise {
	p := &pendingPromise{}
	// A Promise executor runs synchronously inside the constructor, so
	// resolve/reject are captured by the time New returns.
	p.promise = js.Global().Get("Promise").New(js.FuncOf(func(_ js.Value, args []js.Value) any {
		p.resolve, p.reject = args[0], args[1]
		return js.Undefined()
	}))
	return p
}

// fakeBinding is a Workers service binding: one fetch() method whose
// answer for each call the test chooses, and a call count so a test can
// prove which window's binding a call went to.
type fakeBinding struct {
	value  js.Value
	answer func(call int) js.Value

	mu    sync.Mutex
	calls int
}

func newFakeBinding(answer func(call int) js.Value) *fakeBinding {
	b := &fakeBinding{answer: answer}
	obj := newObject()
	obj.Set("fetch", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		b.mu.Lock()
		b.calls++
		call := b.calls
		b.mu.Unlock()
		return b.answer(call)
	}))
	b.value = obj
	return b
}

func (b *fakeBinding) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// neverSettles is the binding of a request whose IoContext has been torn
// down: fetch() returns, and the promise is then abandoned.
func neverSettles() *fakeBinding {
	return newFakeBinding(func(int) js.Value { return newPendingPromise().promise })
}

// fakeStream is the ReadableStream shape streamBody drives: getReader(),
// then one read() per chunk, plus the cancel() that must not be called on
// a dead request's I/O object.
type fakeStream struct {
	value js.Value
	reads func(call int) js.Value

	mu      sync.Mutex
	readN   int
	cancels int
}

func newFakeStream(reads func(call int) js.Value) *fakeStream {
	s := &fakeStream{reads: reads}
	cancel := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s.mu.Lock()
		s.cancels++
		s.mu.Unlock()
		return resolved(js.Undefined())
	})
	reader := newObject()
	reader.Set("read", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		s.mu.Lock()
		s.readN++
		call := s.readN
		s.mu.Unlock()
		return s.reads(call)
	}))
	reader.Set("cancel", cancel)
	obj := newObject()
	obj.Set("getReader", js.FuncOf(func(_ js.Value, _ []js.Value) any { return reader }))
	obj.Set("cancel", cancel)
	s.value = obj
	return s
}

func (s *fakeStream) cancelCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancels
}

// chunkResult is one { done: false, value: Uint8Array } read result.
func chunkResult(data string) js.Value {
	buf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buf, []byte(data))
	out := newObject()
	out.Set("done", false)
	out.Set("value", buf)
	return out
}

// endOfStream is the { done: true } read result.
func endOfStream() js.Value {
	out := newObject()
	out.Set("done", true)
	return out
}

// jsResponse builds the { status, statusText, headers, body } shape
// responseFromJS reads. Deliberately a plain object rather than a real
// Response: the point of the fakes is to control when each promise
// settles, which a real Response body does not allow.
func jsResponse(status int, header map[string]string, body *fakeStream) js.Value {
	entries := js.Global().Get("Array").New()
	for k, v := range header {
		pair := js.Global().Get("Array").New()
		pair.Call("push", k)
		pair.Call("push", v)
		entries.Call("push", pair)
	}
	headers := newObject()
	headers.Set("entries", js.FuncOf(func(_ js.Value, _ []js.Value) any { return entries }))

	resp := newObject()
	resp.Set("status", status)
	resp.Set("statusText", "OK")
	resp.Set("headers", headers)
	if body == nil {
		resp.Set("body", js.Null())
	} else {
		resp.Set("body", body.value)
	}
	return resp
}

// bodyOnce answers a single-chunk body: the chunk, then end of stream.
func bodyOnce(data string) *fakeStream {
	return newFakeStream(func(call int) js.Value {
		if call == 1 {
			return resolved(chunkResult(data))
		}
		return resolved(endOfStream())
	})
}

// envWith publishes an env object carrying one named binding, the way the
// JS bootstrap hands a dispatch's env to openPumpWindow.
func envWith(name string, binding *fakeBinding) js.Value {
	env := newObject()
	env.Set(name, binding.value)
	return env
}

// consoleErrors replaces console.error for the duration of a test and
// collects what was logged to it. syscall/js reports a JS call into a
// released js.Func by logging exactly there (see the runtime's
// handleEvent), which is how production surfaced S34's second defect as
// "call to released function" in wrangler.log -- and the reaction that
// logged it is discarded, so the log line is the only observable.
func consoleErrors(t *testing.T) func() []string {
	t.Helper()
	console := js.Global().Get("console")
	original := console.Get("error")
	var mu sync.Mutex
	var logged []string
	console.Set("error", js.FuncOf(func(_ js.Value, args []js.Value) any {
		parts := make([]string, 0, len(args))
		for _, a := range args {
			parts = append(parts, js.Global().Get("String").Invoke(a).String())
		}
		mu.Lock()
		logged = append(logged, strings.Join(parts, " "))
		mu.Unlock()
		return js.Undefined()
	}))
	t.Cleanup(func() { console.Set("error", original) })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), logged...)
	}
}

// settle yields to the JS event loop long enough for pending promise
// reactions to run and for the goroutines a test started to reach their
// next blocking point.
func settle() { time.Sleep(20 * time.Millisecond) }

// waitFor polls cond, yielding to the event loop between attempts.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
