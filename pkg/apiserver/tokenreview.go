package apiserver

import (
	"io"
	"net/http"

	authenticationv1 "k8s.io/api/authentication/v1"
)

// RegisterAuthenticationHandlers registers POST
// /apis/authentication.k8s.io/v1/tokenreviews behind the same
// AuthMiddleware every other API route uses.
//
// The consumer is the kubelet's webhook token authenticator: per-Pod
// microVM nodes (workers/nodes) keep the stock k3s kubelet security
// config (anonymous disabled, authentication.webhook + authorization
// Webhook), so when the gateway's logs/metrics bridge presents the
// cluster bearer token to the kubelet, the kubelet TokenReviews it here
// -- exactly how a real cluster authenticates apiserver->kubelet
// traffic that uses tokens. Observed live 2026-07-06: the kubelet POSTs
// this endpoint on the first bridged request and caches the verdict
// (default 2m), so per-request overhead stays negligible.
//
// The identity minted for the (single, all-powerful) cluster token
// matches AuthMiddleware's bearer identity so both auth paths agree on
// who the token is.
func RegisterAuthenticationHandlers(mux *http.ServeMux, tokenFn TokenFunc) {
	mux.Handle("POST /apis/authentication.k8s.io/v1/tokenreviews",
		AuthMiddleware(tokenFn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleTokenReview(w, r, tokenFn)
		})))
}

func handleTokenReview(w http.ResponseWriter, r *http.Request, tokenFn TokenFunc) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
		return
	}
	defer r.Body.Close()

	obj, err := decodeBody(body)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
		return
	}
	tr, ok := obj.(*authenticationv1.TokenReview)
	if !ok {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "request body is not a TokenReview")
		return
	}

	if tr.Spec.Token == tokenFn() {
		tr.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User: authenticationv1.UserInfo{
				Username: "admin",
				Groups:   []string{"system:masters", "system:authenticated"},
			},
		}
	} else {
		tr.Status = authenticationv1.TokenReviewStatus{
			Authenticated: false,
			Error:         "token is not the cluster token",
		}
	}
	writeRuntimeObject(w, http.StatusCreated, tr)
}
