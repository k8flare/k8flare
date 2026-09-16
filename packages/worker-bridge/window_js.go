//go:build js && wasm

package bridge

import (
	"context"
	"errors"
	"sync"
	"syscall/js"
	"time"
)

var ErrWindowClosed = errors.New("bridge: pump window closed")

type Window struct {
	env        js.Value
	done       chan struct{}
	holding    bool
	streams    map[int]func()
	nextID     int
	pending    int
	drainFor   time.Duration
	opened     time.Time
	run        bool
	superseded chan struct{}
}

func (w *Window) kind() string {
	switch {
	case w.run:
		return "run"
	case w.holding || w.drainFor == pokeDrain:
		return "poke"
	}
	return "dispatch"
}

func (w *Window) Env() js.Value         { return w.env }
func (w *Window) Done() <-chan struct{} { return w.done }

type windowKey struct{}

const (
	scheduledWork = 10 * time.Second
	drainTimeout  = 5 * time.Second
	pokeDrain     = 12 * time.Second
	runDrain      = 30 * time.Second
	drainPoll     = 100 * time.Millisecond
)

var (
	windowsMu sync.Mutex
	windows   []*Window
	openedCh  = make(chan struct{})
	idle      chan struct{}
)

func openWindow(env js.Value) *Window {
	w := &Window{env: env, done: make(chan struct{}), opened: time.Now(), superseded: make(chan struct{})}
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

func OpenRunWindow(ctx context.Context) {
	w := windowFrom(ctx)
	if w == nil {
		return
	}
	windowsMu.Lock()
	for _, other := range windows {
		if other != w && other.run && other.holding {
			other.holding = false
			close(other.superseded)
		}
	}
	w.holding = true
	w.run = true
	w.drainFor = runDrain
	windowsMu.Unlock()
}

func RunContext(ctx context.Context) context.Context {
	w := windowFrom(ctx)
	if w == nil {
		return ctx
	}
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-w.superseded:
			cancel()
		case <-runCtx.Done():
		}
	}()
	return runCtx
}

func Superseded(ctx context.Context) bool {
	w := windowFrom(ctx)
	if w == nil {
		return false
	}
	select {
	case <-w.superseded:
		return true
	default:
		return false
	}
}

func Holding() bool {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	for _, w := range windows {
		if w.holding {
			return true
		}
	}
	return false
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

const unaryHeaderTimeout = 30 * time.Second

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
