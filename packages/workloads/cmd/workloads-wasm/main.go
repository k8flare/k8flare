//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"

	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"github.com/k8flare/k8flare/packages/workloads"
	crdclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
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
	crdClient, err := crdclientset.NewForConfig(rest.AddUserAgent(cfg, "kube-apiserver-apiextensions"))
	if err != nil {
		panic(err)
	}
	var rootCA []byte
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
		if r.URL.Path == "/crds" {
			crds, err := workloads.SyncCRDs(r.Context(), crdClient)
			if err != nil {
				println("workloads: crd sync failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(crds)
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
