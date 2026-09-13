//go:build !js

package apiserver_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func csrBytes(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		t.Fatal(err)
	}
	return csr
}

func supervisorRequest(t *testing.T, base, method, path string, body []byte, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("node", joinToken)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func firstCert(t *testing.T, pemBytes []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatalf("no PEM in %q", pemBytes)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestSupervisorJoin(t *testing.T) {
	base, cs := startDevURL(t)
	c := ctx(t)

	resp, err := http.Get(base + "/cacerts")
	if err != nil {
		t.Fatal(err)
	}
	caPEM, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || firstCert(t, caPEM).IsCA != true {
		t.Fatalf("/cacerts: %d %q", resp.StatusCode, caPEM)
	}

	if resp, err := http.Get(base + "/v1-k3s/config"); err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("config without token: %v %v", err, resp)
	}
	resp, data := supervisorRequest(t, base, http.MethodGet, "/v1-k3s/config", nil, nil)
	var cfg struct {
		HTTPSPort, SupervisorPort int
		FlannelBackend            string
		DisableKubeProxy          bool
		ClusterDomain             string
	}
	if err := json.Unmarshal(data, &cfg); err != nil || resp.StatusCode != 200 {
		t.Fatalf("config: %d %v %s", resp.StatusCode, err, data)
	}
	u, _ := url.Parse(base)
	port, _ := strconv.Atoi(u.Port())
	if cfg.HTTPSPort != port || cfg.SupervisorPort != port || cfg.FlannelBackend != "vxlan" || !cfg.DisableKubeProxy || cfg.ClusterDomain != "cluster.local" {
		t.Fatalf("config: %+v", cfg)
	}
	resp, data = supervisorRequest(t, base, http.MethodGet, "/v1-k3s/apiservers", nil, nil)
	var servers []string
	if err := json.Unmarshal(data, &servers); err != nil || len(servers) != 1 || servers[0] != u.Host {
		t.Fatalf("apiservers: %d %s", resp.StatusCode, data)
	}

	nodeHeaders := map[string]string{"k3s-Node-Name": "n1", "k3s-Node-Password": "secret-1", "k3s-Node-IP": "192.168.1.10"}
	resp, data = supervisorRequest(t, base, http.MethodPost, "/v1-k3s/client-kubelet.crt", csrBytes(t), nodeHeaders)
	if resp.StatusCode != 200 {
		t.Fatalf("client-kubelet.crt: %d %s", resp.StatusCode, data)
	}
	client := firstCert(t, data)
	if client.Subject.CommonName != "system:node:n1" || len(client.Subject.Organization) != 1 || client.Subject.Organization[0] != "system:nodes" {
		t.Fatalf("client cert subject: %v", client.Subject)
	}
	_, clientCA := supervisorRequest(t, base, http.MethodGet, "/v1-k3s/client-ca.crt", nil, nil)
	if err := client.CheckSignatureFrom(firstCert(t, clientCA)); err != nil {
		t.Fatalf("client cert not signed by client-ca: %v", err)
	}

	resp, data = supervisorRequest(t, base, http.MethodPost, "/v1-k3s/serving-kubelet.crt", csrBytes(t), nodeHeaders)
	if resp.StatusCode != 200 {
		t.Fatalf("serving-kubelet.crt: %d %s", resp.StatusCode, data)
	}
	serving := firstCert(t, data)
	if serving.Subject.CommonName != "n1" || len(serving.IPAddresses) != 3 || !serving.IPAddresses[2].Equal(net.ParseIP("192.168.1.10")) {
		t.Fatalf("serving cert: CN=%s IPs=%v DNS=%v", serving.Subject.CommonName, serving.IPAddresses, serving.DNSNames)
	}
	if err := serving.CheckSignatureFrom(firstCert(t, caPEM)); err != nil {
		t.Fatalf("serving cert not signed by server-ca: %v", err)
	}

	wrong := map[string]string{"k3s-Node-Name": "n1", "k3s-Node-Password": "other"}
	if resp, _ := supervisorRequest(t, base, http.MethodPost, "/v1-k3s/client-kubelet.crt", csrBytes(t), wrong); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong node password: want 403, got %d", resp.StatusCode)
	}
	resp, data = supervisorRequest(t, base, http.MethodPost, "/v1-k3s/client-k3s-controller.crt", csrBytes(t), nil)
	if resp.StatusCode != 200 || firstCert(t, data).Subject.CommonName != "system:k3s-controller" {
		t.Fatalf("controller cert: %d %s", resp.StatusCode, data)
	}

	nodeClient, err := kubernetes.NewForConfig(&rest.Config{Host: base, BearerToken: "node:n1:secret-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nodeClient.CoreV1().Nodes().List(c, metav1.ListOptions{}); err != nil {
		t.Fatalf("node token list nodes: %v", err)
	}
	badClient, _ := kubernetes.NewForConfig(&rest.Config{Host: base, BearerToken: "node:n1:other"})
	if _, err := badClient.CoreV1().Nodes().List(c, metav1.ListOptions{}); err == nil {
		t.Fatal("wrong node token was accepted")
	}
	unknownClient, _ := kubernetes.NewForConfig(&rest.Config{Host: base, BearerToken: "node:never-joined:whatever"})
	if _, err := unknownClient.CoreV1().Nodes().List(c, metav1.ListOptions{}); err == nil {
		t.Fatal("a node token for a node that never joined was accepted")
	}
	for _, auth := range []string{"Bearer ", "Basic " + base64.StdEncoding.EncodeToString([]byte("node:"))} {
		req, _ := http.NewRequest(http.MethodGet, base+"/v1-k3s/config", nil)
		req.Header.Set("Authorization", auth)
		if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("empty credential %q: %v %v", auth, err, resp)
		}
	}

	ssar, err := nodeClient.AuthorizationV1().SelfSubjectAccessReviews().Create(c, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authorizationv1.ResourceAttributes{Namespace: "default", Verb: "list", Group: "discovery.k8s.io", Resource: "endpointslices"}},
	}, metav1.CreateOptions{})
	if err != nil || !ssar.Status.Allowed {
		t.Fatalf("ssar: %v %+v", err, ssar)
	}
	tr, err := cs.AuthenticationV1().TokenReviews().Create(c, &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: "node:n1:secret-1"}}, metav1.CreateOptions{})
	if err != nil || !tr.Status.Authenticated || tr.Status.User.Username != "system:node:n1" {
		t.Fatalf("tokenreview: %v %+v", err, tr)
	}
	ns, err := cs.CoreV1().Namespaces().Get(c, "kube-system", metav1.GetOptions{})
	if err != nil || ns.Status.Phase != "Active" {
		t.Fatalf("kube-system namespace: %v %+v", err, ns)
	}
}
