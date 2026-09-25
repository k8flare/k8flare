//go:build !js

package apiserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestServeAPIRoot(t *testing.T) {
	legacy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("legacy"))
	})
	rest := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("core"))
	})
	h := serveAPIRoot(legacy, rest)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/", nil))
	if rec.Body.String() != "legacy" {
		t.Fatalf("api slash=%q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces", nil))
	if rec.Body.String() != "core" {
		t.Fatalf("api v1=%q", rec.Body.String())
	}
}

func TestLegacyRootAPIVersions(t *testing.T) {
	addresses := discovery.DefaultAddresses{DefaultAddress: "k8flare"}
	legacyAPI := wrapAggregated(discovery.NewLegacyRootAPIHandler(addresses, scheme.Codecs, "/api"), true, nil)
	mux := http.NewServeMux()
	mux.Handle("/api", legacyAPI)
	mux.Handle("/api/", serveAPIRoot(legacyAPI, http.NotFoundHandler()))

	for _, path := range []string{"/api", "/api/"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "application/json")
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		var versions metav1.APIVersions
		if err := json.Unmarshal(rec.Body.Bytes(), &versions); err != nil {
			t.Fatalf("%s parse: %v body=%s", path, err, rec.Body.String())
		}
		if versions.Kind != "APIVersions" || len(versions.Versions) == 0 || versions.Versions[0] != "v1" {
			t.Fatalf("%s versions=%+v", path, versions)
		}
	}
}

func TestRedirectBareProxy(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := redirectBareProxy(inner)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns/services/s/proxy?method=GET", nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("code=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/namespaces/ns/services/s/proxy/?method=GET" {
		t.Fatalf("location=%q", loc)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns/services/s/proxy/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("slashed code=%d", rec.Code)
	}
}
