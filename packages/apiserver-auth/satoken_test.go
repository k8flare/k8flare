package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type fakeObjects struct {
	serviceAccounts map[string]*corev1.ServiceAccount
	pods            map[string]*corev1.Pod
	secrets         map[string]*corev1.Secret
	nodes           map[string]*corev1.Node
}

var errNotFound = errors.New("not found")

func (f fakeObjects) ServiceAccount(_ context.Context, ns, name string) (*corev1.ServiceAccount, error) {
	if o, ok := f.serviceAccounts[ns+"/"+name]; ok {
		return o, nil
	}
	return nil, errNotFound
}

func (f fakeObjects) Pod(_ context.Context, ns, name string) (*corev1.Pod, error) {
	if o, ok := f.pods[ns+"/"+name]; ok {
		return o, nil
	}
	return nil, errNotFound
}

func (f fakeObjects) Secret(_ context.Context, ns, name string) (*corev1.Secret, error) {
	if o, ok := f.secrets[ns+"/"+name]; ok {
		return o, nil
	}
	return nil, errNotFound
}

func (f fakeObjects) Node(_ context.Context, name string) (*corev1.Node, error) {
	if o, ok := f.nodes[name]; ok {
		return o, nil
	}
	return nil, errNotFound
}

var testHMAC = []byte("test-hmac")

func init() {
	key, err := NewServiceAccountKey()
	if err != nil {
		panic(err)
	}
	UseServiceAccountKey(testHMAC, key)
}

func objectsWithSA(ns, name, uid string) fakeObjects {
	return fakeObjects{
		serviceAccounts: map[string]*corev1.ServiceAccount{ns + "/" + name: {ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(uid)}}},
		pods:            map[string]*corev1.Pod{},
		secrets:         map[string]*corev1.Secret{},
		nodes:           map[string]*corev1.Node{},
	}
}

func authenticate(t *testing.T, objects fakeObjects, tok string) (*authenticator.Response, bool, error) {
	t.Helper()
	return ServiceAccountToken{HMAC: testHMAC, Objects: objects}.AuthenticateToken(context.Background(), tok)
}

func issue(t *testing.T, audiences []string, bound BoundObjects) string {
	t.Helper()
	tok, err := IssueServiceAccountToken(testHMAC, "kube-system", "coredns", "uid-1", time.Now().Add(time.Hour), audiences, bound)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestServiceAccountTokenRoundTrip(t *testing.T) {
	tok := issue(t, nil, BoundObjects{})
	resp, ok, err := authenticate(t, objectsWithSA("kube-system", "coredns", "uid-1"), tok)
	if err != nil || !ok {
		t.Fatalf("auth %v %v", ok, err)
	}
	if resp.User.GetName() != "system:serviceaccount:kube-system:coredns" {
		t.Fatalf("name %s", resp.User.GetName())
	}
	if resp.User.GetUID() != "uid-1" {
		t.Fatalf("uid %s", resp.User.GetUID())
	}
	got := resp.User.GetGroups()
	want := map[string]bool{"system:serviceaccounts": true, "system:serviceaccounts:kube-system": true, user.AllAuthenticated: true}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("group %s", g)
		}
	}
	ids := resp.User.GetExtra()[user.CredentialIDKey]
	if len(ids) != 1 || len(ids[0]) < 4 || ids[0][:4] != "JTI=" {
		t.Fatalf("credential-id %v", ids)
	}
	if len(resp.Audiences) == 0 {
		t.Fatal("no audiences in response")
	}
}

func TestServiceAccountTokenClaims(t *testing.T) {
	tok := issue(t, []string{"api"}, BoundObjects{Pod: &BoundObject{Name: "p", UID: "pod-uid"}, Node: &BoundObject{Name: "n", UID: "node-uid"}})
	parsed, _, err := jwt.NewParser().ParseUnverified(tok, &jwt.MapClaims{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method.Alg() != "RS256" {
		t.Fatalf("alg %s", parsed.Method.Alg())
	}
	claims := *parsed.Claims.(*jwt.MapClaims)
	if claims["iss"] != "https://kubernetes.default.svc.cluster.local" {
		t.Fatalf("iss %v", claims["iss"])
	}
	for _, k := range []string{"nbf", "iat", "exp", "jti"} {
		if claims[k] == nil {
			t.Fatalf("missing %s", k)
		}
	}
	private := claims["kubernetes.io"].(map[string]any)
	if private["namespace"] != "kube-system" {
		t.Fatalf("namespace %v", private)
	}
	if pod := private["pod"].(map[string]any); pod["name"] != "p" || pod["uid"] != "pod-uid" {
		t.Fatalf("pod %v", pod)
	}
	if node := private["node"].(map[string]any); node["name"] != "n" || node["uid"] != "node-uid" {
		t.Fatalf("node %v", node)
	}
	if private["secret"] != nil {
		t.Fatalf("unexpected secret %v", private["secret"])
	}
}

func installRandomKey(t *testing.T, secret []byte) {
	t.Helper()
	key, err := NewServiceAccountKey()
	if err != nil {
		t.Fatal(err)
	}
	UseServiceAccountKey(secret, key)
}

func TestNewServiceAccountKeyIsRandom(t *testing.T) {
	a, err := NewServiceAccountKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewServiceAccountKey()
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(b) {
		t.Fatal("keys are deterministic")
	}
}

func TestServiceAccountKeyIsNotDerivedFromSecret(t *testing.T) {
	if _, err := saPrivateKey([]byte("never-installed")); err == nil {
		t.Fatal("key derived without installation")
	}
}

func TestServiceAccountTokenRejectsBadHMAC(t *testing.T) {
	installRandomKey(t, []byte("a"))
	installRandomKey(t, []byte("b"))
	tok, err := IssueServiceAccountToken([]byte("a"), "default", "sa", "u", time.Now().Add(time.Hour), nil, BoundObjects{})
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err := ServiceAccountToken{HMAC: []byte("b"), Objects: objectsWithSA("default", "sa", "u")}.AuthenticateToken(context.Background(), tok)
	if err != nil || ok {
		t.Fatalf("expected reject %v %v", ok, err)
	}
}

func TestServiceAccountTokenRejectsLegacyHS256(t *testing.T) {
	for _, issuer := range []string{"k8flare", saIssuer} {
		legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, saClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    issuer,
				Subject:   "system:serviceaccount:default:sa",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Kubernetes: saKubernetes{Namespace: "default", ServiceAccount: saServiceAccountRef{Name: "sa", UID: "u"}},
		})
		tok, err := legacy.SignedString(testHMAC)
		if err != nil {
			t.Fatal(err)
		}
		if resp, ok, _ := authenticate(t, objectsWithSA("default", "sa", "u"), tok); ok || resp != nil {
			t.Fatalf("issuer %q: HS256 token was accepted", issuer)
		}
	}
}

func TestServiceAccountTokenAudiences(t *testing.T) {
	objects := objectsWithSA("kube-system", "coredns", "uid-1")
	other := issue(t, []string{"https://example.test"}, BoundObjects{})
	if _, ok, err := authenticate(t, objects, other); ok || err == nil {
		t.Fatalf("token for another audience accepted: %v %v", ok, err)
	}
	ctx := authenticator.WithAudiences(context.Background(), authenticator.Audiences{"https://example.test"})
	resp, ok, err := ServiceAccountToken{HMAC: testHMAC, Objects: objects}.AuthenticateToken(ctx, other)
	if err != nil || !ok {
		t.Fatalf("requested audience: %v %v", ok, err)
	}
	if len(resp.Audiences) != 1 || resp.Audiences[0] != "https://example.test" {
		t.Fatalf("audiences %v", resp.Audiences)
	}
	kube := issue(t, nil, BoundObjects{})
	if _, ok, err := (ServiceAccountToken{HMAC: testHMAC, Objects: objects}).AuthenticateToken(ctx, kube); ok || err == nil {
		t.Fatalf("api audience token accepted for another target: %v %v", ok, err)
	}
}

func TestServiceAccountTokenExpired(t *testing.T) {
	tok, err := IssueServiceAccountToken(testHMAC, "kube-system", "coredns", "uid-1", time.Now().Add(-time.Hour), nil, BoundObjects{})
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err := authenticate(t, objectsWithSA("kube-system", "coredns", "uid-1"), tok)
	if ok || err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("%v %v", ok, err)
	}
}

func TestServiceAccountTokenRequiresLiveServiceAccount(t *testing.T) {
	tok := issue(t, nil, BoundObjects{})
	if _, ok, err := authenticate(t, fakeObjects{}, tok); ok || err == nil {
		t.Fatalf("missing service account accepted: %v %v", ok, err)
	}
	if _, ok, err := authenticate(t, objectsWithSA("kube-system", "coredns", "recreated"), tok); ok || err == nil {
		t.Fatalf("recreated service account accepted: %v %v", ok, err)
	}
	deleted := objectsWithSA("kube-system", "coredns", "uid-1")
	gone := metav1.NewTime(time.Now().Add(-time.Hour))
	deleted.serviceAccounts["kube-system/coredns"].DeletionTimestamp = &gone
	if _, ok, err := authenticate(t, deleted, tok); ok || err == nil {
		t.Fatalf("deleted service account accepted: %v %v", ok, err)
	}
	missingObjects := ServiceAccountToken{HMAC: testHMAC}
	if _, ok, err := missingObjects.AuthenticateToken(context.Background(), tok); ok || err == nil {
		t.Fatalf("token accepted without a way to look objects up: %v %v", ok, err)
	}
}

func TestServiceAccountTokenBoundPod(t *testing.T) {
	tok := issue(t, nil, BoundObjects{Pod: &BoundObject{Name: "p", UID: "pod-uid"}, Node: &BoundObject{Name: "n", UID: "node-uid"}})
	objects := objectsWithSA("kube-system", "coredns", "uid-1")
	if _, ok, err := authenticate(t, objects, tok); ok || err == nil {
		t.Fatalf("token for a missing pod accepted: %v %v", ok, err)
	}
	objects.pods["kube-system/p"] = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", UID: "other"}}
	if _, ok, err := authenticate(t, objects, tok); ok || err == nil {
		t.Fatalf("token for a recreated pod accepted: %v %v", ok, err)
	}
	objects.pods["kube-system/p"].UID = "pod-uid"
	resp, ok, err := authenticate(t, objects, tok)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	extra := resp.User.GetExtra()
	if extra["authentication.kubernetes.io/pod-name"][0] != "p" || extra["authentication.kubernetes.io/pod-uid"][0] != "pod-uid" || extra["authentication.kubernetes.io/node-name"][0] != "n" {
		t.Fatalf("extra %v", extra)
	}
	gone := metav1.NewTime(time.Now().Add(-time.Hour))
	objects.pods["kube-system/p"].DeletionTimestamp = &gone
	if _, ok, err := authenticate(t, objects, tok); ok || err == nil {
		t.Fatalf("token for a deleted pod accepted: %v %v", ok, err)
	}
}

func TestServiceAccountTokenBoundSecret(t *testing.T) {
	tok := issue(t, nil, BoundObjects{Secret: &BoundObject{Name: "s", UID: "secret-uid"}})
	objects := objectsWithSA("kube-system", "coredns", "uid-1")
	if _, ok, err := authenticate(t, objects, tok); ok || err == nil {
		t.Fatalf("token for a missing secret accepted: %v %v", ok, err)
	}
	objects.secrets["kube-system/s"] = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", UID: "secret-uid"}}
	if _, ok, err := authenticate(t, objects, tok); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	objects.secrets["kube-system/s"].UID = "other"
	if _, ok, err := authenticate(t, objects, tok); ok || err == nil {
		t.Fatalf("token for a recreated secret accepted: %v %v", ok, err)
	}
}

func TestServiceAccountTokenBoundNode(t *testing.T) {
	tok := issue(t, nil, BoundObjects{Node: &BoundObject{Name: "n", UID: "node-uid"}})
	objects := objectsWithSA("kube-system", "coredns", "uid-1")
	if _, ok, err := authenticate(t, objects, tok); ok || err == nil {
		t.Fatalf("token for a missing node accepted: %v %v", ok, err)
	}
	objects.nodes["n"] = &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "node-uid"}}
	resp, ok, err := authenticate(t, objects, tok)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if resp.User.GetExtra()["authentication.kubernetes.io/node-name"][0] != "n" {
		t.Fatalf("extra %v", resp.User.GetExtra())
	}
	objects.nodes["n"].UID = "other"
	if _, ok, err := authenticate(t, objects, tok); ok || err == nil {
		t.Fatalf("token for a recreated node accepted: %v %v", ok, err)
	}
}
