//go:build js && wasm

package cloudflare

import (
	gocontext "context"
	"errors"
	"sync"
	"syscall/js"
	"time"
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
//
// A window therefore also carries its own expiry and closes itself when
// it is reached, rather than trusting the JS side to say so: the close
// callback rides on the dispatch's ctx.waitUntil, and production can
// tear that request's IoContext down before the timer ever runs. One
// dropped close was enough to wedge the whole resident instance for good
// -- everything blocked on that window's Done waits for a channel nobody
// will ever close, which is S31's original symptom by another route
// (measured 2026-09-09, S31 addendum).
type Window struct {
	id      int
	env     js.Value
	done    chan struct{}
	expires time.Time
	timer   *time.Timer
}

// Env returns the request env this window carries: the source of every
// binding used for outbound I/O while the window is open.
func (w *Window) Env() js.Value { return w.env }

// ID is the window's registry id, the same value OpenPumpWindow returned
// to JS. It names the window a trace observation happened inside.
func (w *Window) ID() int { return w.id }

// CurrentWindowID reports the newest live window's id, or 0 when no
// window is open. It never blocks: a caller that is only labelling an
// observation must not wait for a window that may never come.
func CurrentWindowID() int {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	if w := newestLive(); w != nil {
		return w.id
	}
	return 0
}

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
)

// expiryGrace keeps the Go-side expiry a clear backstop rather than a
// second authority on when a window ends: the JS close normally lands
// first, and only a dispatch whose IoContext died without running it
// falls through to the timer.
const expiryGrace = 2 * time.Second

// OpenPumpWindow publishes env as this isolate's outbound-I/O anchor for
// the next lifetimeMs and returns the id the JS side passes back to
// ClosePumpWindow.
func OpenPumpWindow(env js.Value, lifetimeMs int) int {
	lifetime := time.Duration(lifetimeMs)*time.Millisecond + expiryGrace
	windowsMu.Lock()
	windowID++
	id := windowID
	w := &Window{id: id, env: env, done: make(chan struct{}), expires: time.Now().Add(lifetime)}
	byID[id] = w
	windows = append(windows, w)
	notify := openedCh
	openedCh = make(chan struct{})
	// Arms no cost of its own: the Go runtime already keeps a JS timer
	// for its next deadline, and a frozen isolate simply fires this late
	// -- on the dispatch that gives the released goroutines somewhere to
	// retry (cost invariant #3: event-armed, one shot, self-disarming).
	w.timer = time.AfterFunc(lifetime, func() { closeWindow(id) })
	windowsMu.Unlock()
	close(notify)
	return id
}

// ClosePumpWindow retires the window: it stops being the anchor for new
// calls before anything is woken, so a goroutine released by it waits for
// the next window instead of issuing I/O into a dying IoContext.
func ClosePumpWindow(id int) {
	closeWindow(id)
}

// closeWindow is the one place a window is retired, shared by the JS
// close and the expiry timer. Releasing the waiters is the last thing it
// does and nothing after it can fail: reaping js.Funcs used to run
// first, and anything it threw would have left every goroutine blocked
// on this window's Done with no second chance.
func closeWindow(id int) {
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
	w.timer.Stop()
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
		w := newestLive()
		wait := openedCh
		windowsMu.Unlock()
		if w != nil {
			return w, nil
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// newestLive returns the newest window that has not passed its expiry,
// which is not the same as the newest window: a goroutine can be resumed
// by the JS event loop ahead of the expiry timer that is about to retire
// it, and handing it a window whose request is already gone is how a
// call ends up waiting on a promise that will never settle.
// Callers hold windowsMu.
func newestLive() *Window {
	now := time.Now()
	for i := len(windows) - 1; i >= 0; i-- {
		if now.Before(windows[i].expires) {
			return windows[i]
		}
	}
	return nil
}

// currentWindowEnv is the non-blocking form behind EnvFromContext's
// fallback.
func currentWindowEnv() (js.Value, bool) {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	if w := newestLive(); w != nil {
		return w.env, true
	}
	return js.Value{}, false
}
