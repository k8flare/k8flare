//go:build js && wasm

package main

import (
	"encoding/json"
	"net/http"

	"github.com/k8flare/k8flare/packages/addons"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
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
	cfg = rest.AddUserAgent(cfg, "deploy")
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		panic(err)
	}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
