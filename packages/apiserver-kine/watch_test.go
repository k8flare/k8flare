package kine

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
)

func TestWatchReturnsWhenTheDialerKeepsFailing(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	attempts := 0
	WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
		attempts++
		return nil, nil, fmt.Errorf("dial refused")
	}
	s := NewStorage(nil, nil, func() runtime.Object { return &v1.Pod{} })
	done := make(chan error, 1)
	go func() {
		_, err := s.Watch(context.Background(), "/registry/pods/", storage.ListOptions{Recursive: true, ResourceVersion: "1"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Watch blocked instead of returning an error")
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d, want the redial retries", attempts)
	}
}
