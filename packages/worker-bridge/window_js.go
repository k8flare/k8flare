//go:build js && wasm

package bridge

import (
	"context"
	"errors"
	"sync"
	"syscall/js"
)

var ErrWindowClosed = errors.New("bridge: pump window closed")

type Window struct {
	env     js.Value
	done    chan struct{}
	holding bool
}

func (w *Window) Env() js.Value         { return w.env }
func (w *Window) Done() <-chan struct{} { return w.done }

type windowKey struct{}

var (
	windowsMu sync.Mutex
	windows   []*Window
	openedCh  = make(chan struct{})
)

func openWindow(env js.Value) *Window {
	w := &Window{env: env, done: make(chan struct{})}
	windowsMu.Lock()
	windows = append(windows, w)
	notify := openedCh
	openedCh = make(chan struct{})
	windowsMu.Unlock()
	close(notify)
	return w
}

func (w *Window) close() {
	windowsMu.Lock()
	for i, live := range windows {
		if live == w {
			windows = append(windows[:i], windows[i+1:]...)
			break
		}
	}
	windowsMu.Unlock()
	close(w.done)
}

func OpenWindow(ctx context.Context) {
	w := windowFrom(ctx)
	if w == nil {
		return
	}
	windowsMu.Lock()
	w.holding = true
	windowsMu.Unlock()
	endTrackedStreams(0)
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

func awaitIn(w *Window, promise js.Value) (js.Value, error) {
	type outcome struct {
		value js.Value
		err   error
	}
	ch := make(chan outcome, 1)
	go func() {
		v, err := await(promise)
		ch <- outcome{v, err}
	}()
	if w == nil {
		o := <-ch
		return o.value, o.err
	}
	select {
	case o := <-ch:
		return o.value, o.err
	case <-w.Done():
		return js.Value{}, ErrWindowClosed
	}
}

func anyHolding() bool {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	for _, w := range windows {
		if w.holding {
			return true
		}
	}
	return false
}
