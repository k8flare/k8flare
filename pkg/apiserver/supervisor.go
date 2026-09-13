package apiserver

import (
	"context"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"k8s.io/apiserver/pkg/authentication/user"
)

// supervisor serves the k3s supervisor protocol a k3s agent joins through
// (/cacerts and /v1-k3s/*). It is what a k3s server would answer.
type supervisor struct {
	vault     *vault
	joinToken string
}

type k3sControlConfig struct {
	HTTPSPort          int
	SupervisorPort     int
	ClusterIPRange     *net.IPNet
	ServiceIPRange     *net.IPNet
	ClusterIPRanges    []*net.IPNet
	ServiceIPRanges    []*net.IPNet
	ClusterDNS         net.IP
	ClusterDNSs        []net.IP
	ClusterDomain      string
	FlannelBackend     string
	FlannelIPv6Masq    bool
	FlannelExternalIP  bool
	DisableKubeProxy   bool
	DisableNPC         bool
	DisableCCM         bool
	EgressSelectorMode string
	SupervisorMetrics  bool
	EmbeddedRegistry   bool
	MinTLSVersion      string
	CipherSuites       []string
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func (s *supervisor) config(r *http.Request) k3sControlConfig {
	port := 443
	if _, p, err := net.SplitHostPort(r.Host); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	cluster, service := mustCIDR("10.42.0.0/16"), mustCIDR("10.43.0.0/16")
	dns := net.ParseIP("10.43.0.10")
	return k3sControlConfig{
		HTTPSPort: port, SupervisorPort: port,
		ClusterIPRange: cluster, ServiceIPRange: service,
		ClusterIPRanges: []*net.IPNet{cluster}, ServiceIPRanges: []*net.IPNet{service},
		ClusterDNS: dns, ClusterDNSs: []net.IP{dns}, ClusterDomain: "cluster.local",
		FlannelBackend: "vxlan", DisableKubeProxy: true, DisableNPC: true, DisableCCM: true,
		EgressSelectorMode: "agent",
	}
}

func (s *supervisor) authorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		_, token, ok = r.BasicAuth()
	}
	return ok && token != "" && s.joinToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.joinToken)) == 1
}

type nodeIdentity struct {
	name string
	ips  []net.IP
}

// nodeAuth checks the k3s-Node-* headers the agent sends when it asks for
// its certificates.
func (s *supervisor) nodeAuth(w http.ResponseWriter, r *http.Request) (*nodeIdentity, bool) {
	name := r.Header.Get("k3s-Node-Name")
	password := r.Header.Get("k3s-Node-Password")
	if name == "" || password == "" {
		http.Error(w, "k3s-Node-Name and k3s-Node-Password headers are required", http.StatusBadRequest)
		return nil, false
	}
	if err := s.vault.registerNodePassword(r.Context(), name, password); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, errNodePasswordMismatch) {
			code = http.StatusForbidden
		}
		http.Error(w, err.Error(), code)
		return nil, false
	}
	id := &nodeIdentity{name: name}
	for _, raw := range strings.Split(r.Header.Get("k3s-Node-IP"), ",") {
		if ip := net.ParseIP(strings.TrimSpace(raw)); ip != nil {
			id.ips = append(id.ips, ip)
		}
	}
	return id, true
}

func (s *supervisor) signCSR(w http.ResponseWriter, r *http.Request, caName string, tmpl *x509.Certificate) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	csr, err := x509.ParseCertificateRequest(body)
	if err != nil {
		http.Error(w, "a certificate signing request is required: "+err.Error(), http.StatusBadRequest)
		return
	}
	ca, err := s.vault.ca(r.Context(), caName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cert, err := ca.sign(csr, tmpl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write(cert)
}

func (s *supervisor) caPEM(w http.ResponseWriter, r *http.Request, name string) {
	ca, err := s.vault.ca(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write(ca.certPEM)
}

func (s *supervisor) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /cacerts", func(w http.ResponseWriter, r *http.Request) { s.caPEM(w, r, "server-ca") })
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("pong")) })
	v1 := http.NewServeMux()
	v1.HandleFunc("GET /v1-k3s/readyz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	v1.HandleFunc("GET /v1-k3s/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.config(r))
	})
	v1.HandleFunc("GET /v1-k3s/apiservers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{r.Host})
	})
	v1.HandleFunc("GET /v1-k3s/client-ca.crt", func(w http.ResponseWriter, r *http.Request) { s.caPEM(w, r, "client-ca") })
	v1.HandleFunc("GET /v1-k3s/server-ca.crt", func(w http.ResponseWriter, r *http.Request) { s.caPEM(w, r, "server-ca") })
	v1.HandleFunc("POST /v1-k3s/serving-kubelet.crt", func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.nodeAuth(w, r)
		if !ok {
			return
		}
		s.signCSR(w, r, "server-ca", &x509.Certificate{
			Subject:     pkix.Name{CommonName: id.name},
			DNSNames:    []string{id.name, "localhost"},
			IPAddresses: append([]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, id.ips...),
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
	})
	v1.HandleFunc("POST /v1-k3s/client-kubelet.crt", func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.nodeAuth(w, r)
		if !ok {
			return
		}
		s.signCSR(w, r, "client-ca", clientCertTemplate("system:node:"+id.name, user.NodesGroup))
	})
	v1.HandleFunc("POST /v1-k3s/client-kube-proxy.crt", func(w http.ResponseWriter, r *http.Request) {
		s.signCSR(w, r, "client-ca", clientCertTemplate(user.KubeProxy))
	})
	v1.HandleFunc("POST /v1-k3s/client-k3s-controller.crt", func(w http.ResponseWriter, r *http.Request) {
		s.signCSR(w, r, "client-ca", clientCertTemplate("system:k3s-controller"))
	})
	mux.Handle("/v1-k3s/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
		v1.ServeHTTP(w, r)
	}))
}

func clientCertTemplate(cn string, orgs ...string) *x509.Certificate {
	return &x509.Certificate{
		Subject:     pkix.Name{CommonName: cn, Organization: orgs},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
}

// nodeAuthenticator accepts the token cmd/agent writes into the kubelet's
// kubeconfig: "node:<name>:<node password>", the same secret the
// supervisor verified when it signed the node's certificates.
func nodeAuthenticator(v *vault) Authenticator {
	return func(ctx context.Context, token string) *user.DefaultInfo {
		rest, ok := strings.CutPrefix(token, "node:")
		if !ok {
			return nil
		}
		name, password, ok := strings.Cut(rest, ":")
		if !ok || name == "" || password == "" {
			return nil
		}
		if err := v.checkNodePassword(ctx, name, password); err != nil {
			return nil
		}
		return &user.DefaultInfo{Name: "system:node:" + name, Groups: []string{user.NodesGroup, user.AllAuthenticated}}
	}
}

func decodeBase64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
