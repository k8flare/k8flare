package workloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func TestVaultPEM(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"cert": "CERT", "key": "KEY"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "/vault/ca/client-ca" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"kv": map[string]any{"key": "/vault/ca/client-ca", "value": base64.StdEncoding.EncodeToString(body)},
		})
	}))
	defer srv.Close()
	store := &kine.Client{HTTP: srv.Client()}
	store.HTTP.Transport = rewriteHost(srv.Client().Transport, srv.URL)
	got, err := VaultPEM(context.Background(), store, "client-ca")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "CERTKEY" {
		t.Fatalf("pem = %q", got)
	}
}

type hostRewriter struct {
	base http.RoundTripper
	host string
}

func rewriteHost(base http.RoundTripper, host string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return hostRewriter{base: base, host: host}
}

func (h hostRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	u := *r.URL
	u.Scheme = "http"
	u.Host = h.host[len("http://"):]
	r = r.Clone(r.Context())
	r.URL = &u
	return h.base.RoundTrip(r)
}
