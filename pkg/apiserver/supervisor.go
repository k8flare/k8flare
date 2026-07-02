package apiserver

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// clusterConfig holds the k3s cluster configuration returned by the /v1-k3s/config endpoint.
// Field names use PascalCase (no JSON tags) to match k3s's config.Control struct.
// Fields with complex Go types (net.IPNet, utilnet.PortRange, net.IP) are omitted
// because they require special JSON serialization that differs from plain strings.
// The agent handles nil/zero values for these fields gracefully.
type clusterConfig struct {
	ClusterDomain      string    `json:"ClusterDomain,omitempty"`
	ClusterIPRange     net.IPNet `json:"ClusterIPRange"`
	ServiceIPRange     net.IPNet `json:"ServiceIPRange"`
	HTTPSPort          int       `json:"HTTPSPort,omitempty"`
	SupervisorPort     int       `json:"SupervisorPort,omitempty"`
	DisableCCM         bool      `json:"DisableCCM,omitempty"`
	DisableNPC         bool      `json:"DisableNPC,omitempty"`
	DisableKubeProxy   bool      `json:"DisableKubeProxy,omitempty"`
	DisableServiceLB   bool      `json:"DisableServiceLB,omitempty"`
	FlannelBackend     string    `json:"FlannelBackend,omitempty"`
	EgressSelectorMode string    `json:"EgressSelectorMode,omitempty"`
	NoFlannel          bool      `json:"NoFlannel,omitempty"`
}

// defaultClusterConfig returns the default cluster configuration.
// FlannelBackend is "host-gw" because all EC2 agents are in the same VPC
// subnet, so no encapsulation is needed. The agent sets up flannel and CNI
// automatically using the PodCIDR allocated by the control plane.
func defaultClusterConfig() clusterConfig {
	_, clusterCIDR, _ := net.ParseCIDR("10.42.0.0/16")
	_, serviceCIDR, _ := net.ParseCIDR("10.43.0.0/16")
	return clusterConfig{
		ClusterDomain:  "cluster.local",
		ClusterIPRange: *clusterCIDR,
		ServiceIPRange: *serviceCIDR,
		HTTPSPort:      6443,
		SupervisorPort: 6443,
		DisableCCM:     true,
		DisableNPC:     true,
		// Kept disabled: enabling this reproducibly hangs the Worker/DO within
		// seconds (Cloudflare's own "Workers runtime canceled this request
		// because it detected that your Worker's code had hung" error),
		// confirmed by direct A/B testing (enabled vs disabled, all else
		// equal) both locally and in CI. Not yet root-caused -- see
		// docs/general-purpose-k8s-plan.md's Phase 1 section for the
		// investigation notes and what to try next.
		DisableKubeProxy:   true,
		DisableServiceLB:   true,
		FlannelBackend:     "host-gw",
		EgressSelectorMode: "disabled",
	}
}

// RegisterSupervisorHandlers registers all k3s supervisor protocol endpoints on the mux.
// These endpoints are called by k3s agents during bootstrap to obtain CA certificates,
// cluster configuration, signed certificates, and server information.
func RegisterSupervisorHandlers(mux *http.ServeMux, cam *CAManager, storage *Storage, tokenFn TokenFunc) {
	// 1. /cacerts — unauthenticated, returns empty so the agent uses system CAs.
	// Cloudflare terminates TLS with a publicly trusted certificate. If we returned
	// our self-signed CA here, the agent would use it as the sole trust root and
	// reject Cloudflare's certificate. An empty response makes the agent fall back
	// to the OS CA bundle, which trusts Cloudflare's cert chain.
	// The CA used in kubeconfigs comes from /v1-k3s/server-ca.crt (separate endpoint).
	mux.HandleFunc("GET /cacerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	})

	// 2. /v1-k3s/config — returns cluster configuration JSON
	mux.HandleFunc("GET /v1-k3s/config", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, defaultClusterConfig())
	}))

	// 3. /v1-k3s/client-ca.crt — returns client CA cert
	mux.HandleFunc("GET /v1-k3s/client-ca.crt", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write(cam.ClientCACertPEM())
	}))

	// 4. /v1-k3s/server-ca.crt — returns server CA cert
	mux.HandleFunc("GET /v1-k3s/server-ca.crt", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write(cam.ServerCACertPEM())
	}))

	// 5. /v1-k3s/serving-kubelet.crt — sign serving cert (requires node auth)
	mux.HandleFunc("POST /v1-k3s/serving-kubelet.crt", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !validateNodeAuth(w, r, storage) {
			return
		}
		handleCertSign(w, r, cam, storage, "serving-kubelet")
	}))

	// 6. /v1-k3s/client-kubelet.crt — sign client kubelet cert (requires node auth)
	mux.HandleFunc("POST /v1-k3s/client-kubelet.crt", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !validateNodeAuth(w, r, storage) {
			return
		}
		handleCertSign(w, r, cam, storage, "client-kubelet")
	}))

	// 7. /v1-k3s/client-kube-proxy.crt — sign client kube-proxy cert
	mux.HandleFunc("POST /v1-k3s/client-kube-proxy.crt", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		handleCertSign(w, r, cam, storage, "client-kube-proxy")
	}))

	// 8. /v1-k3s/client-k3s-controller.crt — sign client k3s-controller cert
	mux.HandleFunc("POST /v1-k3s/client-k3s-controller.crt", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		handleCertSign(w, r, cam, storage, "client-k3s-controller")
	}))

	// 9. /v1-k3s/apiservers — returns list of API server URLs
	mux.HandleFunc("GET /v1-k3s/apiservers", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Cloudflare always terminates TLS in front of the Worker, so the
		// agent-facing URL is always https regardless of how this request
		// itself arrived (e.g. plain HTTP under `wrangler dev` locally).
		json.NewEncoder(w).Encode([]string{"https://" + r.Host})
	}))

	// 10. /v1-k3s/readyz — readiness check
	mux.HandleFunc("GET /v1-k3s/readyz", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// /v1-k3s/tls-cert — generate TLS cert signed by server CA (for socat proxy)
	mux.HandleFunc("GET /v1-k3s/tls-cert", supervisorAuth(tokenFn, func(w http.ResponseWriter, r *http.Request) {
		if err := cam.Initialize(r.Context()); err != nil {
			http.Error(w, "failed to initialize CA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dnsNames := []string{"localhost", "k3s-proxy", "host.docker.internal", "host.lima.internal"}
		ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		certPEM, keyPEM, err := cam.GenerateTLSCertAndKey(dnsNames, ips)
		if err != nil {
			http.Error(w, "failed to generate TLS cert: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write(certPEM)
		w.Write([]byte("---KEY---\n"))
		w.Write(keyPEM)
	}))
}

// supervisorAuth is a middleware that validates k3s agent authentication.
// k3s agents authenticate using Basic Auth (password=<token>) or Bearer token.
// The username in Basic Auth is accepted as long as the password matches.
func supervisorAuth(tokenFn TokenFunc, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := tokenFn()
		// Check Basic Auth: any username, password must match token
		if _, password, ok := r.BasicAuth(); ok {
			if password == token {
				next(w, r)
				return
			}
		}

		// Check Bearer token
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			bearerToken := strings.TrimPrefix(authHeader, "Bearer ")
			if bearerToken == token {
				next(w, r)
				return
			}
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

// validateNodeAuth checks node identity headers and validates the node password.
// Returns true if validation passes, false if it fails (and writes the error response).
func validateNodeAuth(w http.ResponseWriter, r *http.Request, storage *Storage) bool {
	nodeName := r.Header.Get("K3s-Node-Name")
	nodePassword := r.Header.Get("K3s-Node-Password")

	if nodeName == "" {
		http.Error(w, "missing K3s-Node-Name header", http.StatusForbidden)
		return false
	}

	if err := ValidateNodePassword(r.Context(), storage, nodeName, nodePassword); err != nil {
		log.Printf("node auth failed for %s: %v", nodeName, err)
		http.Error(w, "node authentication failed", http.StatusForbidden)
		return false
	}

	return true
}

// handleCertSign reads a CSR from the request body, signs it with the appropriate CA,
// and returns the signed certificate as PEM.
func handleCertSign(w http.ResponseWriter, r *http.Request, cam *CAManager, storage *Storage, certType string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		http.Error(w, "empty CSR body", http.StatusBadRequest)
		return
	}

	var certPEM []byte

	switch certType {
	case "serving-kubelet":
		nodeName := r.Header.Get("K3s-Node-Name")
		nodeIPHeader := r.Header.Get("K3s-Node-IP")
		nodeIPs := parseNodeIPs(nodeIPHeader)
		certPEM, err = cam.SignServingCert(body, nodeName, nodeIPs)

	case "client-kubelet":
		nodeName := r.Header.Get("K3s-Node-Name")
		certPEM, err = cam.SignClientCert(body, "system:node:"+nodeName, []string{"system:nodes"})

	case "client-kube-proxy":
		certPEM, err = cam.SignClientCert(body, "system:kube-proxy", nil)

	case "client-k3s-controller":
		certPEM, err = cam.SignClientCert(body, "system:k3s-controller", nil)

	default:
		http.Error(w, "unknown cert type: "+certType, http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Printf("failed to sign %s cert: %v", certType, err)
		http.Error(w, "failed to sign certificate", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write(certPEM)
}

// parseNodeIPs parses a comma-separated list of IP addresses from the K3s-Node-IP header.
func parseNodeIPs(header string) []net.IP {
	if header == "" {
		return nil
	}

	parts := strings.Split(header, ",")
	ips := make([]net.IP, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		ip := net.ParseIP(part)
		if ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}
