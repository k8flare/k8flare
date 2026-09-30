package apiserver

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func healthMux(datastoreUp bool) *http.ServeMux {
	client := &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !datastoreUp {
			return nil, fmt.Errorf("cluster unreachable")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"revision":7}`)), Header: http.Header{}, Request: r}, nil
	})}}
	mux := http.NewServeMux()
	installHealth(mux, client)
	return mux
}

func getHealth(mux http.Handler, target string) (int, string) {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Code, rec.Body.String()
}

func TestHealthReportsDatastore(t *testing.T) {
	up, down := healthMux(true), healthMux(false)
	cases := []struct {
		name   string
		mux    http.Handler
		target string
		code   int
		body   string
	}{
		{"readyz ok", up, "/readyz", 200, "ok"},
		{"healthz ok", up, "/healthz", 200, "ok"},
		{"livez ok", up, "/livez", 200, "ok"},
		{"readyz verbose", up, "/readyz?verbose", 200, "[+]ping ok\n[+]datastore ok\nreadyz check passed\n"},
		{"livez verbose", up, "/livez?verbose", 200, "[+]ping ok\nlivez check passed\n"},
		{"readyz down", down, "/readyz", 500, "[+]ping ok\n[-]datastore failed: reason withheld\nreadyz check failed\n"},
		{"healthz down", down, "/healthz", 500, "[+]ping ok\n[-]datastore failed: reason withheld\nhealthz check failed\n"},
		{"livez down", down, "/livez", 200, "ok"},
		{"readyz exclude", down, "/readyz?exclude=datastore", 200, "ok"},
		{"readyz datastore ok", up, "/readyz/datastore", 200, "ok"},
		{"readyz datastore down", down, "/readyz/datastore", 500, "cluster unreachable"},
		{"healthz ping", down, "/healthz/ping", 200, "ok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := getHealth(c.mux, c.target)
			if code != c.code {
				t.Fatalf("status %d, want %d; body %q", code, c.code, body)
			}
			if !strings.Contains(body, c.body) {
				t.Fatalf("body %q, want it to contain %q", body, c.body)
			}
		})
	}
}
