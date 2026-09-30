package apiserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
)

type createTokenRequest struct {
	Description string `json:"description"`
	TTLSeconds  int64  `json:"ttlSeconds"`
}

type issuedToken struct {
	supervisor.JoinToken
	Token string `json:"token"`
}

func installTokens(mux *http.ServeMux, v *supervisor.Vault) {
	mux.HandleFunc("POST /internal/tokens", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		var req createTokenRequest
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if req.TTLSeconds < 0 {
			http.Error(w, "ttlSeconds must not be negative", http.StatusBadRequest)
			return
		}
		created, token, err := v.CreateJoinToken(r.Context(), req.Description, time.Duration(req.TTLSeconds)*time.Second)
		writeTokenJSON(w, http.StatusCreated, issuedToken{created, token}, err)
	}))
	mux.HandleFunc("GET /internal/tokens", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		items, err := v.ListJoinTokens(r.Context())
		writeTokenJSON(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.HandleFunc("DELETE /internal/tokens/{id}", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		err := v.DeleteJoinToken(r.Context(), r.PathValue("id"))
		writeTokenJSON(w, http.StatusOK, map[string]string{"deleted": r.PathValue("id")}, err)
	}))
	mux.HandleFunc("POST /internal/tokens/{id}/rotate", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		rotated, token, err := v.RotateJoinToken(r.Context(), r.PathValue("id"))
		writeTokenJSON(w, http.StatusOK, issuedToken{rotated, token}, err)
	}))
}

func writeTokenJSON(w http.ResponseWriter, status int, body any, err error) {
	if errors.Is(err, supervisor.ErrJoinTokenNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
