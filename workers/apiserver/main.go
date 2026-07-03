//go:build js && wasm

package main

import (
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8flare/k8flare/pkg/apiserver"
	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare"
)

// getToken returns the K3S_TOKEN from Workers env binding.
// Must be called during request handling (not at init time).
var cachedToken string

func getToken() string {
	if cachedToken != "" {
		return cachedToken
	}
	t := cloudflare.Getenv("K3S_TOKEN")
	if t == "" {
		t = "k8flare-dev-token" // fallback for dev
	}
	cachedToken = t
	return t
}

func main() {
	mux := http.NewServeMux()

	doFetch := func(req *http.Request) (*http.Response, error) {
		ns, err := cloudflare.NewDurableObjectNamespace("CLUSTER")
		if err != nil {
			return nil, err
		}
		id := ns.IdFromName("default")
		stub, err := ns.Get(id)
		if err != nil {
			return nil, err
		}
		return stub.Fetch(req)
	}

	storage := apiserver.NewStorage(doFetch, "/registry")

	// One ResourceStore map per GroupVersion in apidef.Table (replaces what
	// used to be 10 separate hand-written NewXStores calls, one per API
	// group), plus the union of every namespaced store across all of them
	// for namespace cascading delete.
	storesByGV := make(map[schema.GroupVersion]map[string]*apiserver.ResourceStore, len(apidef.GroupVersions()))
	allStoreMaps := make([]map[string]*apiserver.ResourceStore, 0, len(apidef.GroupVersions()))
	for _, gv := range apidef.GroupVersions() {
		stores := apiserver.NewResourceStoresForGroupVersion(storage, gv)
		storesByGV[gv] = stores
		allStoreMaps = append(allStoreMaps, stores)
	}
	namespacedStores := apiserver.NamespacedResourceStores(allStoreMaps...)

	// CA Manager for supervisor protocol
	cam := apiserver.NewCAManager(storage)

	// Discovery (no auth)
	apiserver.RegisterDiscovery(mux)
	apiserver.RegisterGroupDiscovery(mux)
	apiserver.RegisterOpenAPIDiscovery(mux)

	// Supervisor endpoints (/cacerts, /v1-k3s/*)
	apiserver.RegisterSupervisorHandlers(mux, cam, storage, getToken)

	// Internal endpoints, service-binding-only (/internal/*)
	apiserver.RegisterInternalHandlers(mux, storage)

	// One auth-wrapped route per GroupVersion in apidef.Table. core/v1
	// additionally bootstraps the cluster's baseline namespaces/
	// ServiceAccounts on first request and sweeps dependents on Namespace
	// delete (namespacedStores is nil for every other group, since
	// "namespaces" never exists as a key in a non-core stores map --
	// HandleResource's doc comment in pkg/apiserver/handler.go explains why
	// that alone is enough to make the cascading-delete branch a no-op
	// there).
	for _, gv := range apidef.GroupVersions() {
		prefix := apidef.APIPrefix(gv)
		stores := storesByGV[gv]
		isCore := gv == corev1.SchemeGroupVersion

		mux.Handle(prefix, apiserver.AuthMiddleware(getToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isCore {
				apiserver.BootstrapCluster(r.Context(), stores)
				apiserver.HandleResource(w, r, prefix, stores, namespacedStores)
				return
			}
			apiserver.HandleResource(w, r, prefix, stores, nil)
		})))
	}

	workers.Serve(mux)
}
