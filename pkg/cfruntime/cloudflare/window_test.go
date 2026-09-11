//go:build js && wasm

// Unit tests for the pump-window registry. They run as a js/wasm test
// binary under node (`make test-cfruntime`), so the JS boundary is a real
// JS runtime -- what node does NOT have is Cloudflare's request-scoped
// IoContext, so a torn-down request is modelled the way S31 measured it:
// an outbound call whose promise is never settled at all.
package cloudflare

import (
	gocontext "context"
	"syscall/js"
	"testing"
	"time"
)

// retireAllWindows drops registry state a previous test left behind: the
// registry is package-global, so an open window would otherwise be handed
// to the next test as its anchor.
func retireAllWindows(t *testing.T) {
	t.Helper()
	windowsMu.Lock()
	ids := make([]int, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	windowsMu.Unlock()
	for _, id := range ids {
		closeWindow(id)
	}
}

// openWindow publishes a window carrying a marker env, so a test can tell
// which request's env a caller resolved.
func openWindow(t *testing.T, marker string, lifetimeMs int) (int, js.Value) {
	t.Helper()
	env := js.Global().Get("Object").New()
	env.Set("marker", marker)
	return OpenPumpWindow(env, lifetimeMs), env
}

func marker(w *Window) string {
	if w == nil {
		return "<nil window>"
	}
	return w.Env().Get("marker").String()
}

func isClosed(w *Window) bool {
	select {
	case <-w.Done():
		return true
	default:
		return false
	}
}

// currentWindowAsync runs CurrentWindow on its own goroutine so a test can
// assert on whether it is still blocked.
func currentWindowAsync(ctx gocontext.Context) <-chan *Window {
	got := make(chan *Window, 1)
	go func() {
		w, err := CurrentWindow(ctx)
		if err != nil {
			got <- nil
			return
		}
		got <- w
	}()
	return got
}

// settle yields to the JS event loop long enough for the goroutines a test
// started to reach their next blocking point.
func settle() { time.Sleep(20 * time.Millisecond) }

func TestOpenPumpWindowPublishesItsEnvUntilClosed(t *testing.T) {
	retireAllWindows(t)
	id, env := openWindow(t, "w1", 60_000)

	w, err := CurrentWindow(gocontext.Background())
	if err != nil {
		t.Fatalf("CurrentWindow with an open window: %v", err)
	}
	if !w.Env().Equal(env) {
		t.Fatalf("CurrentWindow returned env %q, want the env just published", marker(w))
	}
	if isClosed(w) {
		t.Fatal("Done() is closed while the window is open")
	}

	ClosePumpWindow(id)
	if !isClosed(w) {
		t.Fatal("Done() is not closed after ClosePumpWindow")
	}
	if env, ok := currentWindowEnv(); ok {
		t.Fatalf("currentWindowEnv still returns %q after the only window closed", env.Get("marker"))
	}
}

func TestCurrentWindowBlocksUntilAWindowOpens(t *testing.T) {
	retireAllWindows(t)
	got := currentWindowAsync(gocontext.Background())
	settle()
	select {
	case w := <-got:
		t.Fatalf("CurrentWindow returned %q with no window open; it must wait for the next poke", marker(w))
	default:
	}

	openWindow(t, "w1", 60_000)
	select {
	case w := <-got:
		if marker(w) != "w1" {
			t.Fatalf("CurrentWindow returned %q, want w1", marker(w))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CurrentWindow did not wake when a window opened")
	}
}

func TestCurrentWindowReturnsTheNewestWindow(t *testing.T) {
	retireAllWindows(t)
	openWindow(t, "older", 60_000)
	openWindow(t, "newer", 60_000)

	w, err := CurrentWindow(gocontext.Background())
	if err != nil {
		t.Fatalf("CurrentWindow: %v", err)
	}
	// The newest window is the anchor because its IoContext outlives the
	// others', so a call issued now has the best chance of completing.
	if marker(w) != "newer" {
		t.Fatalf("CurrentWindow returned %q, want the newest window (newer)", marker(w))
	}
}

func TestCurrentWindowFallsBackToAnOlderWindowWhenTheNewestCloses(t *testing.T) {
	retireAllWindows(t)
	openWindow(t, "older", 60_000)
	newest, _ := openWindow(t, "newer", 60_000)

	ClosePumpWindow(newest)

	w, err := CurrentWindow(gocontext.Background())
	if err != nil {
		t.Fatalf("CurrentWindow: %v", err)
	}
	if marker(w) != "older" {
		t.Fatalf("CurrentWindow returned %q after the newest window closed, want older", marker(w))
	}
}

// A goroutine released by an open notification must re-check the registry
// under the lock: the window that woke it can already be retired by the
// time it runs, and handing that one out is how a call ends up awaiting a
// promise nobody will ever settle (S31).
func TestCurrentWindowRacingACloseDoesNotReturnTheClosedWindow(t *testing.T) {
	retireAllWindows(t)
	got := currentWindowAsync(gocontext.Background())
	settle()

	id, _ := openWindow(t, "raced", 60_000)
	ClosePumpWindow(id)
	settle()

	select {
	case w := <-got:
		t.Fatalf("CurrentWindow returned %q, a window that was already closed", marker(w))
	default:
	}

	openWindow(t, "live", 60_000)
	select {
	case w := <-got:
		if marker(w) != "live" {
			t.Fatalf("CurrentWindow returned %q, want live", marker(w))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CurrentWindow did not wake on the window that followed the raced close")
	}
}

// The expiry timer can be scheduled behind the goroutine it is meant to
// protect, so liveness is a comparison against the window's own deadline,
// not just registry membership.
func TestCurrentWindowSkipsAWindowPastItsExpiryBeforeItsTimerRuns(t *testing.T) {
	retireAllWindows(t)
	openWindow(t, "older", 60_000)
	newest, _ := openWindow(t, "expired", 60_000)

	windowsMu.Lock()
	byID[newest].expires = time.Now().Add(-time.Second)
	windowsMu.Unlock()

	w, err := CurrentWindow(gocontext.Background())
	if err != nil {
		t.Fatalf("CurrentWindow: %v", err)
	}
	if marker(w) != "older" {
		t.Fatalf("CurrentWindow returned %q, want the still-live older window", marker(w))
	}
}

func TestCurrentWindowReturnsCtxErrWhenNoWindowOpens(t *testing.T) {
	retireAllWindows(t)
	ctx, cancel := gocontext.WithTimeout(gocontext.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := CurrentWindow(ctx); err == nil {
		t.Fatal("CurrentWindow returned no error when ctx expired with no window open")
	}
}

// S31 addendum: production can tear a dispatch's IoContext down before the
// ctx.waitUntil timer that would have called closePumpWindow, and one
// dropped close used to wedge the whole resident instance forever --
// everything blocked on that window's Done waited on a channel nobody
// would close. The window therefore closes itself at its own deadline.
func TestWindowClosesItselfWhenTheJSCloseIsLost(t *testing.T) {
	retireAllWindows(t)
	id, _ := openWindow(t, "close-lost", 200)
	w := byID[id]
	if w == nil {
		t.Fatal("window is not registered")
	}
	// The Go-side expiry is a backstop, not a second authority on when a
	// window ends: it must not fire before the lifetime JS promised.
	time.Sleep(100 * time.Millisecond)
	if isClosed(w) {
		t.Fatal("window closed before the lifetime the JS side promised")
	}

	deadline := time.After(200*time.Millisecond + expiryGrace + time.Second)
	select {
	case <-w.Done():
	case <-deadline:
		t.Fatal("window never closed although closePumpWindow was never called: every goroutine waiting on it is wedged for the isolate's lifetime")
	}

	if _, ok := currentWindowEnv(); ok {
		t.Fatal("a self-expired window is still being handed out as the anchor")
	}
}

func TestClosePumpWindowIsIdempotentAndIgnoresUnknownIDs(t *testing.T) {
	retireAllWindows(t)
	id, _ := openWindow(t, "w1", 60_000)
	ClosePumpWindow(id)
	ClosePumpWindow(id)
	ClosePumpWindow(id + 1000)
	if _, ok := currentWindowEnv(); ok {
		t.Fatal("currentWindowEnv returns a window after every window was closed")
	}
}
