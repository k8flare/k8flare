//go:build js && wasm

package nodetunnel

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/gorilla/websocket"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"github.com/rancher/remotedialer"
)

var server = remotedialer.New(
	func(*http.Request) (string, bool, error) { return "", false, nil },
	remotedialer.DefaultErrorWriter,
)

var (
	mu      sync.Mutex
	current *session
)

type session struct {
	nodeName string
	send     js.Value
	frames   chan []byte
	mu       sync.Mutex
	closed   bool
}

func newSession(nodeName string, send js.Value) *session {
	return &session{nodeName: nodeName, send: send, frames: make(chan []byte, 64)}
}

func (s *session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.frames)
	return nil
}

func (s *session) NextReader() (int, io.Reader, error) {
	b, ok := <-s.frames
	if !ok {
		return 0, nil, io.EOF
	}
	return websocket.BinaryMessage, bytes.NewReader(b), nil
}

func (s *session) WriteControl(int, time.Time, []byte) error { return nil }

func (s *session) WriteMessage(_ int, _ time.Time, data []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return io.ErrClosedPipe
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	s.send.Invoke(arr)
	return nil
}

func (s *session) push(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.frames <- data:
	default:
	}
}
func Serve() {
	binding := js.Global().Get("context").Get("binding")
	binding.Set("attach", js.FuncOf(func(_ js.Value, args []js.Value) any {
		nodeName := args[0].String()
		send := args[1]
		s := newSession(nodeName, send)
		mu.Lock()
		current = s
		mu.Unlock()
		go func() {
			_ = server.ServeConn(nodeName, s)
			mu.Lock()
			if current == s {
				current = nil
			}
			mu.Unlock()
		}()
		return nil
	}))
	binding.Set("message", js.FuncOf(func(_ js.Value, args []js.Value) any {
		mu.Lock()
		s := current
		mu.Unlock()
		if s == nil {
			return nil
		}
		data := make([]byte, args[0].Get("byteLength").Int())
		js.CopyBytesToGo(data, args[0])
		s.push(data)
		return nil
	}))
	binding.Set("closed", js.FuncOf(func(js.Value, []js.Value) any {
		mu.Lock()
		s := current
		current = nil
		mu.Unlock()
		if s != nil {
			_ = s.Close()
		}
		return nil
	}))
	bridge.Serve(handler())
}
func handler() http.Handler {
	target := &url.URL{Scheme: "https", Host: "127.0.0.1:10250"}
	tlsConfig, bearer := kubeletCredentials()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = pr.In.URL.Path
			pr.Out.URL.RawPath = ""
			if bearer != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+bearer)
			}
		},
		FlushInterval: -1,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nodeName, kubeletPath, ok := parseNodePath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = kubeletPath
		dial := server.Dialer(nodeName)
		proxy.Transport = &http.Transport{
			DialTLSContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				raw, err := dial(ctx, network, "127.0.0.1:10250")
				if err != nil {
					return nil, err
				}
				tconn := tls.Client(raw, tlsConfig)
				if err := tconn.HandshakeContext(ctx); err != nil {
					raw.Close()
					return nil, err
				}
				return tconn, nil
			},
		}
		proxy.ServeHTTP(w, r)
	})
}

func parseNodePath(p string) (nodeName, kubeletPath string, ok bool) {
	rest, ok := strings.CutPrefix(p, "/node/")
	if !ok {
		return "", "", false
	}
	nodeName, kubeletPath, ok = strings.Cut(rest, "/")
	if !ok {
		return "", "", false
	}
	return nodeName, "/" + kubeletPath, true
}

func kubeletCredentials() (*tls.Config, string) {
	certPEM := bridge.Getenv("KUBELET_CLIENT_CERT")
	keyPEM := bridge.Getenv("KUBELET_CLIENT_KEY")
	if certPEM == "" || keyPEM == "" {
		return &tls.Config{InsecureSkipVerify: true}, bridge.Getenv("ADMIN_TOKEN")
	}
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return &tls.Config{InsecureSkipVerify: true}, bridge.Getenv("ADMIN_TOKEN")
	}
	pool := x509.NewCertPool()
	if caPEM := bridge.Getenv("KUBELET_CA"); caPEM != "" {
		pool.AppendCertsFromPEM([]byte(caPEM))
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, RootCAs: pool}, ""
}
