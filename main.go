//go:build js && wasm

package main

import (
	"net/http"

	"github.com/k8flare/k8flare/pkg/apiserver"
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
		ns, err := cloudflare.NewDurableObjectNamespace("ETCD")
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
	stores := apiserver.NewResourceStores(storage)
	leaseStores := apiserver.NewLeaseStores(storage)
	storageStores := apiserver.NewStorageStores(storage)
	nodeAPIStores := apiserver.NewNodeAPIStores(storage)
	resourceAPIStores := apiserver.NewResourceAPIStores(storage)
	namespacedStores := apiserver.NamespacedResourceStores(stores, leaseStores, storageStores, nodeAPIStores, resourceAPIStores)

	// CA Manager for supervisor protocol
	cam := apiserver.NewCAManager(storage)

	// Discovery (no auth)
	apiserver.RegisterDiscovery(mux, apiserver.DefaultResources())
	apiserver.RegisterGroupDiscovery(mux)

	// Supervisor endpoints (/cacerts, /v1-k3s/*)
	apiserver.RegisterSupervisorHandlers(mux, cam, storage, getToken)

	// Core API v1 (with auth)
	mux.Handle("/api/v1/", apiserver.AuthMiddleware(getToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiserver.BootstrapCluster(r.Context(), stores)
		apiserver.HandleAPI(w, r, stores, namespacedStores)
	})))

	// coordination.k8s.io/v1 (with auth)
	mux.Handle("/apis/coordination.k8s.io/v1/", apiserver.AuthMiddleware(getToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiserver.HandleGroupAPI(w, r, leaseStores)
	})))

	// storage.k8s.io/v1 (with auth)
	mux.Handle("/apis/storage.k8s.io/v1/", apiserver.AuthMiddleware(getToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiserver.HandleGroupAPI(w, r, storageStores)
	})))

	// node.k8s.io/v1 (with auth)
	mux.Handle("/apis/node.k8s.io/v1/", apiserver.AuthMiddleware(getToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiserver.HandleGroupAPI(w, r, nodeAPIStores)
	})))

	// resource.k8s.io/v1 (with auth)
	mux.Handle("/apis/resource.k8s.io/v1/", apiserver.AuthMiddleware(getToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiserver.HandleGroupAPI(w, r, resourceAPIStores)
	})))

	workers.Serve(mux)
}
