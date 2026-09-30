package authentication

import (
	"context"
	"errors"
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

type staticToken struct {
	user user.Info
}

func (s staticToken) AuthenticateToken(context.Context, string) (*authenticator.Response, bool, error) {
	return &authenticator.Response{User: s.user}, true, nil
}

func TestSelfSubjectReviewReturnsCaller(t *testing.T) {
	build, ok := registry.Resources["selfsubjectreviews"]
	if !ok {
		t.Fatal("selfsubjectreviews not registered")
	}
	store, ok := build(schema.GroupVersion{Group: "authentication.k8s.io", Version: "v1"}, metav1.APIResource{Kind: "SelfSubjectReview", SingularName: "selfsubjectreview"}, registry.Deps{}).(rest.Creater)
	if !ok {
		t.Fatal("not a create store")
	}
	ctx := genericapirequest.WithUser(context.Background(), &user.DefaultInfo{Name: "ada@kooffice.jp", UID: "u1", Groups: []string{"system:authenticated"}})
	got, err := store.Create(ctx, &authenticationv1.SelfSubjectReview{}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	review, ok := got.(*authenticationv1.SelfSubjectReview)
	if !ok || review.Status.UserInfo.Username != "ada@kooffice.jp" || review.Status.UserInfo.UID != "u1" {
		t.Fatalf("%+v", got)
	}
}

func TestTokenReviewCopiesCredentialID(t *testing.T) {
	build, ok := registry.Resources["tokenreviews"]
	if !ok {
		t.Fatal("tokenreviews not registered")
	}
	store, ok := build(schema.GroupVersion{Group: "authentication.k8s.io", Version: "v1"}, metav1.APIResource{Kind: "TokenReview", SingularName: "tokenreview"}, registry.Deps{
		Tokens: staticToken{user: &user.DefaultInfo{
			Name:  "system:serviceaccount:default:default",
			UID:   "sa-1",
			Extra: map[string][]string{user.CredentialIDKey: {"JTI=abc"}},
		}},
	}).(rest.Creater)
	if !ok {
		t.Fatal("not a create store")
	}
	got, err := store.Create(context.Background(), &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: "tok"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	review, ok := got.(*authenticationv1.TokenReview)
	if !ok || !review.Status.Authenticated {
		t.Fatalf("%+v", got)
	}
	ids := []string(review.Status.User.Extra[user.CredentialIDKey])
	if len(ids) != 1 || ids[0] != "JTI=abc" {
		t.Fatalf("credential-id %v", ids)
	}
}

type audienceEcho struct{ seen *authenticator.Audiences }

func (a audienceEcho) AuthenticateToken(ctx context.Context, _ string) (*authenticator.Response, bool, error) {
	auds, _ := authenticator.AudiencesFrom(ctx)
	*a.seen = auds
	return &authenticator.Response{User: &user.DefaultInfo{Name: "sa"}, Audiences: auds}, true, nil
}

type failingToken struct{ err error }

func (f failingToken) AuthenticateToken(context.Context, string) (*authenticator.Response, bool, error) {
	return nil, false, f.err
}

func newTokenReviewStore(t *testing.T, tokens authenticator.Token) rest.Creater {
	build, ok := registry.Resources["tokenreviews"]
	if !ok {
		t.Fatal("tokenreviews not registered")
	}
	store, ok := build(schema.GroupVersion{Group: "authentication.k8s.io", Version: "v1"}, metav1.APIResource{Kind: "TokenReview", SingularName: "tokenreview"}, registry.Deps{Tokens: tokens}).(rest.Creater)
	if !ok {
		t.Fatal("not a create store")
	}
	return store
}

func TestTokenReviewHonoursSpecAudiences(t *testing.T) {
	var seen authenticator.Audiences
	store := newTokenReviewStore(t, audienceEcho{seen: &seen})
	got, err := store.Create(context.Background(), &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: "tok", Audiences: []string{"aud-a"}}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	review := got.(*authenticationv1.TokenReview)
	if len(seen) != 1 || seen[0] != "aud-a" {
		t.Fatalf("authenticator saw audiences %v", seen)
	}
	if len(review.Status.Audiences) != 1 || review.Status.Audiences[0] != "aud-a" {
		t.Fatalf("status audiences %v", review.Status.Audiences)
	}
}

func TestTokenReviewDefaultsToAPIAudiences(t *testing.T) {
	var seen authenticator.Audiences
	store := newTokenReviewStore(t, audienceEcho{seen: &seen})
	got, err := store.Create(context.Background(), &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: "tok"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	review := got.(*authenticationv1.TokenReview)
	if len(seen) == 0 || len(review.Status.Audiences) != len(seen) {
		t.Fatalf("seen %v status %v", seen, review.Status.Audiences)
	}
}

func TestTokenReviewReportsAuthenticatorError(t *testing.T) {
	store := newTokenReviewStore(t, failingToken{err: errors.New("token audiences are invalid")})
	got, err := store.Create(context.Background(), &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: "tok"}}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	review := got.(*authenticationv1.TokenReview)
	if review.Status.Authenticated || review.Status.Error != "token audiences are invalid" {
		t.Fatalf("%+v", review.Status)
	}
}
