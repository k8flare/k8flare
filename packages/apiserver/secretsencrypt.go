package apiserver

import (
	"encoding/json"
	"net/http"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func installSecretsEncrypt(mux *http.ServeMux, client *kine.Client) {
	mux.HandleFunc("GET /internal/secrets-encrypt/status", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		status, err := client.SecretsStatus(r.Context())
		writeJSON(w, status, err)
	}))
	mux.HandleFunc("POST /internal/secrets-encrypt/reencrypt", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		rewritten, err := client.ReencryptSecrets(r.Context())
		writeJSON(w, map[string]int{"rewritten": rewritten}, err)
	}))
}

func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, ok := genericapirequest.UserFrom(r.Context()); ok {
			for _, g := range u.GetGroups() {
				if g == user.SystemPrivilegedGroup {
					next(w, r)
					return
				}
			}
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}
}

func writeJSON(w http.ResponseWriter, body any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
