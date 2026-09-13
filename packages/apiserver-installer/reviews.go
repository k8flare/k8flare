package installer

import (
	"context"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

// reviewREST is a create-only virtual resource served through the API
// installer, so the review APIs the kubelet and the k3s agent call get the
// same content negotiation and discovery as everything else. Every
// authenticated caller is allowed everything for now.
type reviewREST struct {
	gvk      schema.GroupVersionKind
	singular string
	create   func(ctx context.Context, obj runtime.Object) runtime.Object
}

var _ rest.Creater = (*reviewREST)(nil)

func newReviewREST(gvk schema.GroupVersionKind, singular string, create func(context.Context, runtime.Object) runtime.Object) *reviewREST {
	return &reviewREST{gvk: gvk, singular: singular, create: create}
}

func (r *reviewREST) New() runtime.Object {
	obj, _ := scheme.Scheme.New(r.gvk)
	return obj
}
func (r *reviewREST) Destroy()                {}
func (r *reviewREST) NamespaceScoped() bool   { return false }
func (r *reviewREST) GetSingularName() string { return r.singular }
func (r *reviewREST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	return r.create(ctx, obj), nil
}

func reviewCreators(tokens authenticator.Token) map[string]func(context.Context, runtime.Object) runtime.Object {
	return map[string]func(context.Context, runtime.Object) runtime.Object{
		"selfsubjectaccessreviews": func(_ context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SelfSubjectAccessReview)
			review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
			return review
		},
		"subjectaccessreviews": func(_ context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SubjectAccessReview)
			review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
			return review
		},
		"tokenreviews": func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authenticationv1.TokenReview)
			resp, ok, err := tokens.AuthenticateToken(ctx, review.Spec.Token)
			if err != nil || !ok {
				review.Status = authenticationv1.TokenReviewStatus{Error: "token not recognized"}
				return review
			}
			review.Status = authenticationv1.TokenReviewStatus{
				Authenticated: true,
				User:          authenticationv1.UserInfo{Username: resp.User.GetName(), UID: resp.User.GetUID(), Groups: resp.User.GetGroups()},
			}
			return review
		},
	}
}
