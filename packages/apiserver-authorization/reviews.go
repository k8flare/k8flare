package authorization

import (
	"context"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

func init() {
	registry.Resources["selfsubjectaccessreviews"] = registry.Review(func(authenticator.Token) func(context.Context, runtime.Object) runtime.Object {
		return func(_ context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SelfSubjectAccessReview)
			review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
			return review
		}
	})
	registry.Resources["subjectaccessreviews"] = registry.Review(func(authenticator.Token) func(context.Context, runtime.Object) runtime.Object {
		return func(_ context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SubjectAccessReview)
			review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
			return review
		}
	})
}
