package installer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	_ "github.com/k8flare/k8flare/packages/apiserver-core"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

type rewriteTo struct{ base string }

func (r rewriteTo) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := http.NewRequestWithContext(req.Context(), req.Method, r.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	next.Header = req.Header
	return http.DefaultTransport.RoundTrip(next)
}

func TestApplyCreateOfConfigMapIsAuthorizedAsCreate(t *testing.T) {
	srv := httptest.NewServer(&memoryKine{})
	defer srv.Close()
	mux := http.NewServeMux()
	deps := registry.Deps{Kine: &kine.Client{HTTP: &http.Client{Transport: rewriteTo{srv.URL}}}}
	if _, err := Install(mux, deps, schema.GroupVersion{Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	h := auth.WithRemoteUser(mux)
	apply := func(group string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/namespaces/default/configmaps/cm?fieldManager=test",
			strings.NewReader(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm","namespace":"default"},"data":{"a":"b"}}`))
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
	if rec := apply(""); rec.Code != http.StatusForbidden {
		t.Fatalf("without create permission: status=%d body=%s", rec.Code, rec.Body)
	}
	if rec := apply("system:masters"); rec.Code != http.StatusCreated {
		t.Fatalf("with create permission: status=%d body=%s", rec.Code, rec.Body)
	}
}
