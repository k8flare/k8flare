package nodetunnel

import (
	"bytes"
	"sync"
	"testing"
)

type mockStreamWriter struct {
	mu       sync.Mutex
	closed   bool
	messages [][]byte
}

func (m *mockStreamWriter) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, append([]byte(nil), data...))
	return nil
}

func (m *mockStreamWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestStreamIsolation(t *testing.T) {
	r := NewStreamRegistry()

	r.Register("stream-a")
	r.Register("stream-b")

	connA := &mockStreamWriter{}
	connB := &mockStreamWriter{}

	_, _ = r.Attach("stream-a", connA)
	_, _ = r.Attach("stream-b", connB)

	closed := r.Close("stream-a")
	if closed != nil {
		_ = closed.Close()
	}

	if !connA.closed {
		t.Fatal("expected connA to be closed")
	}
	if connB.closed {
		t.Fatal("closing stream A closed stream B's connection")
	}
}

func TestPendingBytesRouting(t *testing.T) {
	r := NewStreamRegistry()

	r.Register("stream-a")
	r.Register("stream-b")

	connA := &mockStreamWriter{}
	r.Attach("stream-a", connA)

	w, sent := r.Send("stream-b", []byte("hello stream b"))
	if sent {
		_ = w.WriteMessage(0, []byte("hello stream b"))
	}

	connB := &mockStreamWriter{}
	queued, _ := r.Attach("stream-b", connB)
	for _, msg := range queued {
		_ = connB.WriteMessage(0, msg)
	}

	if len(connA.messages) != 0 {
		t.Fatalf("expected 0 messages on connA, got %d", len(connA.messages))
	}
	if len(connB.messages) != 1 || !bytes.Equal(connB.messages[0], []byte("hello stream b")) {
		t.Fatalf("expected message on connB, got %+v", connB.messages)
	}
}

func TestClosedStreamAttachAndUnknownSend(t *testing.T) {
	r := NewStreamRegistry()

	r.Register("stream-a")
	r.Close("stream-a")

	connA := &mockStreamWriter{}
	if _, ok := r.Attach("stream-a", connA); ok {
		t.Fatal("expected Attach to return false for closed stream")
	}
	if len(r.slots) != 0 {
		t.Fatalf("expected registry to hold no slots, got %d", len(r.slots))
	}

	_, sent := r.Send("unknown", []byte("hello"))
	if sent {
		t.Fatal("expected Send to return false for unknown stream")
	}
	if len(r.slots) != 0 {
		t.Fatalf("expected registry to hold no slots after send to unknown id, got %d", len(r.slots))
	}
}
