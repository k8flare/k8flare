package supervisor

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	utilnet "k8s.io/utils/net"
)

// supervisor serves the k3s supervisor protocol a k3s agent joins through
// (/cacerts and /v1-k3s/*). It is what a k3s server would answer.
type Supervisor struct {
	vault       *Vault
	joinToken   string
	ClientCerts authenticator.Request

	ServiceAccounts authenticator.Request
}

const nodeProxyUser = "system:serviceaccount:kube-system:k8flare-node-proxy"

// k3sControlConfig is the subset of k3s's config.Control the agent reads
// from /v1-k3s/config, with the same field names and types so the JSON
// matches. The real type does not build for js (it imports kine's sqlite).
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
	DisableKubeProxy   bool
	DisableNPC         bool
	DisableCCM         bool
	EgressSelectorMode string
}

var (
	ClusterCIDR = mustCIDR("10.42.0.0/16")
	ServiceCIDR = mustCIDR("10.43.0.0/16")
	ClusterDNS  = net.ParseIP("10.43.0.10")

	ClusterDomain = "cluster.local"
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func publicHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return h
	}
	return r.Host
}

func (s *Supervisor) config(r *http.Request) k3sControlConfig {
	port := 443
	if _, p, err := net.SplitHostPort(publicHost(r)); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	return k3sControlConfig{
		HTTPSPort: port, SupervisorPort: port,
		ClusterIPRange: ClusterCIDR, ServiceIPRange: ServiceCIDR,
		ClusterIPRanges: []*net.IPNet{ClusterCIDR}, ServiceIPRanges: []*net.IPNet{ServiceCIDR},
		ClusterDNS: ClusterDNS, ClusterDNSs: []net.IP{ClusterDNS}, ClusterDomain: ClusterDomain,
		FlannelBackend: "vxlan", DisableNPC: true, DisableCCM: true,
		EgressSelectorMode: "cluster",
	}
}

func (s *Supervisor) authorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		_, token, ok = r.BasicAuth()
	}
	if !ok || token == "" {
		return false
	}
	if s.vault.CheckToken(r.Context(), "join", token) == nil {
		return true
	}
	if s.vault.CheckJoinToken(r.Context(), token) == nil {
		return true
	}
	if s.joinToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.joinToken)) == 1 {
		_, _ = s.vault.EnsureToken(r.Context(), "join", s.joinToken)
		return true
	}
	return false
}

type nodeIdentity struct {
	name string
	ips  []net.IP
}

// nodeAuth checks the k3s-Node-* headers the agent sends when it asks for
// its certificates.
func (s *Supervisor) certificateNode(r *http.Request) (string, bool) {
	if s.ClientCerts == nil {
		return "", false
	}
	resp, ok, err := s.ClientCerts.AuthenticateRequest(r)
	if err != nil || !ok {
		return "", false
	}
	name, ok := strings.CutPrefix(resp.User.GetName(), "system:node:")
	if !ok || name == "" || !slices.Contains(resp.User.GetGroups(), user.NodesGroup) {
		return "", false
	}
	return name, true
}

func (s *Supervisor) tunnelNode(w http.ResponseWriter, r *http.Request) {
	if name, ok := s.certificateNode(r); ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"node": name})
		return
	}
	token := r.Header.Get("X-K8flare-Node-Token")
	ok := token != ""
	if !ok {
		token, ok = strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	rest, okName := strings.CutPrefix(token, "node:")
	name, password, okPass := strings.Cut(rest, ":")
	if !ok || !okName || !okPass || name == "" || password == "" || s.vault.CheckNodePassword(r.Context(), name, password) != nil {
		http.Error(w, "not authorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"node": name})
}

func (s *Supervisor) nodeAuth(w http.ResponseWriter, r *http.Request) (*nodeIdentity, bool) {
	name := r.Header.Get("k3s-Node-Name")
	password := r.Header.Get("k3s-Node-Password")
	if name == "" || password == "" {
		http.Error(w, "k3s-Node-Name and k3s-Node-Password headers are required", http.StatusBadRequest)
		return nil, false
	}
	if err := s.vault.RegisterNodePassword(r.Context(), name, password); err != nil {
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

func (s *Supervisor) signCSR(w http.ResponseWriter, r *http.Request, caName string, tmpl *x509.Certificate) {
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

func (s *Supervisor) caPEM(w http.ResponseWriter, r *http.Request, name string) {
	bundle, err := s.vault.CAPEM(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write(bundle)
}

const kubeletClientLifetime = 24 * time.Hour

func (s *Supervisor) KubeletClient(w http.ResponseWriter, r *http.Request) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	clientCA, err := s.vault.ca(r.Context(), "client-ca")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl := clientCertTemplate(user.APIServerUser, user.SystemPrivilegedGroup)
	tmpl.NotAfter = time.Now().Add(kubeletClientLifetime)
	cert, err := clientCA.sign(csr, tmpl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serverCA, err := s.vault.CAPEM(r.Context(), "server-ca")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"cert": string(cert),
		"key":  string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
		"ca":   string(serverCA),
	})
}

// New returns the supervisor for a cluster whose agents join with joinToken.
func New(vault *Vault, joinToken string) *Supervisor {
	return &Supervisor{vault: vault, joinToken: joinToken}
}

// Register mounts /cacerts, /ping and /v1-k3s/* on mux.
func (s *Supervisor) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /cacerts", func(w http.ResponseWriter, r *http.Request) { s.caPEM(w, r, "server-ca") })
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("pong")) })
	v1 := http.NewServeMux()
	v1.HandleFunc("GET /v1-k3s/readyz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	v1.HandleFunc("/v1-k3s/node-tunnel", s.tunnelNode)
	v1.HandleFunc("GET /v1-k3s/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.config(r))
	})
	v1.HandleFunc("GET /v1-k3s/apiservers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := publicHost(r)
		if _, _, err := net.SplitHostPort(host); err != nil {
			host = net.JoinHostPort(host, "443")
		}
		_ = json.NewEncoder(w).Encode([]string{host})
	})
	v1.HandleFunc("GET /v1-k3s/client-ca.crt", func(w http.ResponseWriter, r *http.Request) { s.caPEM(w, r, "client-ca") })
	v1.HandleFunc("GET /v1-k3s/server-ca.crt", func(w http.ResponseWriter, r *http.Request) { s.caPEM(w, r, "server-ca") })
	v1.HandleFunc("POST /v1-k3s/serving-kubernetes.crt", func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.nodeAuth(w, r)
		if !ok {
			return
		}
		clusterIP, _ := utilnet.GetIndexedIP(ServiceCIDR, 1)
		s.signCSR(w, r, "server-ca", &x509.Certificate{
			Subject: pkix.Name{CommonName: "kube-apiserver"},
			DNSNames: []string{
				id.name, "localhost",
				"kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local",
			},
			IPAddresses: append([]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), clusterIP}, id.ips...),
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
	})
	v1.HandleFunc("POST /v1-k3s/serving-node-proxy.crt", func(w http.ResponseWriter, r *http.Request) {
		if s.ServiceAccounts == nil {
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
		resp, ok, err := s.ServiceAccounts.AuthenticateRequest(r)
		if err != nil || !ok {
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
		if resp.User.GetName() != nodeProxyUser {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		clusterIP, _ := utilnet.GetIndexedIP(ServiceCIDR, 1)
		s.signCSR(w, r, "server-ca", &x509.Certificate{
			Subject:     pkix.Name{CommonName: "kube-apiserver"},
			DNSNames:    []string{"kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local"},
			IPAddresses: []net.IP{clusterIP},
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
	})
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
		if r.URL.Path != "/v1-k3s/node-tunnel" && r.URL.Path != "/v1-k3s/serving-node-proxy.crt" && !s.authorized(r) {
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
