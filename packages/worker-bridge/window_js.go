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

const expiryGrace = 2 * time.Second

type Window struct {
	id      int
	env     js.Value
	done    chan struct{}
	expires time.Time
	timer   *time.Timer
}

func (w *Window) Env() js.Value         { return w.env }
func (w *Window) Done() <-chan struct{} { return w.done }

var (
	windowsMu sync.Mutex
	windows   []*Window
	windowID  int
	byID      = map[int]*Window{}
	openedCh  = make(chan struct{})
)

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
	w.timer = time.AfterFunc(lifetime, func() { closeWindow(id) })
	windowsMu.Unlock()
	close(notify)
	return id
}

func ClosePumpWindow(id int) { closeWindow(id) }

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

func CurrentWindow(ctx context.Context) (*Window, error) {
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

func newestLive() *Window {
	now := time.Now()
	for i := len(windows) - 1; i >= 0; i-- {
		if now.Before(windows[i].expires) {
			return windows[i]
		}
	}
	return nil
}

func currentWindowEnv() (js.Value, bool) {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	if w := newestLive(); w != nil {
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
