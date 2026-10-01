package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

type keyValues struct {
	mu   sync.Mutex
	rev  int64
	data map[string][]byte
	revs map[string]int64
}

func (s *keyValues) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reply := func(code int, body map[string]any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}
	var body struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Revision int64  `json:"revision"`
	}
	if r.Method != http.MethodGet {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /kv":
		key := r.URL.Query().Get("key")
		v, ok := s.data[key]
		if !ok {
			reply(http.StatusNotFound, map[string]any{"revision": s.rev})
			return
		}
		reply(http.StatusOK, map[string]any{"revision": s.rev, "kv": kine.KV{Key: key, Value: base64.StdEncoding.EncodeToString(v), ModRevision: s.revs[key]}})
	case "PUT /kv":
		if body.Revision != s.revs[body.Key] {
			reply(http.StatusConflict, map[string]any{"revision": s.rev})
			return
		}
		s.rev++
		s.data[body.Key], _ = base64.StdEncoding.DecodeString(body.Value)
		s.revs[body.Key] = s.rev
		reply(http.StatusOK, map[string]any{"revision": s.rev})
	case "DELETE /kv":
		if _, ok := s.data[body.Key]; !ok {
			reply(http.StatusNotFound, map[string]any{"revision": s.rev})
			return
		}
		s.rev++
		delete(s.data, body.Key)
		delete(s.revs, body.Key)
		reply(http.StatusOK, map[string]any{"revision": s.rev})
	default:
		reply(http.StatusNotFound, map[string]any{})
	}
}

func (s *keyValues) seed(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rev++
	s.data[key] = []byte(value)
	s.revs[key] = s.rev
}

func (s *keyValues) has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[key]
	return ok
}

func servicesOverEndpoints(t *testing.T) (*keyValues, *registry.Store) {
	t.Helper()
	kv := &keyValues{data: map[string][]byte{}, revs: map[string]int64{}}
	srv := httptest.NewServer(kv)
	t.Cleanup(srv.Close)
	client := &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}
	gv := schema.GroupVersion{Version: "v1"}
	services, err := registry.NewStore(client, gv, metav1.APIResource{Name: "services", SingularName: "service", Namespaced: true, Kind: "Service"})
	if err != nil {
		t.Fatal(err)
	}
	registry.Customizers["services"](services, registry.Deps{Kine: client})
	endpoints, err := registry.NewStore(client, gv, metav1.APIResource{Name: "endpoints", SingularName: "endpoints", Namespaced: true, Kind: "Endpoints"})
	if err != nil {
		t.Fatal(err)
	}
	previous := serviceEndpoints.store
	serviceEndpoints.store = endpoints
	t.Cleanup(func() { serviceEndpoints.store = previous })
	kv.seed("/registry/services/ns/web", `{"apiVersion":"v1","kind":"Service","metadata":{"name":"web","namespace":"ns","uid":"s1"},"spec":{"clusterIP":"None","clusterIPs":["None"],"ports":[{"port":80,"protocol":"TCP","targetPort":80}],"type":"ClusterIP"}}`)
	kv.seed("/registry/endpoints/ns/web", `{"apiVersion":"v1","kind":"Endpoints","metadata":{"name":"web","namespace":"ns","uid":"e1"},"subsets":[{"addresses":[{"ip":"10.0.0.24"}],"ports":[{"port":80,"protocol":"TCP"}]}]}`)
	return kv, services
}

func TestDeletingAServiceDeletesItsEndpoints(t *testing.T) {
	kv, services := servicesOverEndpoints(t)
	ctx := genericapirequest.WithNamespace(context.Background(), "ns")
	if _, _, err := services.Delete(ctx, "web", rest.ValidateAllObjectFunc, &metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if kv.has("/registry/services/ns/web") {
		t.Fatal("the Service is still stored")
	}
	if kv.has("/registry/endpoints/ns/web") {
		t.Fatal("the Endpoints outlived their Service")
	}
}

func TestDryRunServiceDeleteKeepsItsEndpoints(t *testing.T) {
	kv, services := servicesOverEndpoints(t)
	ctx := genericapirequest.WithNamespace(context.Background(), "ns")
	if _, _, err := services.Delete(ctx, "web", rest.ValidateAllObjectFunc, &metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
		t.Fatal(err)
	}
	if !kv.has("/registry/services/ns/web") || !kv.has("/registry/endpoints/ns/web") {
		t.Fatal("a dry run removed stored objects")
	}
}

func TestDeletingAServiceWithoutEndpointsSucceeds(t *testing.T) {
	kv, services := servicesOverEndpoints(t)
	ctx := genericapirequest.WithNamespace(context.Background(), "ns")
	endpoints := serviceEndpoints.store
	if _, _, err := endpoints.Delete(ctx, "web", rest.ValidateAllObjectFunc, &metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := services.Delete(ctx, "web", rest.ValidateAllObjectFunc, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	if kv.has("/registry/services/ns/web") {
		t.Fatal("the Service is still stored")
	}
}
