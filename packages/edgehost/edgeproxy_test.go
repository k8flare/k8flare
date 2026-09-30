package edgehost

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type countingStore struct {
	values map[string][]byte
	calls  atomic.Int64
}

func (s *countingStore) client() *kine.Client {
	return &kine.Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		s.calls.Add(1)
		switch r.URL.Path {
		case "/kv":
			key := r.URL.Query().Get("key")
			v, ok := s.values[key]
			if !ok {
				return jsonResp(404, []byte(`{"revision":1}`)), nil
			}
			body, _ := json.Marshal(map[string]any{"revision": 1, "kv": map[string]any{"key": key, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
			return jsonResp(200, body), nil
		case "/list":
			var kvs []any
			for k, v := range s.values {
				if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
					kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1})
				}
			}
			body, _ := json.Marshal(map[string]any{"kvs": kvs})
			return jsonResp(200, body), nil
		}
		return jsonResp(404, []byte(`{}`)), nil
	})}}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func webFixture(rules ...Rule) *countingStore {
	svc := corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
		{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080)},
		{Name: "metrics", Port: 9090, TargetPort: intstr.FromInt32(9191)},
	}}}
	node := "node-a"
	eps := corev1.Endpoints{Subsets: []corev1.EndpointSubset{{
		Addresses: []corev1.EndpointAddress{{IP: "10.42.0.7", NodeName: &node}},
		Ports:     []corev1.EndpointPort{{Name: "http", Port: 8080}, {Name: "metrics", Port: 9191}},
	}}}
	return &countingStore{values: map[string][]byte{
		routeTableKey:                     mustJSON(Table{Rules: rules}),
		"/registry/services/default/web":  mustJSON(svc),
		"/registry/endpoints/default/web": mustJSON(eps),
	}}
}

type tunnelLog struct {
	url  string
	host string
	fwd  string
	n    int
}

func (l *tunnelLog) client() *http.Client {
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		l.n++
		l.url, l.host, l.fwd = r.URL.String(), r.Host, r.Header.Get("X-Forwarded-Host")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: http.Header{"X-Up": {"1"}}}, nil
	})}
}

func useClock(t *testing.T) *time.Time {
	t.Helper()
	now := time.Unix(1000, 0)
	saved := edge
	edge = newEdgeCache(func() time.Time { return now }, edgeCacheTTL)
	t.Cleanup(func() { edge = saved })
	return &now
}

func serve(t *testing.T, store *countingStore, tun *tunnelLog, target string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	r := httptest.NewRequest("GET", target, nil)
	w := httptest.NewRecorder()
	handled := proxyGateway(w, r, store.client(), tun.client())
	return w, handled
}

func TestProxyRoutesIngressByPortNameAndKeepsHost(t *testing.T) {
	useClock(t)
	store := webFixture(Rule{
		Source: "Ingress/default/web", Host: "shop.example.com",
		Path:     &pathMatch{Type: "PathPrefix", Value: "/"},
		Backends: []Backend{{Namespace: "default", Name: "web", PortName: "metrics", Weight: 1}},
	})
	tun := &tunnelLog{}
	w, handled := serve(t, store, tun, "https://shop.example.com/x?a=1")
	if !handled || w.Code != 200 {
		t.Fatalf("%v %d %s", handled, w.Code, w.Body.String())
	}
	if tun.url != "https://nodetunnel.internal/dial/node-a/10.42.0.7/9191/x?a=1" {
		t.Fatal(tun.url)
	}
	if tun.host != "shop.example.com" || tun.fwd != "shop.example.com" {
		t.Fatal(tun.host, tun.fwd)
	}
	if w.Header().Get("X-Up") != "1" {
		t.Fatal(w.Header())
	}
}

func TestProxyHotPathCostsNoStoreCallsWithinTheTTL(t *testing.T) {
	now := useClock(t)
	store := webFixture(Rule{
		Source: "Ingress/default/web", Host: "shop.example.com",
		Path:     &pathMatch{Type: "PathPrefix", Value: "/"},
		Backends: []Backend{{Namespace: "default", Name: "web", Port: 80, Weight: 1}},
	})
	tun := &tunnelLog{}
	serve(t, store, tun, "https://shop.example.com/")
	cold := store.calls.Load()
	if cold == 0 || cold > 4 {
		t.Fatalf("cold request cost %d store calls", cold)
	}
	for i := 0; i < 50; i++ {
		if w, _ := serve(t, store, tun, "https://shop.example.com/"); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if got := store.calls.Load(); got != cold {
		t.Fatalf("warm requests added %d store calls", got-cold)
	}
	*now = now.Add(edgeCacheTTL + time.Millisecond)
	serve(t, store, tun, "https://shop.example.com/")
	if got := store.calls.Load(); got != 2*cold {
		t.Fatalf("refresh cost %d store calls, want %d", got-cold, cold)
	}
	if tun.n != 52 {
		t.Fatal(tun.n)
	}
}

func TestProxyUnknownHostFallsThroughAfterOneTableRead(t *testing.T) {
	useClock(t)
	store := webFixture()
	tun := &tunnelLog{}
	for i := 0; i < 10; i++ {
		if _, handled := serve(t, store, tun, "https://nobody.example.com/"); handled {
			t.Fatal("handled")
		}
	}
	if store.calls.Load() != 1 {
		t.Fatal(store.calls.Load())
	}
}

func TestProxyWithoutTableFallsThrough(t *testing.T) {
	useClock(t)
	store := &countingStore{values: map[string][]byte{}}
	if _, handled := serve(t, store, &tunnelLog{}, "https://shop.example.com/"); handled {
		t.Fatal("handled")
	}
}

func TestProxyHostlessFallbackNeverShadowsTheAPIOnItsOwnAddress(t *testing.T) {
	useClock(t)
	store := webFixture(Rule{
		Source: "Ingress/ingress-5779/e2e-example-ing", Fallback: true,
		Path:     &pathMatch{Type: "PathPrefix", Value: "/"},
		Backends: []Backend{{Namespace: "ingress-5779", Name: "default-backend", Port: 8080, Weight: 1}},
	})
	tun := &tunnelLog{}
	for _, target := range []string{
		"https://127.0.0.1:16443/api/v1/namespaces/x/configmaps?watch=true",
		"https://localhost:6443/api",
		"https://[::1]:6443/apis",
	} {
		if w, handled := serve(t, store, tun, target); handled {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
		}
	}
	if w, handled := serve(t, store, tun, "https://anything.example.com/"); !handled || w.Code != 500 {
		t.Fatal(handled, w.Code)
	}
}

func TestProxyRedirectAndInvalid(t *testing.T) {
	useClock(t)
	store := webFixture(
		Rule{Source: "HTTPRoute/default/old", Host: "old.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"},
			Filters: []Filter{{Type: "RequestRedirect", RequestRedirect: &RequestRedirect{Hostname: "new.example.com", StatusCode: 301}}}},
		Rule{Source: "HTTPRoute/default/bad", Host: "bad.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Invalid: "unsupported filter type RequestMirror"},
		Rule{Source: "HTTPRoute/default/none", Host: "none.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"}},
	)
	tun := &tunnelLog{}
	w, _ := serve(t, store, tun, "https://old.example.com/a?b=1")
	if w.Code != 301 || w.Header().Get("Location") != "https://new.example.com/a?b=1" {
		t.Fatal(w.Code, w.Header())
	}
	if w, _ := serve(t, store, tun, "https://bad.example.com/"); w.Code != 500 {
		t.Fatal(w.Code)
	}
	if w, _ := serve(t, store, tun, "https://none.example.com/"); w.Code != 500 {
		t.Fatal(w.Code)
	}
	if tun.n != 0 {
		t.Fatal(tun.n)
	}
}

func TestProxyMissingServiceIs500AndNoEndpointsIs503(t *testing.T) {
	useClock(t)
	store := webFixture(
		Rule{Source: "a", Host: "gone.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Backends: []Backend{{Namespace: "default", Name: "gone", Port: 80, Weight: 1}}},
		Rule{Source: "b", Host: "port.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Backends: []Backend{{Namespace: "default", Name: "web", Port: 1234, Weight: 1}}},
	)
	tun := &tunnelLog{}
	if w, _ := serve(t, store, tun, "https://gone.example.com/"); w.Code != 500 {
		t.Fatal(w.Code)
	}
	if w, _ := serve(t, store, tun, "https://port.example.com/"); w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestProxyIgnoresClientPortHeaderOnLoadBalancer(t *testing.T) {
	useClock(t)
	svc := corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Ports: []corev1.ServicePort{{Name: "http", Port: 80}, {Name: "admin", Port: 9000}}}}
	node := "node-a"
	eps := corev1.Endpoints{Subsets: []corev1.EndpointSubset{{
		Addresses: []corev1.EndpointAddress{{IP: "10.42.0.7", NodeName: &node}},
		Ports:     []corev1.EndpointPort{{Name: "http", Port: 80}, {Name: "admin", Port: 9000}},
	}}}
	store := &countingStore{values: map[string][]byte{
		"/registry/services/default/web":  mustJSON(svc),
		"/registry/endpoints/default/web": mustJSON(eps),
	}}
	tun := &tunnelLog{}
	r := httptest.NewRequest("GET", "https://web--default.k8flare.com/", nil)
	r.Header.Set("X-K8flare-Port", "9000")
	if !Proxy(httptest.NewRecorder(), r, store.client(), tun.client()) {
		t.Fatal("not handled")
	}
	if !strings.Contains(tun.url, "/10.42.0.7/80/") {
		t.Fatal(tun.url)
	}
	if tun.host != "web--default.k8flare.com" {
		t.Fatal(tun.host)
	}
}

func TestSvcPathOnlyAppliesToControlPlaneHosts(t *testing.T) {
	r := httptest.NewRequest("GET", "https://shop.example.com/svc/a/b", nil)
	if _, ok := ParseServiceRoute(r); ok {
		t.Fatal("app host")
	}
	r = httptest.NewRequest("GET", "https://api.k8flare.com/svc/a/b", nil)
	if ref, ok := ParseServiceRoute(r); !ok || ref.Namespace != "a" || ref.Name != "b" {
		t.Fatal(ref, ok)
	}
}

func TestResolveDialSkipsNotReadySliceEndpoints(t *testing.T) {
	useClock(t)
	notReady, ready := false, true
	nodeA, nodeB := "node-a", "node-b"
	port, name := int32(8080), "http"
	slice := discoveryv1.EndpointSlice{
		Endpoints: []discoveryv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}, NodeName: &nodeA, Conditions: discoveryv1.EndpointConditions{Ready: &notReady}},
			{Addresses: []string{"10.0.0.2"}, NodeName: &nodeB, Conditions: discoveryv1.EndpointConditions{Ready: &ready}},
		},
		Ports: []discoveryv1.EndpointPort{{Name: &name, Port: &port}},
	}
	slice.Labels = map[string]string{"kubernetes.io/service-name": "web"}
	store := &countingStore{values: map[string][]byte{"/registry/endpointslices/default/web-abc": mustJSON(slice)}}
	r := httptest.NewRequest("GET", "https://x/", nil)
	node, host, p := resolveDial(r, store.client(), Ref{Namespace: "default", Name: "web"}, []corev1.ServicePort{{Name: "http", Port: 80}}, portSel{Number: 80})
	if node != "node-b" || host != "10.0.0.2" || p != "8080" {
		t.Fatal(node, host, p)
	}
}

func BenchmarkLookup(b *testing.B) {
	table := &Table{}
	for i := 0; i < 200; i++ {
		table.Rules = append(table.Rules, Rule{
			Source: "Ingress/default/i" + string(rune('a'+i%26)), Host: "h" + string(rune('a'+i%26)) + ".example.com",
			Path: &pathMatch{Type: "PathPrefix", Value: "/p" + string(rune('a'+i%26))}, Backends: backendNamed("web"),
		})
	}
	table.Rules = append(table.Rules, Rule{Source: "x", Host: "app.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Backends: backendNamed("web")})
	table.Prepare()
	header := http.Header{}
	for i := 0; i < b.N; i++ {
		if table.Lookup("app.example.com", "GET", "/api/v1/things", header, nil) == nil {
			b.Fatal("miss")
		}
	}
}
