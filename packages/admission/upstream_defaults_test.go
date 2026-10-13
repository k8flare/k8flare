package admission

import (
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
)

func TestTokenRequestWithoutExpirationKeepsTheDefaultThroughAdmission(t *testing.T) {
	gvk := authenticationv1.SchemeGroupVersion.WithKind("TokenRequest")
	internal, err := decodeRequestObject(map[string]any{"apiVersion": "authentication.k8s.io/v1", "kind": "TokenRequest", "spec": map[string]any{}}, gvk)
	if err != nil {
		t.Fatal(err)
	}
	versioned, err := toVersionedObject(internal, gvk.GroupVersion())
	if err != nil {
		t.Fatal(err)
	}
	seconds := versioned.(*authenticationv1.TokenRequest).Spec.ExpirationSeconds
	if seconds == nil || *seconds != 3600 {
		t.Fatalf("expirationSeconds = %v, want 3600", seconds)
	}
}
