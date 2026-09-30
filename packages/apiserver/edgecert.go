package apiserver

import (
	"encoding/json"
	"net/http"
	"time"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
)

const defaultEdgeCertificateTTL = 365 * 24 * time.Hour

type edgeCertificateRequest struct {
	Hosts      []string `json:"hosts"`
	TTLSeconds int64    `json:"ttlSeconds"`
}

func installEdgeCertificate(mux *http.ServeMux, v *supervisor.Vault) {
	mux.HandleFunc("POST /internal/edge-certificate", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		var req edgeCertificateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Hosts) == 0 || req.TTLSeconds < 0 {
			http.Error(w, "hosts are required and ttlSeconds must not be negative", http.StatusBadRequest)
			return
		}
		ttl := defaultEdgeCertificateTTL
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}
		issued, err := v.IssueEdgeCertificate(r.Context(), req.Hosts, ttl)
		writeTokenJSON(w, http.StatusOK, issued, err)
	}))
}
