package apiserver

import (
	"net/http"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
)

func installCertificates(mux *http.ServeMux, v *supervisor.Vault) {
	mux.HandleFunc("POST /internal/certificate/rotate-ca", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		items, err := v.RotateCAs(r.Context())
		writeTokenJSON(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.HandleFunc("GET /internal/certificate/check", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		items, err := v.CAStatuses(r.Context())
		writeTokenJSON(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
}
