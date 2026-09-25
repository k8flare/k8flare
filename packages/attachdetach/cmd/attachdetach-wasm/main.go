//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"

	"github.com/k8flare/k8flare/packages/attachdetach"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	cfg := &rest.Config{
		Host:          "https://k8flare.internal",
		BearerToken:   bridge.Getenv("ADMIN_TOKEN"),
		QPS:           20,
		Burst:         30,
		Transport:     bridge.BindingTransport{Name: "APISERVER"},
		ContentConfig: rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"},
	}
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "attachdetach-controller"))
	if err != nil {
		panic(err)
	}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := attachdetach.Sync(r.Context(), client)
		if err != nil {
			println("attachdetach: sync failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
}
