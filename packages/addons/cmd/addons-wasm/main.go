//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/k8flare/k8flare/packages/addons"
	"github.com/k8flare/k8flare/packages/helm"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

func main() {
	cfg := &rest.Config{
		Host:          "https://k8flare.internal",
		BearerToken:   bridge.Getenv("API_TOKEN"),
		QPS:           1000,
		Burst:         2000,
		Transport:     bridge.BindingTransport{Name: "APISERVER"},
		ContentConfig: rest.ContentConfig{AcceptContentTypes: "application/json", ContentType: "application/json"},
	}
	cfg = rest.AddUserAgent(cfg, "deploy")
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		panic(err)
	}
	helmController := &helm.Controller{
		Client: client,
		Mapper: func() (meta.RESTMapper, error) {
			groups, err := restmapper.GetAPIGroupResources(disco)
			if err != nil {
				return nil, err
			}
			return restmapper.NewDiscoveryRESTMapper(groups), nil
		},
		HTTP:         http.DefaultClient,
		Capabilities: func() (*chartutil.Capabilities, error) { return helm.DiscoveredCapabilities(disco) },
		Lookup:       cfg,
		Wait:         time.Second,
	}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/helm" {
			if addons.ParseDisable(bridge.Getenv("DISABLE"))["helm-controller"] {
				w.Write([]byte("{}"))
				return
			}
			if err := helmController.Reconcile(r.Context()); err != nil {
				println("helm: reconcile failed:", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{}"))
			return
		}
		groups, err := restmapper.GetAPIGroupResources(disco)
		if err != nil {
			println("addons: discovery failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var extra map[string]string
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&extra)
		}
		files := addons.Render(addons.Packaged(), addons.Vars())
		for name, content := range extra {
			files = append(files, addons.File{Name: name, Content: []byte(content)})
		}
		deployer := &addons.Deployer{Client: client, Mapper: restmapper.NewDiscoveryRESTMapper(groups)}
		if err := deployer.Deploy(r.Context(), files, addons.ParseDisable(bridge.Getenv("DISABLE"))); err != nil {
			println("addons: deploy failed:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	}))
}
