//go:build js && wasm

package main

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/k8flare/k8flare/packages/scheduler"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/rest"
)

const (
	pokeWindow = 20 * time.Second
	maxWindow  = 5 * time.Minute
)

func windowFrom(r *http.Request) time.Duration {
	ms, err := strconv.Atoi(r.URL.Query().Get("window"))
	if err != nil || ms <= 0 {
		return pokeWindow
	}
	if d := time.Duration(ms) * time.Millisecond; d < maxWindow {
		return d
	}
	return maxWindow
}

func main() {
	cfg := &rest.Config{
		Host:        "https://k8flare.internal",
		BearerToken: bridge.Getenv("ADMIN_TOKEN"),
		QPS:         50,
		Burst:       100,
		Transport:   bridge.BindingTransport{Name: "APISERVER", AbortOnWake: true},
	}
	var (
		mu      sync.Mutex
		holding bool
		sched   *scheduler.Scheduler
	)
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if holding {
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		holding = true
		mu.Unlock()
		defer func() {
			mu.Lock()
			holding = false
			mu.Unlock()
		}()
		if sched == nil {
			started, err := scheduler.New(context.Background(), cfg)
			if err != nil {
				println("scheduler: start failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			sched = started
			go sched.Run(context.Background())
		}
		bridge.OpenWindow()
		defer bridge.CloseWindow()
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-time.After(windowFrom(r)):
		case <-r.Context().Done():
		}
	}))
}
