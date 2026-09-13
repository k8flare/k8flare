package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, path, accept string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", accept)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func TestDocuments(t *testing.T) {
	h, err := Handler()
	if err != nil {
		t.Fatal(err)
	}
	var v2 struct {
		Definitions map[string]json.RawMessage `json:"definitions"`
		Paths       map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(get(t, h, "/openapi/v2", "application/json").Body.Bytes(), &v2); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"io.k8s.api.core.v1.Pod", "io.k8s.api.coordination.v1.Lease", "io.k8s.apimachinery.pkg.apis.meta.v1.Status"} {
		if _, ok := v2.Definitions[name]; !ok {
			t.Errorf("v2 lacks definition %s", name)
		}
	}
	if _, ok := v2.Paths["/api/v1/namespaces/{namespace}/pods/{name}/log"]; !ok {
		t.Error("v2 lacks the pods/log path")
	}
	var root struct {
		Paths map[string]struct {
			ServerRelativeURL string `json:"serverRelativeURL"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(get(t, h, "/openapi/v3", "application/json").Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	for _, gv := range []string{"api/v1", "apis/coordination.k8s.io/v1", "apis/storage.k8s.io/v1"} {
		if _, ok := root.Paths[gv]; !ok {
			t.Errorf("v3 root lacks %s", gv)
		}
	}
	var v3 struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(get(t, h, root.Paths["api/v1"].ServerRelativeURL, "application/json").Body.Bytes(), &v3); err != nil {
		t.Fatal(err)
	}
	if _, ok := v3.Components.Schemas["io.k8s.api.core.v1.Pod"]; !ok {
		t.Error("v3 api/v1 lacks the Pod schema")
	}
	proto := get(t, h, root.Paths["api/v1"].ServerRelativeURL, "application/com.github.proto-openapi.spec.v3.v1.0+protobuf")
	if ct := proto.Header().Get("Content-Type"); !strings.Contains(ct, "protobuf") {
		t.Errorf("v3 protobuf content type: %q", ct)
	}
}
