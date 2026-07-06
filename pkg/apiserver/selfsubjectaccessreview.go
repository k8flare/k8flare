package apiserver

import (
	"io"
	"net/http"

	authorizationv1 "k8s.io/api/authorization/v1"
)

// RegisterAuthorizationHandlers registers POST
// /apis/authorization.k8s.io/v1/selfsubjectaccessreviews -- the request
// `kubectl auth can-i` sends -- behind the same AuthMiddleware every other
// API route uses.
//
// This project's authorization is still all-or-nothing per bearer token
// (see auth.go's AuthMiddleware: a request either has a valid token, in
// which case it can do anything, or it's rejected before reaching here at
// all) -- there is no per-verb/per-resource authorizer to consult. So
// handleSelfSubjectAccessReview always answers Allowed: true, which is the
// accurate answer for this project's actual access model, not a stub
// standing in for unimplemented enforcement. This unblocks `kubectl auth
// can-i` (and anything else that gates a codepath on a SelfSubjectAccessReview
// first) from failing outright, matching CLAUDE.md's "cost-sensitive/behavior
// changes get recorded, not silently patched over" spirit: the day this
// project gets real per-subject RBAC enforcement, this handler is exactly
// where that decision needs to be wired in.
func RegisterAuthorizationHandlers(mux *http.ServeMux, tokenFn TokenFunc) {
	mux.Handle("POST /apis/authorization.k8s.io/v1/selfsubjectaccessreviews",
		AuthMiddleware(tokenFn, http.HandlerFunc(handleSelfSubjectAccessReview)))
	// subjectaccessreviews: the kubelet's Webhook authorizer POSTs this
	// after its webhook token authenticator accepted a request (see
	// tokenreview.go). Same all-or-nothing model as the SSAR above -- the
	// only identity TokenReview ever authenticates is the cluster token's
	// "admin" (system:masters), so Allowed:true is the accurate verdict,
	// and this handler is where a real RBAC authorizer would plug in.
	mux.Handle("POST /apis/authorization.k8s.io/v1/subjectaccessreviews",
		AuthMiddleware(tokenFn, http.HandlerFunc(handleSubjectAccessReview)))
}

func handleSelfSubjectAccessReview(w http.ResponseWriter, r *http.Request) {
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
	ssar, ok := obj.(*authorizationv1.SelfSubjectAccessReview)
	if !ok {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "request body is not a SelfSubjectAccessReview")
		return
	}

	ssar.Status = authorizationv1.SubjectAccessReviewStatus{
		Allowed: true,
		Reason:  "k8flare authorizes every request from a valid bearer token (all-or-nothing token model, no per-subject RBAC enforcement yet)",
	}
	writeRuntimeObject(w, http.StatusCreated, ssar)
}

func handleSubjectAccessReview(w http.ResponseWriter, r *http.Request) {
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
	sar, ok := obj.(*authorizationv1.SubjectAccessReview)
	if !ok {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "request body is not a SubjectAccessReview")
		return
	}

	sar.Status = authorizationv1.SubjectAccessReviewStatus{
		Allowed: true,
		Reason:  "k8flare authorizes every request from a valid bearer token (all-or-nothing token model, no per-subject RBAC enforcement yet)",
	}
	writeRuntimeObject(w, http.StatusCreated, sar)
}
