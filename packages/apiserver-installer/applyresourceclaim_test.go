package installer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	_ "github.com/k8flare/k8flare/packages/apiserver-resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestApplyPatchOfExistingResourceClaimHasATypedSchema(t *testing.T) {
	srv := httptest.NewServer(&memoryKine{})
	defer srv.Close()
	mux := http.NewServeMux()
	deps := registry.Deps{Kine: &kine.Client{HTTP: &http.Client{Transport: rewriteTo{srv.URL}}}}
	if _, err := Install(mux, deps, schema.GroupVersion{Group: "resource.k8s.io", Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	h := auth.WithRemoteUser(mux)
	send := func(method, path, contentType, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Remote-User", "admin")
		req.Header.Set("X-Remote-Group", "system:masters")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	claim := `{"apiVersion":"resource.k8s.io/v1","kind":"ResourceClaim","metadata":{"name":"test","namespace":"default"},"spec":{"devices":{"requests":[{"name":"req-0","exactly":{"deviceClassName":"dra.example.com"}}]}}}`
	if rec := send(http.MethodPost, "/apis/resource.k8s.io/v1/namespaces/default/resourceclaims", "application/json", claim); rec.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rec.Code, rec.Body)
	}
	labelled := `{"apiVersion":"resource.k8s.io/v1","kind":"ResourceClaim","metadata":{"name":"test","namespace":"default","labels":{"test.dra.example.com":"test"}}}`
	rec := send(http.MethodPatch, "/apis/resource.k8s.io/v1/namespaces/default/resourceclaims/test?fieldManager=test-apply&force=true&fieldValidation=Strict", "application/apply-patch+yaml", labelled)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply patch of the existing claim: status=%d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"test.dra.example.com":"test"`) {
		t.Fatalf("applied label missing from %s", rec.Body)
	}
}
