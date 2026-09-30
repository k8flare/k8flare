package core

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func init() {
	key, err := auth.NewServiceAccountKey()
	if err != nil {
		panic(err)
	}
	auth.UseServiceAccountKey([]byte("test-hmac"), key)
}

type getterFunc func(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error)

func (f getterFunc) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	return f(ctx, name, opts)
}

func objectGetter(resource string, objs ...metav1.Object) getterFunc {
	return func(_ context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
		for _, o := range objs {
			if o.GetName() == name {
				return o.(runtime.Object), nil
			}
		}
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: resource}, name)
	}
}

func newTokenREST(objs ...metav1.Object) *saTokenREST {
	pick := func(match func(metav1.Object) bool) []metav1.Object {
		var out []metav1.Object
		for _, o := range objs {
			if match(o) {
				out = append(out, o)
			}
		}
		return out
	}
	isPod := func(o metav1.Object) bool { _, ok := o.(*corev1.Pod); return ok }
	isSecret := func(o metav1.Object) bool { _, ok := o.(*corev1.Secret); return ok }
	isNode := func(o metav1.Object) bool { _, ok := o.(*corev1.Node); return ok }
	isSA := func(o metav1.Object) bool { _, ok := o.(*corev1.ServiceAccount); return ok }
	return &saTokenREST{
		sas:     objectGetter("serviceaccounts", pick(isSA)...),
		pods:    objectGetter("pods", pick(isPod)...),
		secrets: objectGetter("secrets", pick(isSecret)...),
		nodes:   objectGetter("nodes", pick(isNode)...),
		hmac:    []byte("test-hmac"),
	}
}

func testServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sa", UID: "sa-uid"}}
}

func requestToken(t *testing.T, r *saTokenREST, spec authenticationv1.TokenRequestSpec) (*authenticationv1.TokenRequest, error) {
	t.Helper()
	ctx := genericapirequest.WithNamespace(context.Background(), "ns")
	out, err := r.Create(ctx, "sa", &authenticationv1.TokenRequest{Spec: spec}, nil, &metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return out.(*authenticationv1.TokenRequest), nil
}

func claimsOf(t *testing.T, token string) jwt.MapClaims {
	t.Helper()
	parsed, _, err := jwt.NewParser().ParseUnverified(token, &jwt.MapClaims{})
	if err != nil {
		t.Fatal(err)
	}
	return *parsed.Claims.(*jwt.MapClaims)
}

func TestTokenRequestDefaultsAndBounds(t *testing.T) {
	r := newTokenREST(testServiceAccount())
	out, err := requestToken(t, r, authenticationv1.TokenRequestSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Spec.ExpirationSeconds == nil || *out.Spec.ExpirationSeconds != 3600 {
		t.Fatalf("default expiration %v", out.Spec.ExpirationSeconds)
	}
	if len(out.Spec.Audiences) == 0 {
		t.Fatal("audiences were not defaulted")
	}
	if time.Until(out.Status.ExpirationTimestamp.Time) > time.Hour {
		t.Fatalf("expiration %v", out.Status.ExpirationTimestamp)
	}
	seconds := func(n int64) authenticationv1.TokenRequestSpec {
		return authenticationv1.TokenRequestSpec{ExpirationSeconds: &n}
	}
	for _, n := range []int64{1, 599, 1<<32 + 1} {
		if _, err := requestToken(t, r, seconds(n)); !apierrors.IsInvalid(err) {
			t.Fatalf("expiration %d: %v", n, err)
		}
	}
	if _, err := requestToken(t, r, seconds(600)); err != nil {
		t.Fatal(err)
	}
}

func TestTokenRequestBoundPod(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: "pod-uid"}, Spec: corev1.PodSpec{ServiceAccountName: "sa", NodeName: "n1"}}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", UID: "node-uid"}}
	r := newTokenREST(testServiceAccount(), pod, node)
	ref := &authenticationv1.BoundObjectReference{Kind: "Pod", APIVersion: "v1", Name: "p", UID: types.UID("pod-uid")}
	out, err := requestToken(t, r, authenticationv1.TokenRequestSpec{BoundObjectRef: ref})
	if err != nil {
		t.Fatal(err)
	}
	private := claimsOf(t, out.Status.Token)["kubernetes.io"].(map[string]any)
	if got := private["pod"].(map[string]any); got["name"] != "p" || got["uid"] != "pod-uid" {
		t.Fatalf("pod claim %v", got)
	}
	if got := private["node"].(map[string]any); got["name"] != "n1" || got["uid"] != "node-uid" {
		t.Fatalf("node claim %v", got)
	}

	stale := *ref
	stale.UID = "old"
	if _, err := requestToken(t, r, authenticationv1.TokenRequestSpec{BoundObjectRef: &stale}); !apierrors.IsConflict(err) {
		t.Fatalf("stale uid: %v", err)
	}
	missing := *ref
	missing.Name = "gone"
	if _, err := requestToken(t, r, authenticationv1.TokenRequestSpec{BoundObjectRef: &missing}); !apierrors.IsNotFound(err) {
		t.Fatalf("missing pod: %v", err)
	}
	pod.Spec.ServiceAccountName = "other"
	if _, err := requestToken(t, r, authenticationv1.TokenRequestSpec{BoundObjectRef: ref}); !apierrors.IsBadRequest(err) {
		t.Fatalf("pod running as another service account: %v", err)
	}
}

func TestTokenRequestBoundSecretAndNode(t *testing.T) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "s", UID: "secret-uid"}}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", UID: "node-uid"}}
	r := newTokenREST(testServiceAccount(), secret, node)

	out, err := requestToken(t, r, authenticationv1.TokenRequestSpec{BoundObjectRef: &authenticationv1.BoundObjectReference{Kind: "Secret", APIVersion: "v1", Name: "s"}})
	if err != nil {
		t.Fatal(err)
	}
	private := claimsOf(t, out.Status.Token)["kubernetes.io"].(map[string]any)
	if got := private["secret"].(map[string]any); got["name"] != "s" || got["uid"] != "secret-uid" {
		t.Fatalf("secret claim %v", got)
	}

	out, err = requestToken(t, r, authenticationv1.TokenRequestSpec{BoundObjectRef: &authenticationv1.BoundObjectReference{Kind: "Node", APIVersion: "v1", Name: "n1"}})
	if err != nil {
		t.Fatal(err)
	}
	private = claimsOf(t, out.Status.Token)["kubernetes.io"].(map[string]any)
	if got := private["node"].(map[string]any); got["name"] != "n1" || got["uid"] != "node-uid" {
		t.Fatalf("node claim %v", got)
	}
	if private["pod"] != nil {
		t.Fatalf("unexpected pod claim %v", private["pod"])
	}

	if _, err := requestToken(t, r, authenticationv1.TokenRequestSpec{BoundObjectRef: &authenticationv1.BoundObjectReference{Kind: "ConfigMap", APIVersion: "v1", Name: "c"}}); !apierrors.IsBadRequest(err) {
		t.Fatalf("unsupported kind: %v", err)
	}
}

func TestTokenRequestChecksServiceAccountUID(t *testing.T) {
	r := newTokenREST(testServiceAccount())
	ctx := genericapirequest.WithNamespace(context.Background(), "ns")
	req := &authenticationv1.TokenRequest{ObjectMeta: metav1.ObjectMeta{UID: "someone-else"}}
	if _, err := r.Create(ctx, "sa", req, nil, &metav1.CreateOptions{}); !apierrors.IsConflict(err) {
		t.Fatalf("uid mismatch: %v", err)
	}
	req = &authenticationv1.TokenRequest{ObjectMeta: metav1.ObjectMeta{Name: "different"}}
	if _, err := r.Create(ctx, "sa", req, nil, &metav1.CreateOptions{}); !apierrors.IsInvalid(err) {
		t.Fatalf("name mismatch: %v", err)
	}
}
