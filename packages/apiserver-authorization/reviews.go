package authorization

import (
	"context"

	authz "github.com/k8flare/k8flare/packages/apiserver-authz"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func attributesFrom(u authorizer.AttributesRecord, resource *authorizationv1.ResourceAttributes, nonResource *authorizationv1.NonResourceAttributes) authorizer.AttributesRecord {
	if resource != nil {
		u.ResourceRequest = true
		u.Namespace = resource.Namespace
		u.Verb = resource.Verb
		u.APIGroup = resource.Group
		u.APIVersion = resource.Version
		u.Resource = resource.Resource
		u.Subresource = resource.Subresource
		u.Name = resource.Name
	}
	if nonResource != nil {
		u.ResourceRequest = false
		u.Path = nonResource.Path
		u.Verb = nonResource.Verb
	}
	return u
}

func init() {
	registry.Resources["subjectaccessreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SubjectAccessReview)
			attrs := attributesFrom(authorizer.AttributesRecord{
				User: &user.DefaultInfo{Name: review.Spec.User, Groups: review.Spec.Groups},
			}, review.Spec.ResourceAttributes, review.Spec.NonResourceAttributes)
			decision, reason, err := authz.New(deps.Kine).Authorize(ctx, attrs)
			review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: decision == authorizer.DecisionAllow, Denied: decision == authorizer.DecisionDeny, Reason: reason}
			if err != nil {
				review.Status.EvaluationError = err.Error()
			}
			return review
		}
	})
	registry.Resources["selfsubjectaccessreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SelfSubjectAccessReview)
			u, _ := genericapirequest.UserFrom(ctx)
			attrs := attributesFrom(authorizer.AttributesRecord{User: u}, review.Spec.ResourceAttributes, review.Spec.NonResourceAttributes)
			decision, reason, err := authz.New(deps.Kine).Authorize(ctx, attrs)
			review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: decision == authorizer.DecisionAllow, Denied: decision == authorizer.DecisionDeny, Reason: reason}
			if err != nil {
				review.Status.EvaluationError = err.Error()
			}
			return review
		}
	})
}
