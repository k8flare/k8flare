package registrytest

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type CountingStore struct {
	mu           sync.Mutex
	rev          int64
	data         map[string][]byte
	revs         map[string]int64
	GetCalls     atomic.Int64
	PutCalls     atomic.Int64
	RegistryPuts atomic.Int64
	MarkerGets   atomic.Int64
	MarkerPuts   atomic.Int64
	FailPuts     atomic.Bool
	FailPutKey   string
	client       *kine.Client
	srv          *httptest.Server
}

type toServer struct{ base string }

func (t toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := http.NewRequestWithContext(req.Context(), req.Method, t.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	next.Header = req.Header
	return http.DefaultTransport.RoundTrip(next)
}

func NewCountingStore(t *testing.T) *CountingStore {
	cs := &CountingStore{
		data: make(map[string][]byte),
		revs: make(map[string]int64),
	}
	cs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(code int, body map[string]any) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(body)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /kv":
			cs.GetCalls.Add(1)
			key := r.URL.Query().Get("key")
			if strings.HasPrefix(key, "/k8flare/bootstrap/") {
				cs.MarkerGets.Add(1)
			}
			cs.mu.Lock()
			v, ok := cs.data[key]
			rev := cs.rev
			modRev := cs.revs[key]
			cs.mu.Unlock()
			if !ok {
				reply(http.StatusNotFound, map[string]any{"revision": rev})
				return
			}
			reply(http.StatusOK, map[string]any{
				"revision": rev,
				"kv": kine.KV{
					Key:         key,
					Value:       base64.StdEncoding.EncodeToString(v),
					ModRevision: modRev,
				},
			})
		case "PUT /kv":
			cs.PutCalls.Add(1)
			var body struct {
				Key      string `json:"key"`
				Value    string `json:"value"`
				Revision int64  `json:"revision"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.HasPrefix(body.Key, "/registry/") {
				cs.RegistryPuts.Add(1)
			}
			if strings.HasPrefix(body.Key, "/k8flare/bootstrap/") {
				cs.MarkerPuts.Add(1)
			}
			cs.mu.Lock()
			defer cs.mu.Unlock()
			if cs.FailPuts.Load() || (cs.FailPutKey != "" && strings.Contains(body.Key, cs.FailPutKey)) {
				reply(http.StatusInternalServerError, map[string]any{"error": "simulated put error"})
				return
			}
			if body.Revision != cs.revs[body.Key] {
				reply(http.StatusConflict, map[string]any{"revision": cs.rev})
				return
			}
			cs.rev++
			raw, _ := base64.StdEncoding.DecodeString(body.Value)
			cs.data[body.Key] = raw
			cs.revs[body.Key] = cs.rev
			reply(http.StatusOK, map[string]any{"revision": cs.rev})
		default:
			reply(http.StatusNotFound, map[string]any{})
		}
	}))
	t.Cleanup(cs.srv.Close)
	cs.client = &kine.Client{HTTP: &http.Client{Transport: toServer{base: cs.srv.URL}}}
	return cs
}

func (cs *CountingStore) Client() *kine.Client {
	return cs.client
}

func (cs *CountingStore) GetMarker(name string) (string, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	v, ok := cs.data["/k8flare/bootstrap/"+name]
	if !ok {
		return "", false
	}
	return string(v), true
}

func (cs *CountingStore) SetMarker(name, hash string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.rev++
	cs.data["/k8flare/bootstrap/"+name] = []byte(hash)
	cs.revs["/k8flare/bootstrap/"+name] = cs.rev
}

func (cs *CountingStore) NewStore(t *testing.T, gv schema.GroupVersion, res metav1.APIResource) *registry.Store {
	t.Helper()
	store, err := registry.NewStore(cs.client, gv, res)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
