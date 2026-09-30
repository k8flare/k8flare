package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAPIServiceAggregatedUsesRemoteDiscovery(t *testing.T) {
	var path, worker string
	groups := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path, worker = r.URL.Path, r.Header.Get(workerHeader)
		list := apidiscoveryv2.APIGroupDiscoveryList{
			TypeMeta: metav1.TypeMeta{APIVersion: "apidiscovery.k8s.io/v2", Kind: "APIGroupDiscoveryList"},
			Items: []apidiscoveryv2.APIGroupDiscovery{{
				ObjectMeta: metav1.ObjectMeta{Name: "wardle.example.com"},
				Versions: []apidiscoveryv2.APIVersionDiscovery{{
					Version:   "v1",
					Freshness: apidiscoveryv2.DiscoveryFreshnessCurrent,
					Resources: []apidiscoveryv2.APIResourceDiscovery{{Resource: "flunders"}},
				}},
			}},
		}
		data, _ := json.Marshal(list)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}, Request: r}, nil
	})}
	got := apiServiceAggregated(t.Context(), Config{Kine: workerAPIServiceStore(t, "wardle").HTTP, Groups: groups})
	if path != remoteDiscoveryPath || worker != "apiserver-apiregistration" {
		t.Fatalf("asked %q on %q", path, worker)
	}
	if len(got) != 1 || len(got[0].Versions) != 1 || len(got[0].Versions[0].Resources) != 1 || got[0].Versions[0].Freshness != apidiscoveryv2.DiscoveryFreshnessCurrent {
		t.Fatalf("got %+v", got)
	}
}

func TestAPIServiceAggregatedIsStaleWhenGroupsUnavailable(t *testing.T) {
	groups := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
	})}
	got := apiServiceAggregated(t.Context(), Config{Kine: workerAPIServiceStore(t, "wardle").HTTP, Groups: groups})
	if len(got) != 1 || len(got[0].Versions) != 1 || got[0].Versions[0].Freshness != apidiscoveryv2.DiscoveryFreshnessStale {
		t.Fatalf("got %+v", got)
	}
}

func TestAPIServiceAggregatedSkipsGroupsWithoutRemoteAPIServices(t *testing.T) {
	groups := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("groups called without remote APIServices")
		return nil, nil
	})}
	if got := apiServiceAggregated(t.Context(), Config{Groups: groups}); got != nil {
		t.Fatalf("got %+v", got)
	}
}
