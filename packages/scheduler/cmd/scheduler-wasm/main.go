//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"

	"github.com/k8flare/k8flare/packages/scheduler"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	cfg := &rest.Config{
		Host:        "https://k8flare.internal",
		BearerToken: bridge.Getenv("ADMIN_TOKEN"),
		QPS:         50,
		Burst:       100,
		Transport:   bridge.BindingTransport{Name: "APISERVER"},
	}
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kube-scheduler"))
	if err != nil {
		panic(err)
	}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := scheduler.Schedule(r.Context(), client)
		if err != nil {
			println("scheduler: schedule failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
}
