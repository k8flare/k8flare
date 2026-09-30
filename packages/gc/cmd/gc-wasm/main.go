//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"github.com/k8flare/k8flare/packages/gc"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	cfg := &rest.Config{
		Host:          "https://k8flare.internal",
		BearerToken:   bridge.Getenv("API_TOKEN"),
		QPS:           20,
		Burst:         30,
		Transport:     bridge.BindingTransport{Name: "APISERVER"},
		ContentConfig: rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"},
	}
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kube-controller-manager"))
	if err != nil {
		panic(err)
	}
	dyn, err := dynamic.NewForConfig(rest.AddUserAgent(cfg, "kube-controller-manager"))
	if err != nil {
		panic(err)
	}
	store := &kine.Client{HTTP: &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}}}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := gc.Collect(r.Context(), client, dyn, store)
		if err != nil {
			println("gc: collect failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
}
