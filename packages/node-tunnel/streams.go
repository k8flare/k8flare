package nodetunnel

import (
	"sync"
)

type streamSlot struct {
	writer  StreamWriter
	pending [][]byte
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

func (r *StreamRegistry) Attach(id string, w StreamWriter) ([][]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.slots[id]
	if !ok {
		return nil, false
	}
	slot.writer = w
	queued := slot.pending
	slot.pending = nil
	return queued, true
}

func (r *StreamRegistry) Send(id string, data []byte) (StreamWriter, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.slots[id]
	if !ok {
		return nil, false
	}
	if slot.writer == nil {
		slot.pending = append(slot.pending, data)
		return nil, false
	}
	return slot.writer, true
}

func (r *StreamRegistry) Close(id string) StreamWriter {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.slots[id]
	if !ok {
		return nil
	}
	delete(r.slots, id)
	return slot.writer
}

func (r *StreamRegistry) Detach(id string, w StreamWriter) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.slots[id]
	if ok && slot.writer == w {
		delete(r.slots, id)
		return true
	}
	return false
}
