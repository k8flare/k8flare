//go:build js && wasm

package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/k8flare/k8flare/packages/scheduler"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	cfg := &rest.Config{
		Host:          "https://k8flare.internal",
		BearerToken:   bridge.Getenv("API_TOKEN"),
		QPS:           50,
		Burst:         100,
		Transport:     bridge.BindingTransport{Name: "APISERVER"},
		ContentConfig: rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"},
	}
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kube-scheduler"))
	if err != nil {
		panic(err)
	}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []scheduler.QueueMessage `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		attempt, ok := scheduler.QueueAttempt(req.Messages)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		result, err := scheduler.Schedule(r.Context(), client)
		if err != nil {
			println("scheduler: schedule failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result.Attempt = attempt
		result.RetryAfterS = scheduler.RetryDelaySeconds(attempt, len(result.Unschedulable))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
}
