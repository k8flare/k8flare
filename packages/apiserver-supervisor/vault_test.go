package supervisor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func TestTunnelNode(t *testing.T) {
	store := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/kv" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := store[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
			return
		}
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		store[body.Key] = raw
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
	}))
	t.Cleanup(srv.Close)
	v := NewVault(&kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}})
	if err := v.RegisterNodePassword(context.Background(), "n1", "secret"); err != nil {
		t.Fatal(err)
	}
	s := New(v, "join")
	mux := http.NewServeMux()
	s.Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/v1-k3s/node-tunnel", nil)
	req.Header.Set("Authorization", "Bearer node:n1:secret")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"node":"n1"`) {
		t.Fatal(rr.Code, rr.Body.String())
	}
	viaHeader := httptest.NewRequest(http.MethodPost, "/v1-k3s/node-tunnel", nil)
	viaHeader.Header.Set("X-K8flare-Node-Token", "node:n1:secret")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, viaHeader)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"node":"n1"`) {
		t.Fatal(rr.Code, rr.Body.String())
	}
	bad := httptest.NewRequest(http.MethodPost, "/v1-k3s/node-tunnel", nil)
	bad.Header.Set("Authorization", "Bearer node:n1:other")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, bad)
	if rr.Code != http.StatusUnauthorized {
		t.Fatal(rr.Code)
	}
}

func TestEnsureAndCheckToken(t *testing.T) {
	store := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/kv" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := store[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
			return
		}
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		store[body.Key] = raw
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
	}))
	t.Cleanup(srv.Close)
	v := NewVault(&kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}})
	got, err := v.EnsureToken(context.Background(), "join", "seed-token")
	if err != nil || got != "seed-token" {
		t.Fatalf("ensure: %q %v", got, err)
	}
	again, err := v.EnsureToken(context.Background(), "join", "other")
	if err != nil || again != "seed-token" {
		t.Fatalf("keep first: %q %v", again, err)
	}
	if err := v.CheckToken(context.Background(), "join", "seed-token"); err != nil {
		t.Fatal(err)
	}
	if err := v.CheckToken(context.Background(), "join", "wrong"); err == nil {
		t.Fatal("expected mismatch")
	}
}

type rewrite struct {
	base string
	next http.RoundTripper
}

func (h rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := http.NewRequest(req.Method, h.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	u.Header = req.Header
	return h.next.RoundTrip(u)
}
