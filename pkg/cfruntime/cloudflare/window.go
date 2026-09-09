//go:build js && wasm

package cloudflare

import (
	gocontext "context"
	"errors"
	"sync"
	"syscall/js"
)

// ErrPumpWindowClosed reports that the pump window an outbound call was
// issued under closed before that call completed.
var ErrPumpWindowClosed = errors.New("cloudflare: pump window closed")

// Window is one request whose pump window is still open: the JS bootstrap
// (packages/k8flare-worker/src/loader/bootstrap.ts) opens one per dispatch
// and closes it when that request's ctx.waitUntil timer fires, so a live
// Window is exactly a request context a resident Go instance is allowed to
// perform I/O on behalf of.
//
// Bindings are request-scoped I/O objects. A Fetcher captured at Go.run
// time belongs to whichever request instantiated the isolate; once that
// request's IoContext is torn down, calls on it do not fail -- their
// promises never settle at all, so a goroutine awaiting one is wedged for
// the isolate's lifetime with nothing logged. That is the S31 defect
// (docs/platform-verification.md): the resident kube-controller-manager's
// reflectors stopped receiving Node/Lease events some minutes after load
// and only a fresh Loader id brought them back.
// Tearing an in-flight call down at close time -- an AbortController per
// request -- was tried and does not work: a Go goroutine resumes inside
// whichever request's JS callback happens to be running, so the
// controller is created under one request and aborted under another,
// which is itself a cross-request I/O access and panics the whole
// instance (measured 2026-09-09, S31). Waiting on Done is the only
// teardown that touches no I/O object at all.
type Window struct {
	env  js.Value
	done chan struct{}
}

// Env returns the request env this window carries: the source of every
// binding used for outbound I/O while the window is open.
func (w *Window) Env() js.Value { return w.env }

// Done is closed when this window closes.
func (w *Window) Done() <-chan struct{} { return w.done }

var (
	windowsMu sync.Mutex
	// Oldest first: the newest window is the anchor, because it is the one
	// whose IoContext survives longest.
	windows  []*Window
	windowID int
	byID     = map[int]*Window{}
	openedCh = make(chan struct{})

	abandonMu sync.Mutex
	abandoned []js.Func
	reapable  []js.Func
)

// OpenPumpWindow publishes env as this isolate's outbound-I/O anchor and
// returns the id the JS side passes back to ClosePumpWindow.
func OpenPumpWindow(env js.Value) int {
	windowsMu.Lock()
	windowID++
	id := windowID
	w := &Window{env: env, done: make(chan struct{})}
	byID[id] = w
	windows = append(windows, w)
	notify := openedCh
	openedCh = make(chan struct{})
	windowsMu.Unlock()
	close(notify)
	return id
}

// AbandonFunc hands over callbacks whose promise can no longer settle
// because the window it was issued under closed. They are released one
// window-close later rather than immediately: a promise the runtime has
// not yet abandoned could still deliver into a callback between this
// window's close and its IoContext actually going away, and invoking a
// released js.Func is a hard JS error.
func AbandonFunc(fns ...js.Func) {
	abandonMu.Lock()
	defer abandonMu.Unlock()
	abandoned = append(abandoned, fns...)
}

func reapAbandoned() {
	abandonMu.Lock()
	stale := reapable
	reapable = abandoned
	abandoned = nil
	abandonMu.Unlock()
	for _, fn := range stale {
		fn.Release()
	}
}

// ClosePumpWindow retires the window: it stops being the anchor for new
// calls before anything is woken, so a goroutine released by it waits for
// the next window instead of issuing I/O into a dying IoContext.
func ClosePumpWindow(id int) {
	reapAbandoned()
	windowsMu.Lock()
	w := byID[id]
	if w == nil {
		windowsMu.Unlock()
		return
	}
	delete(byID, id)
	for i, live := range windows {
		if live == w {
			windows = append(windows[:i], windows[i+1:]...)
			break
		}
	}
	windowsMu.Unlock()
	close(w.done)
}

// CurrentWindow returns the newest open window, blocking until one opens
// or ctx is done. Blocking is the event-armed behaviour this platform
// wants: a resident controller with no window to run under has nothing to
// do until the next poke arrives, and waiting costs nothing (cost
// invariant #2 -- I/O waits are not billed, and no timer is armed here).
func CurrentWindow(ctx gocontext.Context) (*Window, error) {
	for {
		windowsMu.Lock()
		if n := len(windows); n > 0 {
			w := windows[n-1]
			windowsMu.Unlock()
			return w, nil
		}
		wait := openedCh
		windowsMu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// currentWindowEnv is the non-blocking form behind EnvFromContext's
// fallback.
func currentWindowEnv() (js.Value, bool) {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	if n := len(windows); n > 0 {
		return windows[n-1].env, true
	}
	return js.Value{}, false
}
