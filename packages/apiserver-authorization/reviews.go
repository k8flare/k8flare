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

func extraFrom(extra map[string]authorizationv1.ExtraValue) map[string][]string {
	if extra == nil {
		return nil
	}
	out := make(map[string][]string, len(extra))
	for k, v := range extra {
		out[k] = []string(v)
	}
	return out
}

func reviewStatus(decision authorizer.Decision, reason string, err error) authorizationv1.SubjectAccessReviewStatus {
	status := authorizationv1.SubjectAccessReviewStatus{Allowed: decision == authorizer.DecisionAllow, Denied: decision == authorizer.DecisionDeny, Reason: reason}
	if err != nil {
		status.EvaluationError = err.Error()
	}
	return status
}

func resourceRules(infos []authorizer.ResourceRuleInfo) []authorizationv1.ResourceRule {
	rules := make([]authorizationv1.ResourceRule, len(infos))
	for i, info := range infos {
		rules[i] = authorizationv1.ResourceRule{Verbs: info.GetVerbs(), APIGroups: info.GetAPIGroups(), Resources: info.GetResources(), ResourceNames: info.GetResourceNames()}
	}
	return rules
}

func nonResourceRules(infos []authorizer.NonResourceRuleInfo) []authorizationv1.NonResourceRule {
	rules := make([]authorizationv1.NonResourceRule, len(infos))
	for i, info := range infos {
		rules[i] = authorizationv1.NonResourceRule{Verbs: info.GetVerbs(), NonResourceURLs: info.GetNonResourceURLs()}
	}
	return rules
}

func init() {
	registry.Resources["subjectaccessreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SubjectAccessReview)
			attrs := attributesFrom(authorizer.AttributesRecord{
				User: &user.DefaultInfo{Name: review.Spec.User, UID: review.Spec.UID, Groups: review.Spec.Groups, Extra: extraFrom(review.Spec.Extra)},
			}, review.Spec.ResourceAttributes, review.Spec.NonResourceAttributes)
			decision, reason, err := authz.New(deps.Kine).Authorize(ctx, attrs)
			review.Status = reviewStatus(decision, reason, err)
			return review
		}
	})
	registry.Resources["selfsubjectaccessreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SelfSubjectAccessReview)
			u, _ := genericapirequest.UserFrom(ctx)
			attrs := attributesFrom(authorizer.AttributesRecord{User: u}, review.Spec.ResourceAttributes, review.Spec.NonResourceAttributes)
			decision, reason, err := authz.New(deps.Kine).Authorize(ctx, attrs)
			review.Status = reviewStatus(decision, reason, err)
			return review
		}
	})
	registry.Resources["localsubjectaccessreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.LocalSubjectAccessReview)
			ns, _ := genericapirequest.NamespaceFrom(ctx)
			if review.Spec.ResourceAttributes != nil && review.Spec.ResourceAttributes.Namespace == "" {
				review.Spec.ResourceAttributes.Namespace = ns
			}
			attrs := attributesFrom(authorizer.AttributesRecord{
				User: &user.DefaultInfo{Name: review.Spec.User, UID: review.Spec.UID, Groups: review.Spec.Groups, Extra: extraFrom(review.Spec.Extra)},
			}, review.Spec.ResourceAttributes, review.Spec.NonResourceAttributes)
			decision, reason, err := authz.New(deps.Kine).Authorize(ctx, attrs)
			review.Status = reviewStatus(decision, reason, err)
			return review
		}
	})
	registry.Resources["selfsubjectrulesreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authorizationv1.SelfSubjectRulesReview)
			u, _ := genericapirequest.UserFrom(ctx)
			resourceInfo, nonResourceInfo, incomplete, err := authz.Resolver(deps.Kine).RulesFor(ctx, u, review.Spec.Namespace)
			review.Status = authorizationv1.SubjectRulesReviewStatus{
				ResourceRules:    resourceRules(resourceInfo),
				NonResourceRules: nonResourceRules(nonResourceInfo),
				Incomplete:       incomplete,
			}
			if err != nil {
				review.Status.EvaluationError = err.Error()
			}
			return review
		}
	})
}
