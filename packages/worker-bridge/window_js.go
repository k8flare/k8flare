//go:build js && wasm

package bridge

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"
)

var ErrWindowClosed = errors.New("bridge: pump window closed")

type job struct {
	fn    func()
	reply chan struct{}
}

type Window struct {
	id       int
	jobs     chan *job
	env      js.Value
	done     chan struct{}
	holding  bool
	streams  map[int]func()
	nextID   int
	pending  int
	drainFor time.Duration
	opened   time.Time
}

func (w *Window) kind() string {
	if w.holding {
		return "hold"
	}
	return "dispatch"
}

var (
	byIDMu sync.Mutex
	byID   = map[int]*Window{}
)

func windowByID(id int) *Window {
	byIDMu.Lock()
	defer byIDMu.Unlock()
	return byID[id]
}

func forgetWindow(id int) {
	byIDMu.Lock()
	delete(byID, id)
	byIDMu.Unlock()
}

// Run executes fn on the JS stack of the request that owns this window, so
// the I/O it starts belongs to a live request. It runs inline when this
// goroutine was already resumed by that request.
func (w *Window) Run(fn func()) error {
	if w == nil || w.Owns() {
		if w == nil {
			fn()
			return nil
		}
		fn()
		return nil
	}
	j := &job{fn: fn, reply: make(chan struct{})}
	select {
	case w.jobs <- j:
	case <-w.done:
		return ErrWindowClosed
	}
	select {
	case <-j.reply:
		return nil
	case <-w.done:
		return ErrWindowClosed
	}
}

// pump runs the queued jobs; it must be called from the owning request.
func (w *Window) pump() {
	for {
		select {
		case j := <-w.jobs:
			j.fn()
			close(j.reply)
		default:
			return
		}
	}
}

func (w *Window) Env() js.Value         { return w.env }
func (w *Window) Done() <-chan struct{} { return w.done }

type windowKey struct{}

const (
	scheduledWork = 10 * time.Second
	drainTimeout  = 5 * time.Second
	pokeDrain     = 12 * time.Second
	drainPoll     = 100 * time.Millisecond
)

var (
	windowsMu sync.Mutex
	windows   []*Window
	openedCh  = make(chan struct{})
	idle      chan struct{}
)

func openWindowFor(id int, env js.Value) *Window {
	w := &Window{id: id, jobs: make(chan *job, 64), env: env, done: make(chan struct{}), opened: time.Now()}
	if id != 0 {
		byIDMu.Lock()
		byID[id] = w
		byIDMu.Unlock()
	}
	windowsMu.Lock()
	windows = append(windows, w)
	notify := openedCh
	openedCh = make(chan struct{})
	if idle == nil {
		idle = make(chan struct{})
		go keepScheduled(idle)
	}
	windowsMu.Unlock()
	close(notify)
	return w
}

// inFlight reports the fetches this window is waiting on, so a timeout can say
// whether it was alone or one of many.
func (w *Window) inFlight() int {
	if w == nil {
		return 0
	}
	windowsMu.Lock()
	defer windowsMu.Unlock()
	return w.pending
}

func beginFetch(w *Window) func() {
	if w == nil {
		return func() {}
	}
	windowsMu.Lock()
	w.pending++
	windowsMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			windowsMu.Lock()
			w.pending--
			windowsMu.Unlock()
		})
	}
}

func (w *Window) drain() {
	limit := drainTimeout
	if w.drainFor > 0 {
		limit = w.drainFor
	}
	deadline := time.Now().Add(limit)
	for {
		windowsMu.Lock()
		n := w.pending
		windowsMu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			println("bridge: drain timeout kind="+w.kind()+" age="+time.Since(w.opened).Round(time.Second).String()+" in-flight:", n)
			return
		}
		time.Sleep(drainPoll)
	}
}

func (w *Window) close() {
	w.drain()
	windowsMu.Lock()
	for i, live := range windows {
		if live == w {
			windows = append(windows[:i], windows[i+1:]...)
			break
		}
	}
	ends := make([]func(), 0, len(w.streams))
	for _, end := range w.streams {
		ends = append(ends, end)
	}
	w.streams = nil
	if len(windows) == 0 && idle != nil {
		close(idle)
		idle = nil
	}
	windowsMu.Unlock()
	close(w.done)
	for _, end := range ends {
		end()
	}
}

func keepScheduled(stop chan struct{}) {
	t := time.NewTicker(scheduledWork)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-stop:
			return
		}
	}
}

func trackStream(w *Window, end func()) (update func(func()), untrack func()) {
	if w == nil {
		return func(func()) {}, func() {}
	}
	windowsMu.Lock()
	w.nextID++
	id := w.nextID
	if w.streams == nil {
		w.streams = map[int]func(){}
	}
	w.streams[id] = end
	windowsMu.Unlock()
	return func(end func()) {
			windowsMu.Lock()
			if _, ok := w.streams[id]; ok {
				w.streams[id] = end
			}
			windowsMu.Unlock()
		}, func() {
			windowsMu.Lock()
			delete(w.streams, id)
			windowsMu.Unlock()
		}
}

func OpenWindow(ctx context.Context) {
	w := windowFrom(ctx)
	if w == nil {
		return
	}
	windowsMu.Lock()
	w.holding = true
	w.drainFor = pokeDrain
	windowsMu.Unlock()
}

func CloseWindow(ctx context.Context) {
	w := windowFrom(ctx)
	if w == nil {
		return
	}
	windowsMu.Lock()
	w.holding = false
	windowsMu.Unlock()
}

func ownedBy(ctx context.Context, w *Window) bool {
	if windowFrom(ctx) == w {
		return true
	}
	windowsMu.Lock()
	defer windowsMu.Unlock()
	return w.holding
}

func windowFrom(ctx context.Context) *Window {
	w, _ := ctx.Value(windowKey{}).(*Window)
	return w
}

func CurrentWindow(ctx context.Context) (*Window, error) {
	if w := windowFrom(ctx); w != nil {
		return w, nil
	}
	for {
		windowsMu.Lock()
		w := preferred()
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

func preferred() *Window {
	for i := len(windows) - 1; i >= 0; i-- {
		if windows[i].holding {
			return windows[i]
		}
	}
	if len(windows) > 0 {
		return windows[len(windows)-1]
	}
	return nil
}

func currentWindowEnv() (js.Value, bool) {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	if w := preferred(); w != nil {
		return w.env, true
	}
	return js.Value{}, false
}

var ErrFetchTimeout = errors.New("bridge: no response headers in time")

var ErrBodyTimeout = errors.New("bridge: response body did not finish in time")

const (
	unaryHeaderTimeout   = 30 * time.Second
	unaryBodyTimeout     = 60 * time.Second
	apiServerBodyTimeout = 15 * time.Second
	openAPIHeaderTimeout = 90 * time.Second
	watchHeaderTimeout   = 20 * time.Second
	wsDialTimeout        = 10 * time.Second
)

var liveSockets atomic.Int64

// currentTurn names the window whose JS stack is currently running Go, so a
// fetch issued for another window can be reported instead of hanging.
var currentTurn atomic.Pointer[Window]

func EnterTurn(w *Window) func() {
	previous := currentTurn.Swap(w)
	return func() { currentTurn.Store(previous) }
}

func (w *Window) Owns() bool { return currentTurn.Load() == w }

func turnName() string {
	if w := currentTurn.Load(); w != nil {
		return w.kind() + "/" + time.Since(w.opened).Round(time.Second).String()
	}
	return "none"
}

func awaitIn(w *Window, promise js.Value) (js.Value, error) {
	return awaitInCtx(context.Background(), w, promise, 0)
}

func awaitInCtx(ctx context.Context, w *Window, promise js.Value, timeout time.Duration) (js.Value, error) {
	ch := settle(promise)
	var done <-chan struct{}
	if w != nil {
		done = w.Done()
	}
	var expired <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		expired = t.C
	}
	select {
	case o := <-ch:
		return o.value, o.err
	case <-done:
		return js.Value{}, ErrWindowClosed
	case <-ctx.Done():
		return js.Value{}, ctx.Err()
	case <-expired:
		return js.Value{}, ErrFetchTimeout
	}
}
