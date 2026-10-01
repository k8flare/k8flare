package edgehost

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func TestProvisionServiceSetsHostname(t *testing.T) {
	key := "/registry/services/default/lb-web"
	svc := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "lb-web", "namespace": "default"},
		"spec":       map[string]any{"type": "LoadBalancer"},
	}
	raw, _ := json.Marshal(svc)
	stored := map[string][]byte{key: raw}
	store := &kine.Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/kv":
			if r.Method == http.MethodGet {
				body, _ := json.Marshal(map[string]any{"kv": map[string]any{"key": key, "value": base64.StdEncoding.EncodeToString(stored[key]), "modRevision": 3}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}}, nil
			}
			var req struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			decoded, _ := base64.StdEncoding.DecodeString(req.Value)
			stored[req.Key] = decoded
			body, _ := json.Marshal(map[string]any{"revision": 4})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{"error":"missing"}`)), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})}}
	req, _ := http.NewRequest(http.MethodPost, "/internal/loadbalancer/provision", strings.NewReader(`{"keys":["`+key+`"]}`))
	rr := httptestNew(t, store, req)
	if rr != http.StatusNoContent {
		t.Fatal(rr)
	}
	var got struct {
		Status struct {
			LoadBalancer struct {
				Ingress []struct {
					Hostname string `json:"hostname"`
				} `json:"ingress"`
			} `json:"loadBalancer"`
		} `json:"status"`
	}
	if json.Unmarshal(stored[key], &got) != nil || got.Status.LoadBalancer.Ingress[0].Hostname != "lb-web--default.k8flare.com" {
		t.Fatalf("%s", stored[key])
	}
}

func TestProvisionServiceUsesConfiguredDomain(t *testing.T) {
	domain := "example.test"
	key := "/registry/services/default/web"
	svc := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "web", "namespace": "default"},
		"spec":       map[string]any{"type": "LoadBalancer"},
	}
	raw, _ := json.Marshal(svc)
	stored := map[string][]byte{key: raw}
	store := &kine.Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/kv":
			if r.Method == http.MethodGet {
				body, _ := json.Marshal(map[string]any{"kv": map[string]any{"key": key, "value": base64.StdEncoding.EncodeToString(stored[key]), "modRevision": 3}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}}, nil
			}
			var req struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			decoded, _ := base64.StdEncoding.DecodeString(req.Value)
			stored[req.Key] = decoded
			body, _ := json.Marshal(map[string]any{"revision": 4})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{"error":"missing"}`)), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})}}
	SetClusterDomain(domain)
	t.Cleanup(func() { SetClusterDomain("") })
	req, _ := http.NewRequest(http.MethodPost, "/internal/loadbalancer/provision", strings.NewReader(`{"keys":["`+key+`"]}`))
	rr := httptestNew(t, store, req)
	if rr != http.StatusNoContent {
		t.Fatal(rr)
	}
	var got struct {
		Status struct {
			LoadBalancer struct {
				Ingress []struct {
					Hostname string `json:"hostname"`
				} `json:"ingress"`
			} `json:"loadBalancer"`
		} `json:"status"`
	}
	if json.Unmarshal(stored[key], &got) != nil || got.Status.LoadBalancer.Ingress[0].Hostname != "web--default."+domain {
		t.Fatalf("hostname: got %q", got.Status.LoadBalancer.Ingress[0].Hostname)
	}
}

func httptestNew(t *testing.T, store *kine.Client, req *http.Request) int {
	t.Helper()
	w := &codeWriter{header: http.Header{}}
	ProvisionServices(w, req, store)
	return w.code
}

type codeWriter struct {
	header http.Header
	code   int
}

func (w *codeWriter) Header() http.Header { return w.header }
func (w *codeWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return len(b), nil
}
func (w *codeWriter) WriteHeader(status int) { w.code = status }

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
