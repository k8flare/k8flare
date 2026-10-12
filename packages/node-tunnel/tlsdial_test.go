package nodetunnel

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func handshakeCountingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var handshakes atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("converted"))
	}))
	srv.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		handshakes.Add(1)
		return nil, nil
	}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, &handshakes
}

func servingCABundle(srv *httptest.Server) string {
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
}

func convertThrough(transports *tlsTransports, ep tlsEndpoint, srv *httptest.Server) (tlsDialTimes, error) {
	req := httptest.NewRequest(http.MethodPost, "http://"+srv.Listener.Addr().String()+"/convert", strings.NewReader("{}"))
	req.RequestURI = ""
	resp, times, err := transports.roundTrip(req, ep, "", (&net.Dialer{}).DialContext)
	if err != nil {
		return times, err
	}
	defer resp.Body.Close()
	if body, _ := io.ReadAll(resp.Body); string(body) != "converted" {
		return times, io.ErrUnexpectedEOF
	}
	return times, nil
}

func TestTLSDialsToOneEndpointHandshakeOnceAndAgainWhenItsAddressChanges(t *testing.T) {
	first, firstHandshakes := handshakeCountingServer(t)
	moved, movedHandshakes := handshakeCountingServer(t)
	ep := tlsEndpoint{node: "n1", serverName: "example.com", caBundle: servingCABundle(first)}
	var transports tlsTransports
	reused := 0
	for i := range 5 {
		times, err := convertThrough(&transports, ep, first)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if times.reused {
			reused++
		}
	}
	if got := firstHandshakes.Load(); got != 1 {
		t.Fatalf("5 sequential calls to one endpoint performed %d TLS handshakes, want 1", got)
	}
	if reused != 4 {
		t.Fatalf("%d of 5 calls reported a reused connection, want 4", reused)
	}
	if _, err := convertThrough(&transports, ep, moved); err != nil {
		t.Fatal(err)
	}
	if got := movedHandshakes.Load(); got != 1 {
		t.Fatalf("the call after the endpoint moved performed %d TLS handshakes on the new address, want 1", got)
	}
	if got := firstHandshakes.Load(); got != 1 {
		t.Fatalf("the old address saw %d TLS handshakes, want 1", got)
	}
}

func TestTLSDialsDoNotReuseAConnectionVerifiedForAnotherServerName(t *testing.T) {
	srv, handshakes := handshakeCountingServer(t)
	var transports tlsTransports
	trusted := tlsEndpoint{node: "n1", serverName: "example.com", caBundle: servingCABundle(srv)}
	if _, err := convertThrough(&transports, trusted, srv); err != nil {
		t.Fatal(err)
	}
	wrongName := trusted
	wrongName.serverName = "other.example.org"
	if _, err := convertThrough(&transports, wrongName, srv); err == nil {
		t.Fatal("a call naming a server the certificate does not cover succeeded")
	}
	if got := handshakes.Load(); got != 2 {
		t.Fatalf("%d TLS handshakes, want 2: the second name must be verified on its own connection", got)
	}
}
