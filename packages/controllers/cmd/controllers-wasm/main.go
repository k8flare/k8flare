//go:build js && wasm

package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/k8flare/k8flare/packages/controllers"
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
		QPS:         20,
		Burst:       30,
		Transport:   bridge.BindingTransport{Name: "APISERVER", AbortOnWake: true},
	}
	var (
		mu      sync.Mutex
		holding bool
		ctrl    *controllers.Controllers
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
		if ctrl == nil {
			started, err := controllers.New(context.Background(), cfg)
			if err != nil {
				println("controllers: start failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ctrl = started
			go ctrl.Run(context.Background())
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
			if ctrl.Idle() {
				idle++
			} else {
				idle = 0
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
