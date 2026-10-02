package nodetunnel

import (
	"errors"
	"sync"

	"github.com/gorilla/websocket"
)

type streamSlot struct {
	mu      sync.Mutex
	cond    sync.Cond
	writer  StreamWriter
	pending [][]byte
	closed  bool
}

func newStreamSlot() *streamSlot {
	s := &streamSlot{}
	s.cond.L = &s.mu
	return s
}

func (s *streamSlot) writeLoop(w StreamWriter) {
	for {
		s.mu.Lock()
		for len(s.pending) == 0 && !s.closed {
			s.cond.Wait()
		}
		if s.closed {
			s.mu.Unlock()
			return
		}
		msg := s.pending[0]
		s.pending = s.pending[1:]
		s.mu.Unlock()

		if err := w.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			return
		}
	}
}

type StreamWriter interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type StreamRegistry struct {
	mu    sync.Mutex
	slots map[string]*streamSlot
}

func NewStreamRegistry() *StreamRegistry {
	return &StreamRegistry{
		slots: make(map[string]*streamSlot),
	}
}

func (r *StreamRegistry) Register(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.slots[id]; !ok {
		r.slots[id] = newStreamSlot()
	}
}

func (r *StreamRegistry) Attach(id string, w StreamWriter) bool {
	r.mu.Lock()
	slot, ok := r.slots[id]
	if !ok {
		r.mu.Unlock()
		return false
	}
	slot.mu.Lock()
	r.mu.Unlock()
	defer slot.mu.Unlock()

	if slot.closed || slot.writer != nil {
		return false
	}
	slot.writer = w
	go slot.writeLoop(w)
	return true
}

func (r *StreamRegistry) Send(id string, data []byte) error {
	r.mu.Lock()
	slot, ok := r.slots[id]
	if !ok {
		r.mu.Unlock()
		return errors.New("stream not found")
	}
	slot.mu.Lock()
	r.mu.Unlock()
	defer slot.mu.Unlock()

	if slot.closed {
		return errors.New("stream closed")
	}
	slot.pending = append(slot.pending, append([]byte(nil), data...))
	slot.cond.Signal()
	return nil
}

func (r *StreamRegistry) Close(id string) StreamWriter {
	r.mu.Lock()
	slot, ok := r.slots[id]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.slots, id)
	slot.mu.Lock()
	r.mu.Unlock()
	defer slot.mu.Unlock()

	slot.closed = true
	slot.cond.Broadcast()
	w := slot.writer
	return w
}

func (r *StreamRegistry) Detach(id string, w StreamWriter) bool {
	r.mu.Lock()
	slot, ok := r.slots[id]
	if !ok {
		r.mu.Unlock()
		return false
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	defer r.mu.Unlock()

	if slot.writer == w {
		delete(r.slots, id)
		slot.closed = true
		slot.cond.Broadcast()
		return true
	}
	return false
}
