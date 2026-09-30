package supervisor

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func newMemoryVault(t *testing.T) *Vault {
	store := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := store[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
				return
			}
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
	return NewVault(&kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}})
}

func TestKubeletClientIssuesPrivilegedClientCert(t *testing.T) {
	v := newMemoryVault(t)
	rr := httptest.NewRecorder()
	New(v, "join").KubeletClient(rr, httptest.NewRequest(http.MethodPost, "/internal/kubelet-client", nil))
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code, rr.Body.String())
	}
	var out struct{ Cert, Key, CA string }
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair([]byte(out.Cert), []byte(out.Key))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "system:apiserver" || len(leaf.Subject.Organization) != 1 || leaf.Subject.Organization[0] != "system:masters" {
		t.Fatalf("subject %v", leaf.Subject)
	}
	clientCA, err := v.CAPEM(t.Context(), "client-ca")
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(clientCA)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatal(err)
	}
	serverCA, err := v.CAPEM(t.Context(), "server-ca")
	if err != nil {
		t.Fatal(err)
	}
	if out.CA != string(serverCA) {
		t.Fatal("ca is not the server CA")
	}
	if block, _ := pem.Decode([]byte(out.CA)); block == nil {
		t.Fatal("ca is not PEM")
	}
}
