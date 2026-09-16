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

const pokeWindow = 20 * time.Second

func main() {
	cfg := &rest.Config{
		Host:        "https://k8flare.internal",
		BearerToken: bridge.Getenv("ADMIN_TOKEN"),
		QPS:         50,
		Burst:       100,
		Transport:   bridge.BindingTransport{Name: "APISERVER", AbortOnWake: true},
	}
	var (
		mu          sync.Mutex
		pokeHolding bool
		runHolding  bool
		sched       *scheduler.Scheduler
		pacer       bridge.Pacer
	)
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridge.Poked()
		hold := bridge.ParseHold(r, pokeWindow)
		mu.Lock()
		if runHolding || (hold.Reset && pokeHolding) {
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if hold.Reset {
			pokeHolding = true
		} else {
			runHolding = true
		}
		mu.Unlock()
		defer func() {
			mu.Lock()
			if hold.Reset {
				pokeHolding = false
			} else {
				runHolding = false
			}
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
		bridge.OpenWindow(r.Context())
		defer bridge.CloseWindow(r.Context())
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hold.Reset {
			pacer.Reset()
		}
		bridge.Hold(r.Context(), hold, sched.Idle)
		bridge.WriteNext(w, pacer.Next(sched.Pending()))
	}))
}
