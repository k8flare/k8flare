package nodetunnel

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

const (
	idleTLSConnsPerHost = 25
	idleTLSConnTimeout  = 10 * time.Second
)

type dialFunc = func(ctx context.Context, network, address string) (net.Conn, error)

type tlsEndpoint struct {
	node, serverName, caBundle, clientCert string
}

type tlsDialTimes struct {
	reused                       bool
	connect, handshake, exchange time.Duration
}

type dialedConn struct {
	*tls.Conn
	connect, handshake time.Duration
}

type tlsTransports struct {
	mu         sync.Mutex
	byEndpoint map[tlsEndpoint]*http.Transport
}

func (t *tlsTransports) roundTrip(req *http.Request, ep tlsEndpoint, clientKey string, dial dialFunc) (*http.Response, tlsDialTimes, error) {
	var times tlsDialTimes
	transport, err := t.transport(ep, clientKey, dial)
	if err != nil {
		return nil, times, err
	}
	var gotConn time.Time
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		gotConn = time.Now()
		times.reused = info.Reused
		if conn, ok := info.Conn.(*dialedConn); ok && !info.Reused {
			times.connect, times.handshake = conn.connect, conn.handshake
		}
	}}
	out := req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	out.URL.Scheme = "https"
	resp, err := transport.RoundTrip(out)
	if err != nil {
		return nil, times, err
	}
	times.exchange = time.Since(gotConn)
	return resp, times, nil
}

func (t *tlsTransports) transport(ep tlsEndpoint, clientKey string, dial dialFunc) (*http.Transport, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if transport, ok := t.byEndpoint[ep]; ok {
		return transport, nil
	}
	cfg, err := dialTLSConfig(ep.serverName, ep.caBundle, ep.clientCert, clientKey)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			start := time.Now()
			raw, err := dial(ctx, network, address)
			if err != nil {
				return nil, err
			}
			connected := time.Now()
			conn := tls.Client(raw, cfg)
			if err := conn.HandshakeContext(ctx); err != nil {
				raw.Close()
				return nil, err
			}
			return &dialedConn{Conn: conn, connect: connected.Sub(start), handshake: time.Since(connected)}, nil
		},
		DisableCompression:  true,
		MaxIdleConnsPerHost: idleTLSConnsPerHost,
		IdleConnTimeout:     idleTLSConnTimeout,
	}
	if t.byEndpoint == nil {
		t.byEndpoint = map[tlsEndpoint]*http.Transport{}
	}
	t.byEndpoint[ep] = transport
	return transport, nil
}

func dialTLSConfig(serverName, caHeader, clientCertPEM, clientKeyPEM string) (*tls.Config, error) {
	cfg := &tls.Config{ServerName: serverName}
	if caHeader != "" {
		pool := x509.NewCertPool()
		pem := []byte(caHeader)
		if decoded, err := base64.StdEncoding.DecodeString(caHeader); err == nil && bytes.Contains(decoded, []byte("-----BEGIN")) {
			pem = decoded
		}
		pool.AppendCertsFromPEM(pem)
		cfg.RootCAs = pool
	} else {
		cfg.InsecureSkipVerify = true
	}
	if clientCertPEM != "" && clientKeyPEM != "" {
		cert, err := tls.X509KeyPair([]byte(clientCertPEM), []byte(clientKeyPEM))
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
