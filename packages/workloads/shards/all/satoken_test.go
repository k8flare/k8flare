package all

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/k8flare/k8flare/packages/workloads"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

func useServiceAccountKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	previous := workloads.ServiceAccountKey
	workloads.ServiceAccountKey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	t.Cleanup(func() { workloads.ServiceAccountKey = previous })
	return key
}

func tokenSecretFixture(name, account string) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       "default",
			UID:             types.UID(name + "-uid"),
			ResourceVersion: "1",
			Annotations:     map[string]string{v1.ServiceAccountNameKey: account},
		},
		Type: v1.SecretTypeServiceAccountToken,
	}
}

func serviceAccountFixture(name string) *v1.ServiceAccount {
	return &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name + "-uid"), ResourceVersion: "1"}}
}

func TestSyncPopulatesServiceAccountTokenSecret(t *testing.T) {
	key := useServiceAccountKey(t)
	client := fake.NewSimpleClientset(serviceAccountFixture("builder"), tokenSecretFixture("builder-token", "builder"))
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"secrets"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().Secrets("default").Get(context.Background(), "builder-token", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data[v1.ServiceAccountRootCAKey]) != "ca" {
		t.Fatalf("ca.crt = %q", got.Data[v1.ServiceAccountRootCAKey])
	}
	if string(got.Data[v1.ServiceAccountNamespaceKey]) != "default" {
		t.Fatalf("namespace = %q", got.Data[v1.ServiceAccountNamespaceKey])
	}
	if got.Annotations[v1.ServiceAccountUIDKey] != "builder-uid" {
		t.Fatalf("annotations = %v", got.Annotations)
	}
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(string(got.Data[v1.ServiceAccountTokenKey]), claims, func(*jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	if err != nil || !parsed.Valid {
		t.Fatalf("token %q: %v", got.Data[v1.ServiceAccountTokenKey], err)
	}
	want := [][2]string{
		{"iss", serviceaccount.LegacyIssuer},
		{"sub", "system:serviceaccount:default:builder"},
		{"kubernetes.io/serviceaccount/namespace", "default"},
		{"kubernetes.io/serviceaccount/secret.name", "builder-token"},
		{"kubernetes.io/serviceaccount/service-account.name", "builder"},
		{"kubernetes.io/serviceaccount/service-account.uid", "builder-uid"},
	}
	for _, claim := range want {
		if claims[claim[0]] != claim[1] {
			t.Fatalf("claim %s = %v, want %s", claim[0], claims[claim[0]], claim[1])
		}
	}
	sa, err := client.CoreV1().ServiceAccounts("default").Get(context.Background(), "builder", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Secrets) != 0 {
		t.Fatalf("service account secrets = %v", sa.Secrets)
	}
}

func TestSyncDeletesTokenSecretOfDeletedServiceAccount(t *testing.T) {
	useServiceAccountKey(t)
	client := fake.NewSimpleClientset(serviceAccountFixture("builder"), tokenSecretFixture("builder-token", "builder"))
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"secrets"}); err != nil {
		t.Fatal(err)
	}
	populated, err := client.CoreV1().Secrets("default").Get(context.Background(), "builder-token", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(populated.Data[v1.ServiceAccountTokenKey]) == 0 {
		t.Fatal("token was not populated")
	}
	if err := client.CoreV1().ServiceAccounts("default").Delete(context.Background(), "builder", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"serviceaccounts"}); err != nil {
		t.Fatal(err)
	}
	_, err = client.CoreV1().Secrets("default").Get(context.Background(), "builder-token", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("token secret of a deleted service account: %v", err)
	}
}

func TestSyncDeletesTokenSecretNamingMissingServiceAccount(t *testing.T) {
	useServiceAccountKey(t)
	client := fake.NewSimpleClientset(serviceAccountFixture("builder"), tokenSecretFixture("ghost-token", "ghost"))
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"secrets"}); err != nil {
		t.Fatal(err)
	}
	_, err := client.CoreV1().Secrets("default").Get(context.Background(), "ghost-token", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("token secret naming a missing service account: %v", err)
	}
}

func TestSyncLeavesTokenSecretsAloneWithoutSigningKey(t *testing.T) {
	client := fake.NewSimpleClientset(serviceAccountFixture("builder"), tokenSecretFixture("builder-token", "builder"), tokenSecretFixture("ghost-token", "ghost"))
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"secrets"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().Secrets("default").Get(context.Background(), "builder-token", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Data) != 0 {
		t.Fatalf("data = %v", got.Data)
	}
	if _, err := client.CoreV1().Secrets("default").Get(context.Background(), "ghost-token", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}
