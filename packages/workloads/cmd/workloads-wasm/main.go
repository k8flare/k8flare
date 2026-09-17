//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"
	"strings"

	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"github.com/k8flare/k8flare/packages/workloads"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
)

func main() {
	cfg := &rest.Config{
		Host:        "https://k8flare.internal",
		BearerToken: bridge.Getenv("ADMIN_TOKEN"),
		QPS:         20,
		Burst:       30,
		Transport:   bridge.BindingTransport{Name: "APISERVER"},
	}
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kube-controller-manager"))
	if err != nil {
		panic(err)
	}
	metadataClient, err := metadata.NewForConfig(rest.AddUserAgent(cfg, "kube-controller-manager"))
	if err != nil {
		panic(err)
	}
	var rootCA []byte
	var deleter *workloads.Deleter
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rootCA == nil {
			ca, err := client.CoreV1().RESTClient().Get().AbsPath("/cacerts").DoRaw(r.Context())
			if err != nil {
				println("workloads: cacerts failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rootCA = ca
		}
		if r.URL.Path == "/namespaces" {
			if deleter == nil {
				deleter = workloads.NewDeleter(r.Context(), client, metadataClient)
			}
			names := strings.Split(r.URL.Query().Get("names"), ",")
			result, err := deleter.DeleteTerminating(r.Context(), client, names)
			if err != nil {
				println("workloads: namespace delete failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
			return
		}
		if node := r.URL.Query().Get("node"); r.URL.Path == "/nodehealth" && node != "" {
			health, err := workloads.NodeHealth(r.Context(), client, node)
			if err != nil {
				println("workloads: node health failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(health)
			return
		}
		result, err := workloads.Sync(r.Context(), client, rootCA)
		if err != nil {
			println("workloads: sync failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
}
