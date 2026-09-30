package customresources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type memoryKine struct {
	mu   sync.Mutex
	rev  int64
	data map[string]memoryEntry
}

type memoryEntry struct {
	value string
	rev   int64
}

func (m *memoryKine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = map[string]memoryEntry{}
	}
	reply := func(status int, body map[string]any) {
		body["revision"] = m.rev
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	switch {
	case r.URL.Path == "/kv" && r.Method == http.MethodGet:
		key := r.URL.Query().Get("key")
		if e, ok := m.data[key]; ok {
			reply(http.StatusOK, map[string]any{"kv": map[string]any{"key": key, "value": e.value, "modRevision": e.rev}})
			return
		}
		reply(http.StatusOK, map[string]any{})
	case r.URL.Path == "/kv" && r.Method == http.MethodPut:
		var in struct {
			Key      string `json:"key"`
			Value    string `json:"value"`
			Revision int64  `json:"revision"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		existing, exists := m.data[in.Key]
		if (in.Revision == 0 && exists) || (in.Revision != 0 && existing.rev != in.Revision) {
			reply(http.StatusConflict, map[string]any{})
			return
		}
		m.rev++
		m.data[in.Key] = memoryEntry{value: in.Value, rev: m.rev}
		reply(http.StatusOK, map[string]any{})
	case r.URL.Path == "/list":
		prefix := r.URL.Query().Get("prefix")
		kvs := []map[string]any{}
		for k, e := range m.data {
			if strings.HasPrefix(k, prefix) {
				kvs = append(kvs, map[string]any{"key": k, "value": e.value, "modRevision": e.rev})
			}
		}
		reply(http.StatusOK, map[string]any{"kvs": kvs})
	default:
		reply(http.StatusOK, map[string]any{})
	}
}

func applyCreate(t *testing.T, h http.Handler, group, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path+"?fieldManager=test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/apply-patch+yaml")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Remote-User", "alice")
	if group != "" {
		req.Header.Set("X-Remote-Group", group)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const helmChartCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "helmcharts.helm.cattle.io"},
  "spec": {
    "group": "helm.cattle.io",
    "names": {"kind": "HelmChart", "plural": "helmcharts", "singular": "helmchart"},
    "scope": "Namespaced",
    "versions": [{"name": "v1", "served": true, "storage": true,
      "schema": {"openAPIV3Schema": {"type": "object", "x-kubernetes-preserve-unknown-fields": true}}}]
  }
}`

func TestApplyCreateOfCRDIsAuthorizedAsCreate(t *testing.T) {
	srv := httptest.NewServer(&memoryKine{})
	defer srv.Close()
	h, err := NewHandler(Config{Kine: &http.Client{Transport: rewriteTo{srv.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/helmcharts.helm.cattle.io"

	denied := applyCreate(t, h, "", path, helmChartCRD)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("without create permission: status=%d body=%s", denied.Code, denied.Body)
	}
	allowed := applyCreate(t, h, "system:masters", path, helmChartCRD)
	if allowed.Code != http.StatusCreated {
		t.Fatalf("with create permission: status=%d body=%s", allowed.Code, allowed.Body)
	}
}

type rewriteTo struct{ base string }

func (r rewriteTo) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := http.NewRequestWithContext(req.Context(), req.Method, r.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	next.Header = req.Header
	return http.DefaultTransport.RoundTrip(next)
}
