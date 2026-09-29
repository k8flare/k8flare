//go:build js && wasm

package bridge

import (
	"errors"
	"io"
	"net/url"
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

func TestAPIServerBodiesGetAShorterDeadlineThanTunnelBodies(t *testing.T) {
	cases := []struct {
		url  string
		want time.Duration
	}{
		{"https://k8flare.internal/api/v1/namespaces/ns/configmaps", apiServerBodyTimeout},
		{"https://k8flare.internal/api/v1/namespaces/ns/configmaps?watch=true", 0},
		{"https://nodetunnel.internal/node/n1/metrics/resource", unaryBodyTimeout},
		{"https://openapi.internal/openapi/v3", 0},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.url)
		if _, got := fetchTimeouts(u); got != c.want {
			t.Errorf("%s: body timeout %v, want %v", c.url, got, c.want)
		}
	}
}
