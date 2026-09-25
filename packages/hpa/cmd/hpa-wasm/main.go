//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"

	"github.com/k8flare/k8flare/packages/hpa"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale"
	metricsclient "k8s.io/kubernetes/pkg/controller/podautoscaler/metrics"
	customclient "k8s.io/metrics/pkg/client/custom_metrics"
	externalclient "k8s.io/metrics/pkg/client/external_metrics"
	metricsversioned "k8s.io/metrics/pkg/client/clientset/versioned"
)

func main() {
	cfg := &rest.Config{
		Host:            "https://k8flare.internal",
		BearerToken:     bridge.Getenv("ADMIN_TOKEN"),
		QPS:             20,
		Burst:           30,
		Transport:       bridge.BindingTransport{Name: "APISERVER"},
		ContentConfig:   rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"},
	}
	client, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "horizontal-pod-autoscaler"))
	if err != nil {
		panic(err)
	}
	mapper := hpa.RESTMapper()
	scales, err := scale.NewForConfig(cfg, mapper, dynamic.LegacyAPIPathResolverFunc, scale.NewDiscoveryScaleKindResolver(client.Discovery()))
	if err != nil {
		panic(err)
	}
	metricsCS, err := metricsversioned.NewForConfig(rest.AddUserAgent(cfg, "horizontal-pod-autoscaler"))
	if err != nil {
		panic(err)
	}
	custom := customclient.NewForConfig(cfg, mapper, customclient.NewAvailableAPIsGetter(client.Discovery()))
	external, err := externalclient.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	metrics := metricsclient.NewRESTMetricsClient(metricsCS.MetricsV1beta1(), custom, external)
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := hpa.Sync(r.Context(), client, scales, metrics, mapper)
		if err != nil {
			println("hpa: sync failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
}
