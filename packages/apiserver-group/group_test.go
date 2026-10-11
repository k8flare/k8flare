package group

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/k8flare/k8flare/packages/apiserver-authentication"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type emptyStore struct{}

func (emptyStore) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
}

func tokenReview(t *testing.T, handler http.Handler, token string) authenticationv1.TokenReviewStatus {
	t.Helper()
	body, _ := json.Marshal(authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: token}})
	req := httptest.NewRequest(http.MethodPost, "/apis/authentication.k8s.io/v1/tokenreviews", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Remote-User", "admin")
	req.Header.Set("X-Remote-Group", "system:masters")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var out authenticationv1.TokenReview
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	return out.Status
}

func TestTokenReviewRecognisesTheAdminTokenFromItsDigest(t *testing.T) {
	sum := sha256.Sum256([]byte("admin-token"))
	handler, err := NewHandler(schema.GroupVersion{Group: "authentication.k8s.io", Version: "v1"}, Config{Kine: &http.Client{Transport: emptyStore{}}, AdminTokenSHA256: hex.EncodeToString(sum[:])})
	if err != nil {
		t.Fatal(err)
	}
	if status := tokenReview(t, handler, "admin-token"); !status.Authenticated || status.User.Username != "admin" {
		t.Fatalf("admin token: %+v", status)
	}
	if status := tokenReview(t, handler, hex.EncodeToString(sum[:])); status.Authenticated {
		t.Fatalf("the digest itself authenticated: %+v", status)
	}
}
