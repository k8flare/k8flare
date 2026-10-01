package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	"k8s.io/apiserver/pkg/authentication/user"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
)

type bootstrapObjects struct {
	secret *corev1.Secret
	err    error
}

func (o *bootstrapObjects) Secret(_ context.Context, namespace, name string) (*corev1.Secret, error) {
	if o.err != nil {
		return nil, o.err
	}
	if o.secret == nil || o.secret.Namespace != namespace || o.secret.Name != name {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, name)
	}
	return o.secret.DeepCopy(), nil
}

func TestBootstrapToken(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*corev1.Secret)
		token  string
		want   bool
		groups []string
	}{
		{name: "valid", want: true},
		{name: "wrong secret", token: "abcdef.0000000000000000"},
		{name: "expired", change: func(s *corev1.Secret) {
			s.Data[bootstrapapi.BootstrapTokenExpirationKey] = []byte(time.Now().Add(-time.Hour).Format(time.RFC3339))
		}},
		{name: "future expiration", want: true, change: func(s *corev1.Secret) {
			s.Data[bootstrapapi.BootstrapTokenExpirationKey] = []byte(time.Now().Add(time.Hour).Format(time.RFC3339))
		}},
		{name: "invalid expiration", change: func(s *corev1.Secret) { s.Data[bootstrapapi.BootstrapTokenExpirationKey] = []byte("invalid") }},
		{name: "missing usage", change: func(s *corev1.Secret) { delete(s.Data, bootstrapapi.BootstrapTokenUsageAuthentication) }},
		{name: "wrong usage", change: func(s *corev1.Secret) { s.Data[bootstrapapi.BootstrapTokenUsageAuthentication] = []byte("True") }},
		{name: "wrong namespace", change: func(s *corev1.Secret) { s.Namespace = "default" }},
		{name: "wrong name", change: func(s *corev1.Secret) { s.Name = "bootstrap-token-123456" }},
		{name: "wrong type", change: func(s *corev1.Secret) { s.Type = corev1.SecretTypeOpaque }},
		{name: "wrong id", change: func(s *corev1.Secret) { s.Data[bootstrapapi.BootstrapTokenIDKey] = []byte("123456") }},
		{name: "deleted", change: func(s *corev1.Secret) { now := metav1.Now(); s.DeletionTimestamp = &now }},
		{name: "extra groups", want: true, change: func(s *corev1.Secret) {
			s.Data[bootstrapapi.BootstrapTokenExtraGroupsKey] = []byte("system:bootstrappers:workers,system:bootstrappers:workers")
		}, groups: []string{"system:bootstrappers", "system:bootstrappers:workers", "system:authenticated"}},
		{name: "invalid groups", change: func(s *corev1.Secret) { s.Data[bootstrapapi.BootstrapTokenExtraGroupsKey] = []byte("system:masters") }},
		{name: "malformed", token: "abcdef.short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "bootstrap-token-abcdef"}, Type: bootstrapapi.SecretTypeBootstrapToken, Data: map[string][]byte{bootstrapapi.BootstrapTokenIDKey: []byte("abcdef"), bootstrapapi.BootstrapTokenSecretKey: []byte("0123456789abcdef"), bootstrapapi.BootstrapTokenUsageAuthentication: []byte("true")}}
			if tc.change != nil {
				tc.change(secret)
			}
			objects := &bootstrapObjects{secret: secret}
			tokens := WithAuthenticatedGroup(BootstrapToken{Objects: objects})
			token := tc.token
			if token == "" {
				token = "abcdef.0123456789abcdef"
			}
			resp, ok, err := tokens.AuthenticateToken(t.Context(), token)
			if err != nil || ok != tc.want {
				t.Fatalf("ok=%v err=%v want=%v", ok, err, tc.want)
			}
			if ok {
				groups := tc.groups
				if groups == nil {
					groups = []string{"system:bootstrappers", "system:authenticated"}
				}
				if resp.User.GetName() != "system:bootstrap:abcdef" || !reflect.DeepEqual(resp.User.GetGroups(), groups) {
					t.Fatalf("user=%v", resp.User)
				}
			}
			s := supervisor.New(supervisor.NewVault(&kine.Client{HTTP: &http.Client{Transport: bootstrapTransport{}}}), "")
			s.BootstrapTokens = bearertoken.New(tokens)
			mux := http.NewServeMux()
			s.Register(mux)
			req := httptest.NewRequest(http.MethodGet, "/v1-k3s/config", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			wantCode := http.StatusUnauthorized
			if tc.want {
				wantCode = http.StatusOK
			}
			if rec.Code != wantCode {
				t.Fatalf("join status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestBootstrapTokenStorageError(t *testing.T) {
	failure := errors.New("storage unavailable")
	_, ok, err := (BootstrapToken{Objects: &bootstrapObjects{err: failure}}).AuthenticateToken(t.Context(), "abcdef.0123456789abcdef")
	if ok || !errors.Is(err, failure) {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

type bootstrapTransport struct{}

func (bootstrapTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"revision":1}`)), Request: r}, nil
}

func TestBootstrapTokenRevocation(t *testing.T) {
	objects := &bootstrapObjects{secret: &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "bootstrap-token-abcdef"},
		Type:       bootstrapapi.SecretTypeBootstrapToken,
		Data: map[string][]byte{
			bootstrapapi.BootstrapTokenIDKey:               []byte("abcdef"),
			bootstrapapi.BootstrapTokenSecretKey:           []byte("0123456789abcdef"),
			bootstrapapi.BootstrapTokenUsageAuthentication: []byte("true"),
		},
	}}
	token := BootstrapToken{Objects: objects}
	if _, ok, err := token.AuthenticateToken(t.Context(), "abcdef.0123456789abcdef"); err != nil || !ok {
		t.Fatalf("before deletion: %v %v", ok, err)
	}
	objects.secret = nil
	if _, ok, err := token.AuthenticateToken(t.Context(), "abcdef.0123456789abcdef"); err != nil || ok {
		t.Fatalf("after deletion: %v %v", ok, err)
	}
}

func TestBootstrapTokenMissingKineSecret(t *testing.T) {
	objects := KineObjects{Client: &kine.Client{HTTP: &http.Client{Transport: bootstrapTransport{}}}}
	if _, ok, err := (BootstrapToken{Objects: objects}).AuthenticateToken(t.Context(), "abcdef.0123456789abcdef"); ok || err != nil {
		t.Fatalf("missing secret: %v %v", ok, err)
	}
}

func TestBootstrapSupervisorRequiresBootstrapGroup(t *testing.T) {
	for _, groups := range [][]string{{"system:authenticated"}, {"system:masters"}, {"system:bootstrappers"}} {
		s := supervisor.New(supervisor.NewVault(&kine.Client{HTTP: &http.Client{Transport: bootstrapTransport{}}}), "")
		s.BootstrapTokens = authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
			return &authenticator.Response{User: &user.DefaultInfo{Name: "someone", Groups: groups}}, true, nil
		})
		mux := http.NewServeMux()
		s.Register(mux)
		req := httptest.NewRequest(http.MethodGet, "/v1-k3s/config", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		want := http.StatusUnauthorized
		if groups[0] == "system:bootstrappers" {
			want = http.StatusOK
		}
		if rec.Code != want {
			t.Fatalf("groups=%v status=%d want=%d", groups, rec.Code, want)
		}
		req = httptest.NewRequest(http.MethodGet, "/v1-k3s/node-tunnel", nil)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("bootstrap group allowed node tunnel: %d", rec.Code)
		}
	}
}
