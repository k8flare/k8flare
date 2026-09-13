package apiserver

import (
	"context"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
)

// reviewREST is a create-only virtual resource served through the API
// installer, so the review APIs the kubelet and the k3s agent call get the
// same content negotiation and discovery as everything else. Every
// authenticated caller is allowed everything for now.
type reviewREST struct {
	singular string
	newFunc  func() runtime.Object
	create   func(ctx context.Context, obj runtime.Object) runtime.Object
}

var _ rest.Creater = (*reviewREST)(nil)

func (r *reviewREST) New() runtime.Object     { return r.newFunc() }
func (r *reviewREST) Destroy()                {}
func (r *reviewREST) NamespaceScoped() bool   { return false }
func (r *reviewREST) GetSingularName() string { return r.singular }
func (r *reviewREST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	return r.create(ctx, obj), nil
}

type reviewKind struct {
	gv       schema.GroupVersion
	resource string
	kind     string
	rest     *reviewREST
}

func reviewKinds(authenticators []Authenticator) []reviewKind {
	authz := authorizationv1.SchemeGroupVersion
	authn := authenticationv1.SchemeGroupVersion
	return []reviewKind{
		{authz, "selfsubjectaccessreviews", "SelfSubjectAccessReview", &reviewREST{
			singular: "selfsubjectaccessreview",
			newFunc:  func() runtime.Object { return &authorizationv1.SelfSubjectAccessReview{} },
			create: func(_ context.Context, obj runtime.Object) runtime.Object {
				review := obj.(*authorizationv1.SelfSubjectAccessReview)
				review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
				return review
			},
		}},
		{authz, "subjectaccessreviews", "SubjectAccessReview", &reviewREST{
			singular: "subjectaccessreview",
			newFunc:  func() runtime.Object { return &authorizationv1.SubjectAccessReview{} },
			create: func(_ context.Context, obj runtime.Object) runtime.Object {
				review := obj.(*authorizationv1.SubjectAccessReview)
				review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
				return review
			},
		}},
		{authn, "tokenreviews", "TokenReview", &reviewREST{
			singular: "tokenreview",
			newFunc:  func() runtime.Object { return &authenticationv1.TokenReview{} },
			create: func(ctx context.Context, obj runtime.Object) runtime.Object {
				review := obj.(*authenticationv1.TokenReview)
				for _, a := range authenticators {
					if u := a(ctx, review.Spec.Token); u != nil {
						review.Status = authenticationv1.TokenReviewStatus{
							Authenticated: true,
							User:          authenticationv1.UserInfo{Username: u.Name, UID: u.UID, Groups: u.Groups},
						}
						return review
					}
				}
				review.Status = authenticationv1.TokenReviewStatus{Error: "token not recognized"}
				return review
			},
		}},
	}
}
