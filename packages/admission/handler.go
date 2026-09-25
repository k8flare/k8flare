package admission

import (
	"encoding/json"
	"net/http"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/kubernetes/scheme"
)

type Config struct {
	Kine     *http.Client
	Tunnel   *http.Client
	Hooks    *http.Client
	Outbound *http.Client
	API      *http.Client
	Token    string
}

type Handler struct {
	store    *store
	tunnel   *http.Client
	hooks    *http.Client
	outbound *http.Client
	authz    authorizer.Authorizer
}

func NewHandler(cfg Config) http.Handler {
	_ = admissionv1.AddToScheme(scheme.Scheme)
	_ = corev1.AddToScheme(scheme.Scheme)
	_ = discoveryv1.AddToScheme(scheme.Scheme)
	h := &Handler{
		store:    &store{client: &kine.Client{HTTP: cfg.Kine}},
		tunnel:   cfg.Tunnel,
		hooks:    cfg.Hooks,
		outbound: cfg.Outbound,
	}
	if cfg.API != nil {
		h.authz = &sarAuthorizer{client: cfg.API, token: cfg.Token}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admit", h.admit)
	return mux
}

func (h *Handler) admit(w http.ResponseWriter, r *http.Request) {
	var req admit.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, err := h.run(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
