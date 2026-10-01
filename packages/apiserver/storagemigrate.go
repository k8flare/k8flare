package apiserver

import (
	"net/http"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
)

func installStorageMigrate(mux *http.ServeMux, client *kine.Client) {
	mux.HandleFunc("GET /internal/storage-migrate/status", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		resources, err := registry.StoredResources()
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		status, err := client.StorageVersionStatus(r.Context(), resources)
		writeJSON(w, status, err)
	}))
	mux.HandleFunc("POST /internal/storage-migrate", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		resources, err := registry.StoredResources()
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		status, err := client.MigrateStorage(r.Context(), resources, kine.MigrationPassBudget)
		writeJSON(w, status, err)
	}))
}
