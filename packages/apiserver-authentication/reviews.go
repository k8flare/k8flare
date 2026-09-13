package authentication

import (
	"context"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

func init() {
	registry.Resources["tokenreviews"] = registry.Review(func(tokens authenticator.Token) func(context.Context, runtime.Object) runtime.Object {
		return func(ctx context.Context, obj runtime.Object) runtime.Object {
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
		}
	})
}
