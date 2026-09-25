package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	aggregated "k8s.io/apiserver/pkg/endpoints/discovery/aggregated"
)

func TestAggregatedDiscoveryListsServedGroups(t *testing.T) {
	unaggregated := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"}})
	})
	h := wrapAggregated(unaggregated, false, nil)
	req := httptest.NewRequest(http.MethodGet, "/apis", nil)
	req.Header.Set("Accept", aggregatedAccept)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var list apidiscoveryv2.APIGroupDiscoveryList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Kind != "APIGroupDiscoveryList" {
		t.Fatalf("kind %q", list.Kind)
	}
	found := map[string]bool{}
	var eventsResources int
	for _, g := range list.Items {
		found[g.Name] = true
		if g.Name != "events.k8s.io" {
			continue
		}
		for _, v := range g.Versions {
			eventsResources += len(v.Resources)
		}
	}
	for _, name := range []string{"apps", "events.k8s.io", "apiregistration.k8s.io", "metrics.k8s.io"} {
		if !found[name] {
			t.Fatalf("missing group %s in %#v", name, found)
		}
	}
	if found[""] {
		t.Fatal("core group must be served at /api, not /apis")
	}
	if eventsResources == 0 {
		t.Fatal("events.k8s.io has no resources")
	}
}

func TestUnaggregatedDiscoveryStillAPIGroupList(t *testing.T) {
	unaggregated := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"}, Groups: []metav1.APIGroup{{Name: "apps"}}})
	})
	h := wrapAggregated(unaggregated, false, nil)
	req := httptest.NewRequest(http.MethodGet, "/apis", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var list metav1.APIGroupList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Kind != "APIGroupList" {
		t.Fatalf("kind %q body %s", list.Kind, rec.Body.String())
	}
}

func TestAggregatedDiscoveryKeepsBuiltinWhenAddingCRDs(t *testing.T) {
	unaggregated := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	extras := func(_ context.Context, manager aggregated.ResourceManager) {
		manager.WithSource(aggregated.CRDSource).AddGroupVersion("example.com", apidiscoveryv2.APIVersionDiscovery{
			Version:   "v1",
			Freshness: apidiscoveryv2.DiscoveryFreshnessCurrent,
			Resources: []apidiscoveryv2.APIResourceDiscovery{{Resource: "widgets", SingularResource: "widget"}},
		})
	}
	h := wrapAggregated(unaggregated, false, extras)
	req := httptest.NewRequest(http.MethodGet, "/apis", nil)
	req.Header.Set("Accept", aggregatedAccept)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var list apidiscoveryv2.APIGroupDiscoveryList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, g := range list.Items {
		found[g.Name] = true
	}
	if !found["apps"] || !found["events.k8s.io"] || !found["example.com"] {
		t.Fatalf("%v", found)
	}
}

func TestServeAPIsRootDoesNotForward(t *testing.T) {
	root := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("root"))
	})
	rest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forwarded", http.StatusBadRequest)
	})
	h := serveAPIs(root, rest)
	for _, path := range []string{"/apis", "/apis/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "root" {
			t.Fatalf("%s -> %d %s", path, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/apis/apps/v1", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("group path %d", rec.Code)
	}
}
