package auth

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

func legacyFixture(t *testing.T) (fakeObjects, string) {
	t.Helper()
	key, err := saPrivateKey(testHMAC)
	if err != nil {
		t.Fatal(err)
	}
	generator, err := serviceaccount.JWTTokenGenerator(serviceaccount.LegacyIssuer, key)
	if err != nil {
		t.Fatal(err)
	}
	objects := objectsWithSA("default", "builder", "sa-uid")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "builder-token", UID: "secret-uid"},
		Type:       corev1.SecretTypeServiceAccountToken,
	}
	public, private := serviceaccount.LegacyClaims(*objects.serviceAccounts["default/builder"], *secret)
	tok, err := generator.GenerateToken(context.Background(), public, private)
	if err != nil {
		t.Fatal(err)
	}
	secret.Data = map[string][]byte{corev1.ServiceAccountTokenKey: []byte(tok)}
	objects.secrets["default/builder-token"] = secret
	return objects, tok
}

func TestLegacySecretTokenAuthenticatesUntilSecretIsDeleted(t *testing.T) {
	objects, tok := legacyFixture(t)
	authn := ServiceAccountToken{HMAC: testHMAC, Objects: NewServiceAccountObjects(objects)}
	resp, ok, err := authn.AuthenticateToken(context.Background(), tok)
	if err != nil || !ok {
		t.Fatalf("auth %v %v", ok, err)
	}
	if resp.User.GetName() != "system:serviceaccount:default:builder" {
		t.Fatalf("name %s", resp.User.GetName())
	}
	if resp.User.GetUID() != "sa-uid" {
		t.Fatalf("uid %s", resp.User.GetUID())
	}
	if len(resp.Audiences) == 0 {
		t.Fatal("no audiences in response")
	}
	delete(objects.secrets, "default/builder-token")
	if _, ok, err := authn.AuthenticateToken(context.Background(), tok); ok || err == nil {
		t.Fatalf("token of a deleted secret: ok=%v err=%v", ok, err)
	}
}

func TestLegacySecretTokenRejected(t *testing.T) {
	deleted := metav1.NewTime(time.Now())
	cases := map[string]func(fakeObjects){
		"secret holds another token": func(o fakeObjects) {
			o.secrets["default/builder-token"].Data[corev1.ServiceAccountTokenKey] = []byte("other")
		},
		"secret is being deleted": func(o fakeObjects) {
			o.secrets["default/builder-token"].DeletionTimestamp = &deleted
		},
		"secret is marked invalid": func(o fakeObjects) {
			o.secrets["default/builder-token"].Labels = map[string]string{serviceaccount.InvalidSinceLabelKey: "2026-01-01"}
		},
		"service account is gone": func(o fakeObjects) {
			delete(o.serviceAccounts, "default/builder")
		},
		"service account is being deleted": func(o fakeObjects) {
			o.serviceAccounts["default/builder"].DeletionTimestamp = &deleted
		},
		"service account was recreated": func(o fakeObjects) {
			o.serviceAccounts["default/builder"].UID = "other-uid"
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			objects, tok := legacyFixture(t)
			change(objects)
			authn := ServiceAccountToken{HMAC: testHMAC, Objects: NewServiceAccountObjects(objects)}
			if _, ok, err := authn.AuthenticateToken(context.Background(), tok); ok || err == nil {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestLegacySecretTokenNeedsRequestedAudience(t *testing.T) {
	objects, tok := legacyFixture(t)
	authn := ServiceAccountToken{HMAC: testHMAC, Objects: NewServiceAccountObjects(objects)}
	ctx := authenticator.WithAudiences(context.Background(), authenticator.Audiences{"vault"})
	if _, ok, err := authn.AuthenticateToken(ctx, tok); ok || err == nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
