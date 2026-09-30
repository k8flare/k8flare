package authentication

import (
	"context"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func init() {
	registry.Resources["tokenreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authenticationv1.TokenReview)
			auds := authenticator.Audiences(review.Spec.Audiences)
			if len(auds) == 0 {
				auds = auth.APIAudiences
			}
			resp, ok, err := deps.Tokens.AuthenticateToken(authenticator.WithAudiences(ctx, auds), review.Spec.Token)
			if err != nil {
				review.Status = authenticationv1.TokenReviewStatus{Error: err.Error()}
				return review
			}
			if !ok {
				review.Status = authenticationv1.TokenReviewStatus{Error: "token not recognized"}
				return review
			}
			review.Status = authenticationv1.TokenReviewStatus{
				Authenticated: true,
				User:          userInfoFrom(resp.User),
				Audiences:     resp.Audiences,
			}
			return review
		}
	})
	registry.Resources["selfsubjectreviews"] = registry.Review(func(deps registry.Deps) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
			review := obj.(*authenticationv1.SelfSubjectReview)
			u, ok := genericapirequest.UserFrom(ctx)
			if !ok || u == nil {
				return review
			}
			review.Status.UserInfo = userInfoFrom(u)
			return review
		}
	})
}

func userInfoFrom(u user.Info) authenticationv1.UserInfo {
	info := authenticationv1.UserInfo{Username: u.GetName(), UID: u.GetUID(), Groups: u.GetGroups()}
	extra := u.GetExtra()
	if len(extra) == 0 {
		return info
	}
	info.Extra = make(map[string]authenticationv1.ExtraValue, len(extra))
	for k, v := range extra {
		info.Extra[k] = authenticationv1.ExtraValue(v)
	}
	return info
}
