//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"github.com/k8flare/k8flare/packages/workloads"
	_ "github.com/k8flare/k8flare/packages/workloads/shards/vap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
)

func main() {
	cfg := &rest.Config{
		Host:          "https://k8flare.internal",
		BearerToken:   bridge.Getenv("ADMIN_TOKEN"),
		QPS:           1000,
		Burst:         2000,
		Transport:     bridge.BindingTransport{Name: "APISERVER"},
		ContentConfig: rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"},
	}
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kube-controller-manager"))
	if err != nil {
		panic(err)
	}
	metadataClient, err := metadata.NewForConfig(rest.AddUserAgent(cfg, "kube-controller-manager"))
	if err != nil {
		panic(err)
	}
	var rootCA, signingCA, servingCA []byte
	var deleter *workloads.Deleter
	store := &kine.Client{HTTP: &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}}}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/namespaces" {
			if deleter == nil {
				deleter = workloads.NewDeleter(r.Context(), client, metadataClient)
			}
			var req struct {
				Messages []workloads.NamespaceMessage `json:"messages"`
			}
			if r.Body != nil {
				_ = json.NewDecoder(r.Body).Decode(&req)
			}
			names := workloads.NamespaceNames(req.Messages)
			if len(names) == 0 && r.URL.Query().Get("names") != "" {
				names = strings.Split(r.URL.Query().Get("names"), ",")
			}
			result, err := deleter.DeleteTerminating(r.Context(), client, names)
			if err != nil {
				println("workloads: namespace delete failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(result.Names) == 0 {
				result.Names = names
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
		if v := bridge.Getenv("CLUSTER_SERVER"); v != "" {
			workloads.ClusterServer = v
		}
		if signingCA == nil {
			signingCA, _ = workloads.VaultPEM(r.Context(), store, "client-ca")
		}
		if servingCA == nil {
			servingCA, _ = workloads.VaultPEM(r.Context(), store, "server-ca")
		}
		if rootCA == nil {
			ca, err := client.CoreV1().RESTClient().Get().AbsPath("/cacerts").DoRaw(r.Context())
			if err != nil {
				println("workloads: cacerts failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rootCA = ca
		}
		changed := []string{}
		if raw := r.URL.Query().Get("changed"); raw != "" {
			changed = strings.Split(raw, ",")
		}
		result, err := workloads.SyncWithin(r.Context(), client, rootCA, signingCA, servingCA, changed, syncBudget(r), holdWindow)
		if err != nil {
			println("workloads: sync failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
}

func syncBudget(r *http.Request) time.Duration {
	ms, err := strconv.Atoi(r.URL.Query().Get("budgetMs"))
	if err != nil || ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func holdWindow(ctx context.Context) func() {
	bridge.OpenWindow(ctx)
	return func() { bridge.CloseWindow(ctx) }
}
