package nodetunnel

import (
	"errors"
	"sync"

	"github.com/gorilla/websocket"
)

type streamSlot struct {
	mu      sync.Mutex
	writer  StreamWriter
	pending [][]byte
	closed  bool
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
		r.slots[id] = &streamSlot{}
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

	if slot.closed {
		return false
	}
	for _, data := range slot.pending {
		_ = w.WriteMessage(websocket.BinaryMessage, data)
	}
	slot.pending = nil
	slot.writer = w
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
	if slot.writer == nil {
		slot.pending = append(slot.pending, append([]byte(nil), data...))
		return nil
	}
	return slot.writer.WriteMessage(websocket.BinaryMessage, data)
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
	w := slot.writer
	slot.writer = nil
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
		slot.writer = nil
		return true
	}
	return false
}
