//go:build !js

package apiserver_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rancher/remotedialer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/proxy"
	translator "k8s.io/apiserver/pkg/util/proxy"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	criportforward "k8s.io/cri-streaming/pkg/streaming/portforward"
	criremotecommand "k8s.io/cri-streaming/pkg/streaming/remotecommand"
	"k8s.io/streaming/pkg/httpstream/wsstream"
)

func TestNodeStreamsOverWebSocket(t *testing.T) {
	base, cs := startDevURL(t)
	c, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	joinNodeTunnel(t, base, "n1", serveKubelet(t, base, "n1"))
	createNode(t, cs, c, "n1")
	if _, err := cs.CoreV1().Pods("default").Create(c, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "streams"},
		Spec:       corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{Name: "c", Image: "img"}}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	streamConfig := &rest.Config{Host: base, BearerToken: devToken}

	t.Run("port-forward", func(t *testing.T) {
		reply, err := retryUntilTunnelIsServed(c, func() (string, error) { return portForwardOnce(c, cs, streamConfig) })
		if err != nil {
			t.Fatal(err)
		}
		if reply != "streams:8080 ping\n" {
			t.Fatalf("port-forward reply %q, want %q", reply, "streams:8080 ping\n")
		}
	})

	t.Run("attach", func(t *testing.T) {
		reply, err := retryUntilTunnelIsServed(c, func() (string, error) { return attachOnce(c, cs, streamConfig) })
		if err != nil {
			t.Fatal(err)
		}
		if reply != "streams/c: hello\n" {
			t.Fatalf("attach stdout %q, want %q", reply, "streams/c: hello\n")
		}
	})
}

func retryUntilTunnelIsServed(c context.Context, try func() (string, error)) (string, error) {
	for {
		reply, err := try()
		if err == nil || c.Err() != nil {
			return reply, err
		}
		select {
		case <-c.Done():
			return reply, err
		case <-time.After(2 * time.Second):
		}
	}
}

func portForwardOnce(c context.Context, cs *kubernetes.Clientset, config *rest.Config) (string, error) {
	req := cs.CoreV1().RESTClient().Post().Resource("pods").Namespace("default").Name("streams").SubResource("portforward")
	dialer, err := portforward.NewSPDYOverWebsocketDialerForStreaming(req.URL(), config)
	if err != nil {
		return "", err
	}
	stop, ready := make(chan struct{}), make(chan struct{})
	defer close(stop)
	var errOut bytes.Buffer
	fw, err := portforward.NewOnAddressesForStreaming(dialer, []string{"127.0.0.1"}, []string{"0:8080"}, stop, ready, io.Discard, &errOut)
	if err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- fw.ForwardPorts() }()
	select {
	case <-ready:
	case err := <-done:
		return "", fmt.Errorf("port-forward ended before it was ready: %v", err)
	case <-c.Done():
		return "", c.Err()
	}
	ports, err := fw.GetPorts()
	if err != nil {
		return "", err
	}
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ports[0].Local))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		return "", err
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return reply, fmt.Errorf("reading the forwarded port: %v (port-forward errors: %s)", err, errOut.String())
	}
	return reply, nil
}

func attachOnce(c context.Context, cs *kubernetes.Clientset, config *rest.Config) (string, error) {
	req := cs.CoreV1().RESTClient().Post().Resource("pods").Namespace("default").Name("streams").SubResource("attach").
		VersionedParams(&corev1.PodAttachOptions{Container: "c", Stdin: true, Stdout: true, Stderr: true}, scheme.ParameterCodec)
	exec, err := remotecommand.NewWebSocketExecutor(config, "GET", req.URL().String())
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	streamCtx, cancel := context.WithTimeout(c, 30*time.Second)
	defer cancel()
	err = exec.StreamWithContext(streamCtx, remotecommand.StreamOptions{Stdin: strings.NewReader("hello\n"), Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		return stdout.String(), fmt.Errorf("attach: %v (stderr %q)", err, stderr.String())
	}
	return stdout.String(), nil
}

type emulatedRuntime struct{}

func (emulatedRuntime) PortForward(_ context.Context, name string, _ string, port int32, stream io.ReadWriteCloser) error {
	defer stream.Close()
	if _, err := fmt.Fprintf(stream, "%s:%d ", name, port); err != nil {
		return err
	}
	_, err := io.Copy(stream, stream)
	return err
}

func (emulatedRuntime) AttachContainer(_ context.Context, name string, _ string, container string, in io.Reader, out, _ io.WriteCloser, _ bool, _ <-chan criremotecommand.TerminalSize) error {
	if _, err := fmt.Fprintf(out, "%s/%s: ", name, container); err != nil {
		return err
	}
	_, err := io.Copy(out, in)
	return err
}

type proxyErrorResponder struct{}

func (proxyErrorResponder) Error(w http.ResponseWriter, _ *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func serveKubelet(t *testing.T, base, node string) string {
	t.Helper()
	runtime := http.NewServeMux()
	runtime.HandleFunc("/portforward/{pod}", func(w http.ResponseWriter, r *http.Request) {
		criportforward.ServePortForward(w, r, emulatedRuntime{}, r.PathValue("pod"), "", nil,
			4*time.Hour, criremotecommand.DefaultStreamCreationTimeout, criportforward.SupportedProtocols)
	})
	runtime.HandleFunc("/attach/{pod}/{container}", func(w http.ResponseWriter, r *http.Request) {
		opts, err := criremotecommand.NewOptions(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		criremotecommand.ServeAttach(w, r, emulatedRuntime{}, r.PathValue("pod"), "", r.PathValue("container"), opts,
			4*time.Hour, criremotecommand.DefaultStreamCreationTimeout, criremotecommand.SupportedStreamingProtocols)
	})
	cri := httptest.NewServer(runtime)
	t.Cleanup(cri.Close)

	kubelet := http.NewServeMux()
	kubelet.HandleFunc("/portForward/{namespace}/{pod}", func(w http.ResponseWriter, r *http.Request) {
		if !wsstream.IsWebSocketRequestWithTunnelingProtocol(r) {
			http.Error(w, "want a WebSocket request with a SPDY tunneling protocol", http.StatusBadRequest)
			return
		}
		location, _ := url.Parse(cri.URL + "/portforward/" + r.PathValue("pod"))
		translator.NewTunnelingHandler(proxy.NewUpgradeAwareHandler(location, nil, false, true, proxyErrorResponder{})).ServeHTTP(w, r)
	})
	kubelet.HandleFunc("/attach/{namespace}/{pod}/{container}", func(w http.ResponseWriter, r *http.Request) {
		if !wsstream.IsWebSocketRequestWithStreamCloseProtocol(r) {
			http.Error(w, "want a WebSocket request with a stream-close protocol", http.StatusBadRequest)
			return
		}
		opts, err := criremotecommand.NewOptions(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		location, _ := url.Parse(cri.URL + "/attach/" + r.PathValue("pod") + "/" + r.PathValue("container") + "?" + r.URL.RawQuery)
		translator.NewStreamTranslatorHandler(location, nil, 0, translator.Options{
			Stdin: opts.Stdin, Stdout: opts.Stdout, Stderr: opts.Stderr, Tty: opts.TTY,
		}).ServeHTTP(w, r)
	})

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{servingCertificate(t, base, node)}})
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: kubelet}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener.Addr().String()
}

func servingCertificate(t *testing.T, base, node string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		t.Fatal(err)
	}
	resp, certPEM := supervisorRequest(t, base, http.MethodPost, "/v1-k3s/serving-kubelet.crt", csr,
		map[string]string{"k3s-Node-Name": node, "k3s-Node-Password": "secret-1", "k3s-Node-IP": "127.0.0.1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("serving-kubelet.crt: %d %s", resp.StatusCode, certPEM)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func joinNodeTunnel(t *testing.T, base, node, kubelet string) {
	t.Helper()
	tunnelCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	connected := make(chan struct{})
	headers := http.Header{"Authorization": {"Bearer node:" + node + ":secret-1"}}
	allowKubeletPort := func(_, address string) bool { return address == "127.0.0.1:10250" }
	dialKubelet := func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, kubelet)
	}
	go func() {
		_ = remotedialer.ConnectToProxyWithDialer(tunnelCtx, "ws"+strings.TrimPrefix(base, "http")+"/v1-k3s/connect", headers,
			allowKubeletPort, nil, dialKubelet, func(context.Context, *remotedialer.Session) error {
				close(connected)
				return nil
			})
	}()
	select {
	case <-connected:
	case <-time.After(time.Minute):
		t.Fatal("the node tunnel did not connect")
	}
}
