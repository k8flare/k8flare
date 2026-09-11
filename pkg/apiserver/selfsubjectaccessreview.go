package apiserver

import (
	"context"
	"io"
	"net/http"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

// RegisterAuthorizationHandlers registers POST
// /apis/authorization.k8s.io/v1/selfsubjectaccessreviews -- the request
// `kubectl auth can-i` sends -- and subjectaccessreviews (what the
// kubelet's Webhook authorizer POSTs after its webhook token
// authenticator accepted a request, see tokenreview.go), behind the same
// AuthMiddleware every other API route uses.
//
// Both answer from the SAME real RBACAuthorizer (rbac.go) that gates
// live traffic, so `kubectl auth can-i` and enforcement can never
// disagree. The history here: until RBAC enforcement landed these
// handlers hardcoded Allowed:true, which was the accurate answer for the
// then all-or-nothing token model -- kept in git history, replaced (not
// papered over) the day the real authorizer arrived.
func RegisterAuthorizationHandlers(mux *http.ServeMux, tokensFn TokensFunc, authz authorizer.Authorizer) {
	mux.Handle("POST /apis/authorization.k8s.io/v1/selfsubjectaccessreviews",
		AuthMiddleware(tokensFn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleSelfSubjectAccessReview(w, r, authz)
		})))
	mux.Handle("POST /apis/authorization.k8s.io/v1/subjectaccessreviews",
		AuthMiddleware(tokensFn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleSubjectAccessReview(w, r, authz)
		})))
}

// evaluateReviewSpec runs one SubjectAccessReviewSpec-shaped question
// through the authorizer for the given identity. The system:masters
// bypass mirrors authorizeRequest (rbac.go).
func evaluateReviewSpec(ctx context.Context, authz authorizer.Authorizer, info *user.DefaultInfo,
	res *authorizationv1.ResourceAttributes, nonRes *authorizationv1.NonResourceAttributes,
) authorizationv1.SubjectAccessReviewStatus {
	for _, g := range info.Groups {
		if g == user.SystemPrivilegedGroup {
			return authorizationv1.SubjectAccessReviewStatus{Allowed: true, Reason: "system:masters bypass"}
		}
	}
	attrs := authorizer.AttributesRecord{User: info}
	if res != nil {
		attrs.Verb = res.Verb
		attrs.Namespace = res.Namespace
		attrs.APIGroup = res.Group
		attrs.APIVersion = res.Version
		attrs.Resource = res.Resource
		attrs.Subresource = res.Subresource
		attrs.Name = res.Name
		attrs.ResourceRequest = true
	} else if nonRes != nil {
		attrs.Verb = nonRes.Verb
		attrs.Path = nonRes.Path
	} else {
		return authorizationv1.SubjectAccessReviewStatus{
			Allowed: false,
			Reason:  "spec.resourceAttributes or spec.nonResourceAttributes is required",
		}
	}
	decision, reason, err := authz.Authorize(ctx, attrs)
	if err != nil {
		return authorizationv1.SubjectAccessReviewStatus{Allowed: false, Reason: reason, EvaluationError: err.Error()}
	}
	return authorizationv1.SubjectAccessReviewStatus{Allowed: decision == authorizer.DecisionAllow, Reason: reason}
}

func handleSelfSubjectAccessReview(w http.ResponseWriter, r *http.Request, authz authorizer.Authorizer) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
		return
	}
	defer r.Body.Close()

	obj, err := decodeBody(body, nil)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
		return
	}
	ssar, ok := obj.(*authorizationv1.SelfSubjectAccessReview)
	if !ok {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "request body is not a SelfSubjectAccessReview")
		return
	}

	u := UserFromContext(r.Context())
	if u == nil {
		writeStatusError(w, http.StatusUnauthorized, "Unauthorized", "no authenticated user")
		return
	}
	info := &user.DefaultInfo{Name: u.Name, Groups: u.Groups}
	ssar.Status = evaluateReviewSpec(r.Context(), authz, info, ssar.Spec.ResourceAttributes, ssar.Spec.NonResourceAttributes)
	writeRuntimeObject(w, http.StatusCreated, ssar)
}

func handleSubjectAccessReview(w http.ResponseWriter, r *http.Request, authz authorizer.Authorizer) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
		return
	}
	defer r.Body.Close()

	obj, err := decodeBody(body, nil)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
		return
	}
	sar, ok := obj.(*authorizationv1.SubjectAccessReview)
	if !ok {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", "request body is not a SubjectAccessReview")
		return
	}

	info := &user.DefaultInfo{Name: sar.Spec.User, Groups: sar.Spec.Groups}
	sar.Status = evaluateReviewSpec(r.Context(), authz, info, sar.Spec.ResourceAttributes, sar.Spec.NonResourceAttributes)
	writeRuntimeObject(w, http.StatusCreated, sar)
}
