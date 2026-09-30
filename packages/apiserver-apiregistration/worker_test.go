package apiregistration

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	helper "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1/helper"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, contentType string, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}
}

func workerAPIService(group, version, worker string) *apiregistrationv1.APIService {
	return &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: version + "." + group, Annotations: map[string]string{workerAnnot: worker}},
		Spec:       apiregistrationv1.APIServiceSpec{Group: group, Version: version},
	}
}

func useStoredAPIServices(t *testing.T, svcs ...*apiregistrationv1.APIService) {
	t.Helper()
	data := map[string][]byte{}
	for _, svc := range svcs {
		raw, err := json.Marshal(svc)
		if err != nil {
			t.Fatal(err)
		}
		data["/registry/apiservices/"+svc.Name] = raw
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list":
			var kvs []map[string]any
			for k, v := range data {
				kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": kvs})
		case "/kv":
			k := r.URL.Query().Get("key")
			if v, ok := data[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	previousKine, previousCache := kineClient, remoteDiscoveries
	kineClient = &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		u := *r.URL
		u.Scheme, u.Host = "http", strings.TrimPrefix(srv.URL, "http://")
		r.URL = &u
		return http.DefaultTransport.RoundTrip(r)
	})}}
	remoteDiscoveries = newDiscoveryCache()
	t.Cleanup(func() { kineClient, remoteDiscoveries = previousKine, previousCache })
}

func useHooks(t *testing.T, next roundTripFunc) {
	t.Helper()
	previous := hooks
	hooks = next
	if next == nil {
		hooks = nil
	}
	t.Cleanup(func() { hooks = previous })
}

func TestWorkerNameFromAnnotation(t *testing.T) {
	svc := workerAPIService("wardle.example.com", "v1", " wardle ")
	if got := workerName(svc); got != "wardle" {
		t.Fatalf("workerName = %q", got)
	}
	if !isRemote(svc) {
		t.Fatal("APIService with a worker is remote")
	}
	if isRemote(&apiregistrationv1.APIService{}) {
		t.Fatal("APIService without service or worker is local")
	}
}

func TestServeAggregatedForwardsToWorker(t *testing.T) {
	var seen *http.Request
	useHooks(t, func(r *http.Request) (*http.Response, error) {
		seen = r
		return jsonResponse(http.StatusOK, "application/json", map[string]string{"ok": "yes"}), nil
	})
	svc := workerAPIService("wardle.example.com", "v1", "wardle")
	req := httptest.NewRequest(http.MethodGet, "/apis/wardle.example.com/v1/flunders?limit=1", nil)
	rec := httptest.NewRecorder()
	if err := serveAggregated(rec, req, svc); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if seen == nil || seen.URL.String() != "https://hooks.internal/hook/wardle/apis/wardle.example.com/v1/flunders?limit=1" {
		t.Fatalf("worker request = %v", seen)
	}
}

func TestServeAggregatedWorkerWithoutHooksIsUnavailable(t *testing.T) {
	useHooks(t, nil)
	svc := workerAPIService("wardle.example.com", "v1", "wardle")
	req := httptest.NewRequest(http.MethodGet, "/apis/wardle.example.com/v1/flunders", nil)
	if err := serveAggregated(httptest.NewRecorder(), req, svc); err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyAvailabilityMarksWorkerAvailable(t *testing.T) {
	svc := workerAPIService("wardle.example.com", "v1", "wardle")
	applyAvailability(svc)
	if !helper.IsAPIServiceConditionTrue(svc, apiregistrationv1.Available) {
		t.Fatalf("conditions = %+v", svc.Status.Conditions)
	}
}

func TestMarkLocalLeavesWorkerAPIServiceAlone(t *testing.T) {
	svc := workerAPIService("wardle.example.com", "v1", "wardle")
	markLocal(runtime.Object(svc))
	if len(svc.Status.Conditions) != 0 {
		t.Fatalf("conditions = %+v", svc.Status.Conditions)
	}
}

func discoveryList(group, version, resource string) apidiscoveryv2.APIGroupDiscoveryList {
	return apidiscoveryv2.APIGroupDiscoveryList{
		TypeMeta: metav1.TypeMeta{APIVersion: "apidiscovery.k8s.io/v2", Kind: "APIGroupDiscoveryList"},
		Items: []apidiscoveryv2.APIGroupDiscovery{
			{
				ObjectMeta: metav1.ObjectMeta{Name: group},
				Versions: []apidiscoveryv2.APIVersionDiscovery{{
					Version:   version,
					Resources: []apidiscoveryv2.APIResourceDiscovery{{Resource: resource, Scope: apidiscoveryv2.ScopeNamespace}},
				}},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "other.example.com"},
				Versions:   []apidiscoveryv2.APIVersionDiscovery{{Version: "v9"}},
			},
		},
	}
}

func TestRemoteDiscoveryFetchesAggregatedOnceAndCaches(t *testing.T) {
	useStoredAPIServices(t, workerAPIService("wardle.example.com", "v1", "wardle"))
	var fetches atomic.Int32
	var accept, user string
	useHooks(t, func(r *http.Request) (*http.Response, error) {
		fetches.Add(1)
		accept = r.Header.Get("Accept")
		user = r.Header.Get("X-Remote-User")
		if r.URL.String() != "https://hooks.internal/hook/wardle/apis" {
			t.Errorf("unexpected URL %s", r.URL)
		}
		return jsonResponse(http.StatusOK, aggregatedAccept, discoveryList("wardle.example.com", "v1", "flunders")), nil
	})
	for i := 0; i < 3; i++ {
		groups := remoteDiscoveries.groups(t.Context())
		if len(groups) != 1 || groups[0].Name != "wardle.example.com" || len(groups[0].Versions) != 1 {
			t.Fatalf("groups = %+v", groups)
		}
		version := groups[0].Versions[0]
		if version.Freshness != apidiscoveryv2.DiscoveryFreshnessCurrent || len(version.Resources) != 1 || version.Resources[0].Resource != "flunders" {
			t.Fatalf("version = %+v", version)
		}
	}
	if fetches.Load() != 1 {
		t.Fatalf("fetched %d times, want 1", fetches.Load())
	}
	if accept != aggregatedAccept || user != "system:kube-aggregator" {
		t.Fatalf("accept %q user %q", accept, user)
	}
}

func TestRemoteDiscoveryFallsBackToLegacyResourceList(t *testing.T) {
	useStoredAPIServices(t, workerAPIService("wardle.example.com", "v1", "wardle"))
	useHooks(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/hook/wardle/apis":
			return jsonResponse(http.StatusNotFound, "application/json", metav1.Status{}), nil
		case "/hook/wardle/apis/wardle.example.com/v1":
			return jsonResponse(http.StatusOK, "application/json", metav1.APIResourceList{
				TypeMeta:     metav1.TypeMeta{APIVersion: "v1", Kind: "APIResourceList"},
				GroupVersion: "wardle.example.com/v1",
				APIResources: []metav1.APIResource{{Name: "flunders", Namespaced: true, Kind: "Flunder", Verbs: metav1.Verbs{"get"}}},
			}), nil
		}
		return jsonResponse(http.StatusNotFound, "application/json", metav1.Status{}), nil
	})
	groups := remoteDiscoveries.groups(t.Context())
	if len(groups) != 1 || len(groups[0].Versions) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	version := groups[0].Versions[0]
	if version.Freshness != apidiscoveryv2.DiscoveryFreshnessCurrent || len(version.Resources) != 1 || version.Resources[0].Resource != "flunders" {
		t.Fatalf("version = %+v", version)
	}
}

func TestRemoteDiscoveryMarksUnreachableWorkerStale(t *testing.T) {
	useStoredAPIServices(t, workerAPIService("wardle.example.com", "v1", "wardle"))
	useHooks(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusServiceUnavailable, "application/json", metav1.Status{}), nil
	})
	groups := remoteDiscoveries.groups(t.Context())
	if len(groups) != 1 || len(groups[0].Versions) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	if groups[0].Versions[0].Freshness != apidiscoveryv2.DiscoveryFreshnessStale || len(groups[0].Versions[0].Resources) != 0 {
		t.Fatalf("version = %+v", groups[0].Versions[0])
	}
}

func TestRemoteDiscoveryHandlerServesGroupList(t *testing.T) {
	useStoredAPIServices(t, workerAPIService("wardle.example.com", "v1", "wardle"))
	useHooks(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, aggregatedAccept, discoveryList("wardle.example.com", "v1", "flunders")), nil
	})
	rec := httptest.NewRecorder()
	proxyRemote()(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, remoteDiscoveryPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var list apidiscoveryv2.APIGroupDiscoveryList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Kind != "APIGroupDiscoveryList" || len(list.Items) != 1 || list.Items[0].Name != "wardle.example.com" {
		t.Fatalf("list = %+v", list)
	}
}
