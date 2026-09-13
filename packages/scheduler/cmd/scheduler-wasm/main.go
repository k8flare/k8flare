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
	pokeWindow  = 20 * time.Second
	idleChecks  = 3
	checkPeriod = 500 * time.Millisecond
)

func main() {
	cfg := &rest.Config{
		Host:        "https://k8flare.internal",
		BearerToken: bridge.Getenv("ADMIN_TOKEN"),
		Transport:   bridge.BindingTransport{Name: "APISERVER"},
	}
	var (
		once  sync.Once
		sched *scheduler.Scheduler
		err   error
	)
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ctx := context.Background()
			if sched, err = scheduler.New(ctx, cfg); err == nil {
				go sched.Run(ctx)
			}
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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
