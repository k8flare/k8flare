package supervisor

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func certsIn(t *testing.T, data []byte) []*x509.Certificate {
	t.Helper()
	var out []*x509.Certificate
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			return out
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, cert)
		data = rest
	}
}

func poolOf(certs ...*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	return pool
}

func issueKubeletClient(t *testing.T, v *Vault) []*x509.Certificate {
	t.Helper()
	rr := httptest.NewRecorder()
	New(v, "join").KubeletClient(rr, httptest.NewRequest(http.MethodPost, "/internal/kubelet-client", nil))
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code, rr.Body.String())
	}
	var out struct{ Cert string }
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return certsIn(t, []byte(out.Cert))
}

func verifyClient(chain []*x509.Certificate, roots *x509.CertPool) error {
	_, err := chain[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: poolOf(chain[1:]...),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	return err
}

func TestRotateCAKeepsOldLeavesTrusted(t *testing.T) {
	v := newMemoryVault(t)
	oldBundle, err := v.CAPEM(t.Context(), "client-ca")
	if err != nil {
		t.Fatal(err)
	}
	oldRoot := certsIn(t, oldBundle)[0]
	oldLeaf := issueKubeletClient(t, v)
	if _, err := v.RotateCAs(t.Context()); err != nil {
		t.Fatal(err)
	}
	bundle, err := v.CAPEM(t.Context(), "client-ca")
	if err != nil {
		t.Fatal(err)
	}
	roots := certsIn(t, bundle)
	if len(roots) != 2 || roots[0].Equal(oldRoot) || !roots[1].Equal(oldRoot) {
		t.Fatalf("bundle should be the new CA then the old one, got %d certificates", len(roots))
	}
	if err := verifyClient(oldLeaf, poolOf(roots...)); err != nil {
		t.Fatalf("a leaf signed before the rotation: %v", err)
	}
	newLeaf := issueKubeletClient(t, v)
	if newLeaf[0].Issuer.String() != roots[0].Subject.String() {
		t.Fatalf("new leaf issued by %s, want the new CA", newLeaf[0].Issuer)
	}
	if err := verifyClient(newLeaf, poolOf(roots...)); err != nil {
		t.Fatalf("a leaf signed after the rotation against the bundle: %v", err)
	}
	if err := verifyClient(newLeaf, poolOf(oldRoot)); err != nil {
		t.Fatalf("a node that still trusts only the old CA must accept the new leaf: %v", err)
	}
}

func TestRotateCAServesBundles(t *testing.T) {
	v := newMemoryVault(t)
	if _, err := v.RotateCAs(t.Context()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(v, "join").Register(mux)
	for _, path := range []string{"/cacerts", "/v1-k3s/server-ca.crt", "/v1-k3s/client-ca.crt"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer join")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatal(path, rr.Code)
		}
		if n := len(certsIn(t, rr.Body.Bytes())); n != 2 {
			t.Errorf("%s: %d certificates, want the new and the old CA", path, n)
		}
	}
}

func TestRotateCAKeepsTheSigningPairIntact(t *testing.T) {
	v := newMemoryVault(t)
	if _, err := v.RotateCAs(t.Context()); err != nil {
		t.Fatal(err)
	}
	kv, _, err := v.kine.Get(t.Context(), "/vault/ca/client-ca")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(kv.Value)
	var record struct{ Cert, Key string }
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair([]byte(record.Cert), []byte(record.Key)); err != nil {
		t.Fatalf("cert and key of the record must stay one pair: %v", err)
	}
	if n := len(certsIn(t, []byte(record.Cert))); n != 1 {
		t.Fatalf("cert holds %d certificates, want only the signing CA", n)
	}
}

func TestRotateCAIsSeenByOtherIsolates(t *testing.T) {
	a := newMemoryVault(t)
	b := NewVault(a.kine)
	clock := time.Now()
	b.now = func() time.Time { return clock }
	if got, _ := b.CAPEM(t.Context(), "server-ca"); len(certsIn(t, got)) != 1 {
		t.Fatal("expected one CA before the rotation")
	}
	if _, err := a.RotateCAs(t.Context()); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Hour)
	got, err := b.CAPEM(t.Context(), "server-ca")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(certsIn(t, got)); n != 2 {
		t.Fatalf("another isolate still serves %d certificates", n)
	}
}

func expiredCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "expired"},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(-24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestBundleDropsExpiredCAs(t *testing.T) {
	record, err := generateCA("k8flare-test")
	if err != nil {
		t.Fatal(err)
	}
	record.Previous = []string{expiredCA(t)}
	c, err := newCA(record)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(certsIn(t, c.bundle(time.Now()))); n != 1 {
		t.Fatalf("bundle has %d certificates, want only the live one", n)
	}
}

func TestCAStatuses(t *testing.T) {
	v := newMemoryVault(t)
	if _, err := v.RotateCAs(t.Context()); err != nil {
		t.Fatal(err)
	}
	statuses, err := v.CAStatuses(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 6 {
		t.Fatalf("got %d entries, want the new and the old of all three CAs", len(statuses))
	}
	current := map[string]int{}
	for _, s := range statuses {
		if s.Current {
			current[s.Name]++
		}
		if s.Expired || !s.NotAfter.After(time.Now().Add(9*365*24*time.Hour)) {
			t.Errorf("%s: unexpected expiry %v", s.Name, s.NotAfter)
		}
	}
	if current["server-ca"] != 1 || current["client-ca"] != 1 || current[RequestHeaderCAName] != 1 {
		t.Fatalf("each CA needs exactly one current entry: %v", current)
	}
}

func TestKubeletClientCertIsShortLived(t *testing.T) {
	v := newMemoryVault(t)
	leaf := issueKubeletClient(t, v)[0]
	if got := time.Until(leaf.NotAfter); got > 48*time.Hour {
		t.Fatalf("apiserver to kubelet client certificate lives %v; node-tunnel re-fetches it and it should be short lived", got)
	}
}
