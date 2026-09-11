//go:build js && wasm

package fetch

import (
	gocontext "context"
	"errors"
	"io"
	"net/http"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/k8flare/k8flare/pkg/cfruntime/cloudflare"
)

// liveClient is the resident-controller client shape: the binding is
// resolved from whichever window is open at the moment each call is
// issued.
func liveClient() *http.Client {
	return NewClient(WithLiveBinding("STORAGE")).HTTPClient(RedirectModeFollow)
}

type roundTripResult struct {
	status int
	body   string
	err    error
}

// getAsync issues a GET on its own goroutine so a test can assert on
// whether the call is still outstanding.
func getAsync(client *http.Client, url string) <-chan roundTripResult {
	out := make(chan roundTripResult, 1)
	go func() {
		resp, err := client.Get(url)
		if err != nil {
			out <- roundTripResult{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		out <- roundTripResult{status: resp.StatusCode, body: string(body), err: err}
	}()
	return out
}

// S31's fault shape: outbound I/O pinned to one request. A binding
// captured once belongs to the request that supplied it, and after that
// request's IoContext is torn down its calls neither succeed nor fail --
// the promise is abandoned, so a re-watch issued on it waits for the
// isolate's lifetime. Every call must therefore resolve its binding from
// the window that is open now, and a call caught by a closing window must
// be handed to the next one.
func TestWithLiveBindingHandsOffToTheNextWindow(t *testing.T) {
	dead := neverSettles()
	first := cloudflare.OpenPumpWindow(envWith("STORAGE", dead), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(first) })

	got := getAsync(liveClient(), "http://storage.internal/list/registry/nodes/")
	waitFor(t, "the call to reach the first window's binding", func() bool { return dead.callCount() == 1 })

	// The dispatch ends: the window is retired and the in-flight call is
	// let go of, rather than left waiting on a promise nobody will settle.
	cloudflare.ClosePumpWindow(first)

	live := newFakeBinding(func(int) js.Value {
		return resolved(jsResponse(http.StatusOK, map[string]string{"Content-Type": "application/json"}, bodyOnce(`{"kvs":[]}`)))
	})
	second := cloudflare.OpenPumpWindow(envWith("STORAGE", live), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(second) })

	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("GET across a window boundary failed: %v", r.err)
		}
		if r.status != http.StatusOK || r.body != `{"kvs":[]}` {
			t.Fatalf("GET returned %d %q, want 200 {\"kvs\":[]}", r.status, r.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GET never returned after its window closed and a new one opened: the caller is wedged, which is S31's symptom (no error, no retry, nothing logged)")
	}

	if live.callCount() != 1 {
		t.Fatalf("the live window's binding saw %d calls, want exactly 1 -- the retry must be issued on the window that is open now", live.callCount())
	}
}

// A resident controller with no window to run under has nothing to do
// until the next poke: waiting costs nothing, while failing the call would
// make client-go back off and burn the next window on a retry it did not
// need.
func TestWithLiveBindingWaitsForAWindowInsteadOfFailing(t *testing.T) {
	binding := newFakeBinding(func(int) js.Value {
		return resolved(jsResponse(http.StatusOK, nil, bodyOnce("ok")))
	})

	got := getAsync(liveClient(), "http://storage.internal/revision")
	settle()
	select {
	case r := <-got:
		t.Fatalf("GET returned %+v with no window open; it must wait for the next poke", r)
	default:
	}
	if binding.callCount() != 0 {
		t.Fatalf("binding was called %d times before any window opened", binding.callCount())
	}

	id := cloudflare.OpenPumpWindow(envWith("STORAGE", binding), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(id) })

	select {
	case r := <-got:
		if r.err != nil || r.status != http.StatusOK {
			t.Fatalf("GET on the window that opened: status %d, err %v", r.status, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GET did not proceed when a window opened")
	}
}

func TestWithLiveBindingReportsCtxCancellationRatherThanBlocking(t *testing.T) {
	ctx, cancel := gocontext.WithTimeout(gocontext.Background(), 100*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://storage.internal/revision", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := liveClient().Do(req)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "waiting for a pump window") {
			t.Fatalf("Do returned %v, want a waiting-for-a-pump-window error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Do never returned after its context expired with no window open")
	}
}

// The response-body half of S31, and the one that actually stalled the
// informers: a watch stream is answered immediately and then stays open,
// so the read that waits for the next event is the call the closing window
// catches. Body reads are deliberately not bounded by a deadline, which
// means the window is the only thing that can release this read.
func TestWatchBodyReadIsReleasedWhenItsWindowCloses(t *testing.T) {
	quietAfterFirstEvent := newFakeStream(func(call int) js.Value {
		if call == 1 {
			return resolved(chunkResult("{\"type\":\"ADDED\"}\n"))
		}
		// A watch with nothing to report: in production this promise is
		// abandoned with the request's IoContext, never rejected.
		return newPendingPromise().promise
	})
	binding := newFakeBinding(func(int) js.Value {
		return resolved(jsResponse(http.StatusOK, nil, quietAfterFirstEvent))
	})
	id := cloudflare.OpenPumpWindow(envWith("STORAGE", binding), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(id) })

	resp, err := liveClient().Get("http://storage.internal/list/registry/nodes/?watch=true")
	if err != nil {
		t.Fatalf("opening the watch: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 64)
	if n, err := resp.Body.Read(buf); err != nil || n == 0 {
		t.Fatalf("first watch event: n=%d err=%v", n, err)
	}

	type readResult struct {
		n   int
		err error
	}
	next := make(chan readResult, 1)
	go func() {
		n, err := resp.Body.Read(buf)
		next <- readResult{n, err}
	}()
	settle()
	select {
	case r := <-next:
		t.Fatalf("the second read returned %+v while the window was still open; it must be waiting on the stream", r)
	default:
	}

	cloudflare.ClosePumpWindow(id)
	select {
	case r := <-next:
		if !errors.Is(r.err, cloudflare.ErrPumpWindowClosed) {
			t.Fatalf("read after its window closed returned err %v, want ErrPumpWindowClosed so client-go re-watches on a live window", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read never returned after its window closed: the reflector is wedged for the isolate's lifetime with nothing logged, which is S31")
	}
}

// Tearing an in-flight call down by touching its I/O objects was tried and
// panics the whole instance: a goroutine resumes inside whichever
// request's JS callback happens to be running, so cancelling there is
// itself a cross-request I/O access (S31, rejected implementation).
// Closing a body whose window is gone must therefore touch nothing.
func TestClosingABodyWhoseWindowIsGoneTouchesNoIOObject(t *testing.T) {
	stream := newFakeStream(func(int) js.Value { return newPendingPromise().promise })
	binding := newFakeBinding(func(int) js.Value {
		return resolved(jsResponse(http.StatusOK, nil, stream))
	})
	id := cloudflare.OpenPumpWindow(envWith("STORAGE", binding), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(id) })

	resp, err := liveClient().Get("http://storage.internal/list/registry/nodes/?watch=true")
	if err != nil {
		t.Fatalf("opening the watch: %v", err)
	}
	cloudflare.ClosePumpWindow(id)
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := stream.cancelCount(); got != 0 {
		t.Fatalf("Close cancelled the stream %d times after its window closed; that is an I/O object belonging to a request that is gone", got)
	}

	// With the window still open, cancelling is both safe and required --
	// otherwise the DO-side watch socket is never closed.
	stream2 := newFakeStream(func(int) js.Value { return newPendingPromise().promise })
	binding2 := newFakeBinding(func(int) js.Value {
		return resolved(jsResponse(http.StatusOK, nil, stream2))
	})
	id2 := cloudflare.OpenPumpWindow(envWith("STORAGE", binding2), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(id2) })
	resp2, err := liveClient().Get("http://storage.internal/list/registry/nodes/?watch=true")
	if err != nil {
		t.Fatalf("opening the second watch: %v", err)
	}
	if err := resp2.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := stream2.cancelCount(); got != 1 {
		t.Fatalf("Close cancelled the stream %d times with the window open, want 1", got)
	}
}

// A withdrawn window is not an API failure, but re-issuing is only safe
// where the call is idempotent: the abandoned call may have reached the
// server, and a replayed POST with a generateName would mint a second
// object.
func TestPOSTIsNotReplayedOnANewWindow(t *testing.T) {
	dead := neverSettles()
	first := cloudflare.OpenPumpWindow(envWith("STORAGE", dead), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(first) })

	out := make(chan error, 1)
	go func() {
		_, err := liveClient().Post("http://storage.internal/key/registry/events/default/e1", "application/json", strings.NewReader(`{}`))
		out <- err
	}()
	waitFor(t, "the POST to reach the first window's binding", func() bool { return dead.callCount() == 1 })
	cloudflare.ClosePumpWindow(first)

	live := newFakeBinding(func(int) js.Value {
		return resolved(jsResponse(http.StatusCreated, nil, bodyOnce("{}")))
	})
	second := cloudflare.OpenPumpWindow(envWith("STORAGE", live), 60_000)
	t.Cleanup(func() { cloudflare.ClosePumpWindow(second) })

	select {
	case err := <-out:
		if !errors.Is(err, cloudflare.ErrPumpWindowClosed) {
			t.Fatalf("POST returned %v, want ErrPumpWindowClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("POST never returned after its window closed")
	}
	if live.callCount() != 0 {
		t.Fatalf("the POST was re-issued %d times on the next window; a non-idempotent write must not be replayed", live.callCount())
	}
}

// S34's second fault shape: a promise abandoned when its window closed can
// still settle much later (an old watch stream finally gets an event, a
// dying fetch finally rejects with "Network connection lost"). The js.Func
// reacting to it must therefore still be alive -- releasing it made
// syscall/js log "call to released function" and discard the reaction, and
// the earlier claim that releasing one window later was safe was wrong.
func TestAbandonedPromiseSettlingAfterItsWindowClosedIsHarmless(t *testing.T) {
	logged := consoleErrors(t)

	abandoned := make([]*pendingPromise, 0, 2)
	binding := newFakeBinding(func(int) js.Value {
		p := newPendingPromise()
		abandoned = append(abandoned, p)
		return p.promise
	})

	// POST twice so both trampolines are exercised: one late resolve and
	// one late reject. POST because it must not be replayed, which keeps
	// each call to exactly one abandoned promise.
	for i := 0; i < 2; i++ {
		id := cloudflare.OpenPumpWindow(envWith("STORAGE", binding), 60_000)
		out := make(chan error, 1)
		go func() {
			_, err := liveClient().Post("http://storage.internal/key/registry/events/default/e", "application/json", strings.NewReader(`{}`))
			out <- err
		}()
		want := i + 1
		waitFor(t, "the POST to reach the binding", func() bool { return binding.callCount() == want })
		cloudflare.ClosePumpWindow(id)
		select {
		case err := <-out:
			if !errors.Is(err, cloudflare.ErrPumpWindowClosed) {
				t.Fatalf("POST returned %v, want ErrPumpWindowClosed", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("POST never returned after its window closed")
		}
	}
	if len(abandoned) != 2 {
		t.Fatalf("expected 2 abandoned promises, got %d", len(abandoned))
	}

	// Three further dispatches carrying no calls of their own. This is
	// what made the defect rare rather than constant: the release was
	// deferred by a window, so only a stream that stayed quiet across
	// several windows (~50s) reached it -- measured as one occurrence per
	// 165-second probe, and one per whole conformance run.
	for i := 0; i < 3; i++ {
		cloudflare.ClosePumpWindow(cloudflare.OpenPumpWindow(envWith("STORAGE", binding), 60_000))
		settle()
	}

	// Only now does the runtime settle them.
	abandoned[0].resolve.Invoke(jsResponse(http.StatusOK, nil, bodyOnce("late")))
	abandoned[1].reject.Invoke(js.Global().Get("Error").New("Network connection lost."))
	settle()
	settle()

	for _, msg := range logged() {
		if strings.Contains(msg, "call to released function") {
			t.Fatalf("a promise that settled after its window closed hit a released js.Func (%q); the reaction is discarded and, in production, this is what S34 saw in wrangler.log", msg)
		}
	}
}

// The per-request path (pkg/apiserver's STORAGE self-binding) has no
// window at all: the request being served is the I/O context, so a
// captured binding is correct there and no window must be required.
func TestWithBindingNeedsNoWindow(t *testing.T) {
	binding := newFakeBinding(func(int) js.Value {
		return resolved(jsResponse(http.StatusOK, map[string]string{"Content-Type": "application/json"}, bodyOnce(`{"revision":7}`)))
	})
	client := NewClient(WithBinding(binding.value)).HTTPClient(RedirectModeFollow)

	resp, err := client.Get("http://storage.internal/revision")
	if err != nil {
		t.Fatalf("GET with a captured binding and no window open: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != `{"revision":7}` {
		t.Fatalf("GET returned %d %q", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestTracedComponentComesFromTheUserAgent(t *testing.T) {
	for ua, want := range map[string]string{
		// The real shape: restclient.AddUserAgent appends the component
		// name to DefaultKubernetesUserAgent(), which under wasm starts
		// with os.Args[0] == "js".
		"js/v0.0.0 (js/wasm) kubernetes/$Format/kube-controller-manager": "kube-controller-manager",
		"js/v0.0.0 (js/wasm) kubernetes/$Format/kube-scheduler":          "kube-scheduler",
		"js/v0.0.0 (js/wasm) kubernetes/$Format/garbage-collector":       "garbage-collector",
		"plain": "plain",
		"":      "unknown",
	} {
		req, _ := http.NewRequest(http.MethodPut, "https://x/api/v1/pods/p", nil)
		if ua != "" {
			req.Header.Set("User-Agent", ua)
		}
		if got := tracedComponent(req); got != want {
			t.Errorf("tracedComponent(%q) = %q, want %q", ua, got, want)
		}
	}
}

func TestOnlyWritesAreTracedAsIssued(t *testing.T) {
	// Reads and watches are the bulk of a controller's traffic and none of
	// them is an action; tracing them would make the boundary useless and
	// the log volume unbounded.
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req, _ := http.NewRequest(m, "https://x/api/v1/pods", nil)
		if isTracedWrite(req) {
			t.Errorf("%s is traced as an issued write", m)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req, _ := http.NewRequest(m, "https://x/api/v1/pods", nil)
		if !isTracedWrite(req) {
			t.Errorf("%s is not traced as an issued write", m)
		}
	}
}
