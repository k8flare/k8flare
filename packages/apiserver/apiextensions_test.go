package apiserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/discovery"
)

func crdWorkerDown() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("bridge: no response headers in time")
	})}
}

func crdWorkerServing(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		if r.Header.Get("Accept") == aggregatedAccept {
			body, _ = json.Marshal(apidiscoveryv2.APIGroupDiscoveryList{
				TypeMeta: metav1.TypeMeta{APIVersion: "apidiscovery.k8s.io/v2", Kind: "APIGroupDiscoveryList"},
				Items: []apidiscoveryv2.APIGroupDiscovery{
					{ObjectMeta: metav1.ObjectMeta{Name: "apiextensions.k8s.io"}, Versions: []apidiscoveryv2.APIVersionDiscovery{{Version: "v1", Resources: []apidiscoveryv2.APIResourceDiscovery{{Resource: "customresourcedefinitions"}}}}},
					{ObjectMeta: metav1.ObjectMeta{Name: "example.com"}, Versions: []apidiscoveryv2.APIVersionDiscovery{{Version: "v1", Resources: []apidiscoveryv2.APIResourceDiscovery{{Resource: "widgets"}}}}},
				},
			})
		} else {
			body, _ = json.Marshal(metav1.APIGroupList{
				TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"},
				Groups: []metav1.APIGroup{
					{Name: "apiextensions.k8s.io", Versions: []metav1.GroupVersionForDiscovery{{GroupVersion: "apiextensions.k8s.io/v1", Version: "v1"}}},
					{Name: "example.com", Versions: []metav1.GroupVersionForDiscovery{{GroupVersion: "example.com/v1", Version: "v1"}}},
				},
			})
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{"Content-Type": {"application/json"}}, Request: r}, nil
	})}
}

func apisHandler(cfg Config) http.Handler {
	addresses := discovery.DefaultAddresses{DefaultAddress: "k8flare"}
	return wrapAggregated(rootAPIs(addresses, cfg), false, addDynamicAggregated(cfg))
}

func legacyGroups(t *testing.T, h http.Handler) map[string][]string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/apis", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var list metav1.APIGroupList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, g := range list.Groups {
		if _, dup := out[g.Name]; dup {
			t.Fatalf("group %s listed twice in %s", g.Name, rec.Body.String())
		}
		var versions []string
		for _, v := range g.Versions {
			versions = append(versions, v.Version)
		}
		out[g.Name] = versions
	}
	return out
}

func aggregatedGroups(t *testing.T, h http.Handler) map[string][]string {
	t.Helper()
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
	out := map[string][]string{}
	for _, g := range list.Items {
		if _, dup := out[g.Name]; dup {
			t.Fatalf("group %s listed twice in %s", g.Name, rec.Body.String())
		}
		var resources []string
		for _, v := range g.Versions {
			for _, r := range v.Resources {
				resources = append(resources, v.Version+"/"+r.Resource)
				for _, sub := range r.Subresources {
					resources = append(resources, v.Version+"/"+r.Resource+"/"+sub.Subresource)
				}
			}
		}
		out[g.Name] = resources
	}
	return out
}

func TestLegacyAPIsKeepsApiextensionsWhenCRDWorkerFails(t *testing.T) {
	got := legacyGroups(t, apisHandler(Config{CustomResources: crdWorkerDown()}))
	if versions := got["apiextensions.k8s.io"]; len(versions) != 1 || versions[0] != "v1" {
		t.Fatalf("apiextensions.k8s.io versions %v in %v", versions, got)
	}
	if _, ok := got["apps"]; !ok {
		t.Fatalf("apps missing in %v", got)
	}
}

func TestAggregatedAPIsKeepsApiextensionsWhenCRDWorkerFails(t *testing.T) {
	got := aggregatedGroups(t, apisHandler(Config{CustomResources: crdWorkerDown()}))
	resources := got["apiextensions.k8s.io"]
	if len(resources) != 2 || resources[0] != "v1/customresourcedefinitions" || resources[1] != "v1/customresourcedefinitions/status" {
		t.Fatalf("apiextensions.k8s.io resources %v in %v", resources, got)
	}
}

func TestAPIsListsApiextensionsOnceWhenCRDWorkerServes(t *testing.T) {
	h := apisHandler(Config{CustomResources: crdWorkerServing(t)})
	legacy := legacyGroups(t, h)
	if len(legacy["apiextensions.k8s.io"]) != 1 || len(legacy["example.com"]) != 1 {
		t.Fatalf("legacy %v", legacy)
	}
	aggregated := aggregatedGroups(t, h)
	if len(aggregated["apiextensions.k8s.io"]) != 2 || len(aggregated["example.com"]) != 1 {
		t.Fatalf("aggregated %v", aggregated)
	}
}
