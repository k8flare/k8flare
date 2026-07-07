package apiserver_test

import (
	"context"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The kubelet's webhook authenticator/authorizer path used by the
// per-Pod microVM logs/metrics bridge: TokenReview must authenticate
// exactly the cluster token, and SubjectAccessReview -- fed the user
// AND groups TokenReview returned, exactly as the kubelet's webhook
// authorizer does -- must allow via the system:masters identity (RBAC
// enforcement landed 2026-07-07; a bare username with no groups is no
// longer allowed by default). Driven through real client-go against
// wrangler dev, like every other integration test here.
func TestTokenReviewAndSubjectAccessReview(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	t.Run("TokenReview_ClusterToken", func(t *testing.T) {
		tr, err := client.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
			Spec: authenticationv1.TokenReviewSpec{Token: "k8flare-dev-token"},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("TokenReview create: %v", err)
		}
		if !tr.Status.Authenticated {
			t.Fatalf("cluster token not authenticated: %+v", tr.Status)
		}
		if tr.Status.User.Username != "admin" {
			t.Errorf("username = %q, want admin", tr.Status.User.Username)
		}
	})

	t.Run("TokenReview_WrongToken", func(t *testing.T) {
		tr, err := client.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
			Spec: authenticationv1.TokenReviewSpec{Token: "not-the-cluster-token"},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("TokenReview create: %v", err)
		}
		if tr.Status.Authenticated {
			t.Fatalf("wrong token must not authenticate: %+v", tr.Status)
		}
	})

	t.Run("SubjectAccessReview_Allows", func(t *testing.T) {
		sar, err := client.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User:   "admin",
				Groups: []string{"system:masters", "system:authenticated"},
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb: "get", Resource: "nodes", Subresource: "proxy",
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("SubjectAccessReview create: %v", err)
		}
		if !sar.Status.Allowed {
			t.Fatalf("SAR not allowed: %+v", sar.Status)
		}
	})

	// A bare username with no groups gets a real RBAC decision now:
	// nothing binds "admin"-the-user (only the system:masters group), so
	// this must be denied -- the counterpart of the Allows case above.
	t.Run("SubjectAccessReview_DeniesUngroupedUser", func(t *testing.T) {
		sar, err := client.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User: "admin",
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb: "get", Resource: "nodes", Subresource: "proxy",
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("SubjectAccessReview create: %v", err)
		}
		if sar.Status.Allowed {
			t.Fatalf("SAR for ungrouped user unexpectedly allowed: %+v", sar.Status)
		}
	})
}
