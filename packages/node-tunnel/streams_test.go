package nodetunnel

import (
	"bytes"
	"sync"
	"sync/atomic"
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

	r.Attach("stream-a", connA)
	r.Attach("stream-b", connB)

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

	_ = r.Send("stream-b", []byte("hello stream b"))

	connB := &mockStreamWriter{}
	r.Attach("stream-b", connB)

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
	if r.Attach("stream-a", connA) {
		t.Fatal("expected Attach to return false for closed stream")
	}
	if len(r.slots) != 0 {
		t.Fatalf("expected registry to hold no slots, got %d", len(r.slots))
	}

	err := r.Send("unknown", []byte("hello"))
	if err == nil {
		t.Fatal("expected Send to return error for unknown stream")
	}
	if len(r.slots) != 0 {
		t.Fatalf("expected registry to hold no slots after send to unknown id, got %d", len(r.slots))
	}
}

type blockingWriter struct {
	mu         sync.Mutex
	writes     int
	concurrent int32
	overlapped bool
	firstBlock chan struct{}
	release    chan struct{}
	messages   [][]byte
}

func (w *blockingWriter) WriteMessage(messageType int, data []byte) error {
	if atomic.AddInt32(&w.concurrent, 1) > 1 {
		w.mu.Lock()
		w.overlapped = true
		w.mu.Unlock()
	}
	defer atomic.AddInt32(&w.concurrent, -1)

	w.mu.Lock()
	w.writes++
	w.messages = append(w.messages, append([]byte(nil), data...))
	isFirst := w.writes == 1
	w.mu.Unlock()

	if isFirst {
		close(w.firstBlock)
		<-w.release
	}
	return nil
}

func (w *blockingWriter) Close() error {
	return nil
}

func TestStreamAttachOrder(t *testing.T) {
	r := NewStreamRegistry()
	r.Register("stream-1")

	_ = r.Send("stream-1", []byte("frame-1"))
	_ = r.Send("stream-1", []byte("frame-2"))

	w := &blockingWriter{
		firstBlock: make(chan struct{}),
		release:    make(chan struct{}),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Attach("stream-1", w)
	}()

	<-w.firstBlock
	sendDone := make(chan struct{})
	go func() {
		_ = r.Send("stream-1", []byte("frame-3"))
		close(sendDone)
	}()

	close(w.release)
	<-sendDone
	wg.Wait()

	if w.overlapped {
		t.Fatal("writes overlapped concurrently")
	}
	if len(w.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(w.messages))
	}
	expected := [][]byte{[]byte("frame-1"), []byte("frame-2"), []byte("frame-3")}
	for i, exp := range expected {
		if !bytes.Equal(w.messages[i], exp) {
			t.Fatalf("message %d: expected %s, got %s", i, exp, w.messages[i])
		}
	}
}
