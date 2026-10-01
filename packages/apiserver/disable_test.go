package apiserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/k8flare/k8flare/packages/edgehost"
	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func handlerWithDisabled(t *testing.T, disable string) http.Handler {
	t.Helper()
	edgehost.SetDisabled(disable)
	t.Cleanup(func() { edgehost.SetDisabled("") })
	store := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	handler, err := NewHandler(Config{Kine: store, AdminToken: "admin-token", ClusterUID: "disable-test"})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func adminGet(handler http.Handler, target, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Accept", accept)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func servedGroups(t *testing.T, handler http.Handler) (legacy, aggregated map[string]bool) {
	t.Helper()
	legacy, aggregated = map[string]bool{}, map[string]bool{}
	rec := adminGet(handler, "/apis", "application/json")
	var list metav1.APIGroupList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("/apis: %d %v %s", rec.Code, err, rec.Body.String())
	}
	for _, g := range list.Groups {
		legacy[g.Name] = true
	}
	rec = adminGet(handler, "/apis", aggregatedAccept)
	var discovery apidiscoveryv2.APIGroupDiscoveryList
	if err := json.Unmarshal(rec.Body.Bytes(), &discovery); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("aggregated /apis: %d %v %s", rec.Code, err, rec.Body.String())
	}
	for _, g := range discovery.Items {
		aggregated[g.Name] = true
	}
	return legacy, aggregated
}

func TestMetricsAPIIsServedByDefault(t *testing.T) {
	handler := handlerWithDisabled(t, "servicelb,edge-routing")
	if rec := adminGet(handler, "/apis/metrics.k8s.io", "application/json"); rec.Code != http.StatusOK {
		t.Fatalf("group: %d %s", rec.Code, rec.Body.String())
	}
	if rec := adminGet(handler, "/apis/metrics.k8s.io/v1beta1", "application/json"); rec.Code != http.StatusOK {
		t.Fatalf("version: %d %s", rec.Code, rec.Body.String())
	}
	legacy, aggregated := servedGroups(t, handler)
	if !legacy["metrics.k8s.io"] || !aggregated["metrics.k8s.io"] || !legacy["apps"] || !aggregated["apps"] {
		t.Fatalf("groups: %v %v", legacy, aggregated)
	}
}

func TestMetricsAPIDoesNotExistWhenMetricsServerIsDisabled(t *testing.T) {
	handler := handlerWithDisabled(t, "metrics-server")
	for _, target := range []string{"/apis/metrics.k8s.io", "/apis/metrics.k8s.io/v1beta1", "/apis/metrics.k8s.io/v1beta1/nodes"} {
		if rec := adminGet(handler, target, "application/json"); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d %s", target, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/metrics/scrape", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("scrape: %d %s", rec.Code, rec.Body.String())
	}
	legacy, aggregated := servedGroups(t, handler)
	if legacy["metrics.k8s.io"] || aggregated["metrics.k8s.io"] || !legacy["apps"] || !aggregated["apps"] {
		t.Fatalf("groups: %v %v", legacy, aggregated)
	}
}
