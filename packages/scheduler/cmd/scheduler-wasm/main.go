//go:build js && wasm

package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/k8flare/k8flare/packages/scheduler"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/rest"
)

const (
	pokeWindow    = 20 * time.Second
	idleChecks    = 3
	checkPeriod   = 500 * time.Millisecond
	watchLifetime = 15 * time.Second
)

func main() {
	cfg := &rest.Config{
		Host:        "https://k8flare.internal",
		BearerToken: bridge.Getenv("ADMIN_TOKEN"),
		Transport:   bridge.BindingTransport{Name: "APISERVER", WatchLifetime: watchLifetime},
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
		deadline := time.After(pokeWindow)
		idle := 0
		for idle < idleChecks {
			select {
			case <-deadline:
				w.WriteHeader(http.StatusAccepted)
				return
			case <-r.Context().Done():
				return
			case <-time.After(checkPeriod):
			}
			if sched.Idle() {
				idle++
			} else {
				idle = 0
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
