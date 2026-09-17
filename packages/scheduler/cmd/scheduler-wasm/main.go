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
	pokeWindow    = 20 * time.Second
	handoverAfter = 200 * time.Second
	deadRunAfter  = 10 * time.Second
)

func main() {
	cfg := &rest.Config{
		Host:        "https://k8flare.internal",
		BearerToken: bridge.Getenv("ADMIN_TOKEN"),
		QPS:         50,
		Burst:       100,
		Transport:   bridge.BindingTransport{Name: "APISERVER", AbortOnWake: true},
	}
	var (
		mu           sync.Mutex
		runs         int
		lastRunStart time.Time
		sched        *scheduler.Scheduler
		pacer        bridge.Pacer
	)
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridge.Poked()
		hold := bridge.ParseHold(r, pokeWindow)
		if hold.Kick {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mu.Lock()
		if age := time.Since(lastRunStart); runs > 0 && age < handoverAfter && bridge.SinceTick() < deadRunAfter {
			mu.Unlock()
			retry := handoverAfter - age
			if dead := deadRunAfter - bridge.SinceTick(); dead < retry {
				retry = dead
			}
			w.Header().Set("X-Retry-After-Ms", strconv.FormatInt((retry+time.Second).Milliseconds(), 10))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		runs++
		lastRunStart = time.Now()
		bridge.MarkTick()
		mu.Unlock()
		defer func() {
			mu.Lock()
			runs--
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
		bridge.OpenRunWindow(r.Context())
		defer bridge.CloseWindow(r.Context())
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hold.Reset {
			pacer.Reset()
		}
		bridge.Hold(bridge.RunContext(r.Context()), hold, sched.Idle)
		if bridge.Superseded(r.Context()) {
			bridge.WriteNext(w, -time.Millisecond)
			return
		}
		bridge.WriteNext(w, pacer.Next(sched.Pending()))
	}))
}
