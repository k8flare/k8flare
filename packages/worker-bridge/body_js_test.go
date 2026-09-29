//go:build js && wasm

package bridge

import (
	"errors"
	"io"
	"syscall/js"
	"testing"
	"time"
)

func TestStreamBodyGivesUpOnABodyThatNeverFinishes(t *testing.T) {
	stalled := js.Global().Get("ReadableStream").New(js.ValueOf(map[string]any{}))
	body, _ := streamBody(stalled, func() {}, 200*time.Millisecond)
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(body)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrBodyTimeout) {
			t.Fatalf("got %v, want ErrBodyTimeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading a body that never finishes did not return")
	}
}

func TestStreamBodyReadsACompleteBody(t *testing.T) {
	complete := js.Global().Get("Response").New("payload").Get("body")
	body, _ := streamBody(complete, func() {}, time.Second)
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "payload" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestStreamBodyWithoutDeadlineKeepsWaiting(t *testing.T) {
	stalled := js.Global().Get("ReadableStream").New(js.ValueOf(map[string]any{}))
	body, _ := streamBody(stalled, func() {}, 0)
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(body)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("a streaming body without a deadline ended early: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
}
