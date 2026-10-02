//go:build js && wasm

package nodetunnel

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
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
	return &session{nodeName: nodeName, send: send, frames: make(chan []byte, 4096)}
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
	cp := append([]byte(nil), data...)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	ch := s.frames
	s.mu.Unlock()
	select {
	case ch <- cp:
	default:
		go func() {
			defer func() { _ = recover() }()
			ch <- cp
		}()
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
	binding.Set("upgrade", js.FuncOf(func(_ js.Value, args []js.Value) any {
		id := args[0].String()
		rawURL := args[1].String()
		headers := args[2]
		send := args[3]
		onErr := js.Undefined()
		if len(args) > 4 {
			onErr = args[4]
		}
		n := headers.Length()
		h := http.Header{}
		for i := 0; i < n; i++ {
			pair := headers.Index(i)
			h.Add(pair.Index(0).String(), pair.Index(1).String())
		}
		query := ""
		if len(args) > 5 {
			query = args[5].String()
		}
		streamRegistry.Register(id)
		go startKubeletStream(id, rawURL, h, send, onErr, query)
		return nil
	}))
	binding.Set("upgradeMessage", js.FuncOf(func(_ js.Value, args []js.Value) any {
		id := args[0].String()
		data := make([]byte, args[1].Get("byteLength").Int())
		js.CopyBytesToGo(data, args[1])
		if w, sent := streamRegistry.Send(id, data); sent {
			_ = w.WriteMessage(websocket.BinaryMessage, data)
		}
		return nil
	}))
	binding.Set("upgradeClosed", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			id := args[0].String()
			if w := streamRegistry.Close(id); w != nil {
				_ = w.Close()
			}
		}
		return nil
	}))
	bridge.Serve(handler())
}

var streamRegistry = NewStreamRegistry()

type firstLineConn struct {
	net.Conn
	logged bool
}

func (c *firstLineConn) Write(p []byte) (int, error) {
	if !c.logged {
		c.logged = true
		line, _ := bufio.NewReader(bytes.NewReader(p)).ReadString('\n')
		println("kubelet write", strings.TrimSpace(line))
	}
	return c.Conn.Write(p)
}

func failUpgrade(onErr js.Value, msg string) {
	if onErr.Truthy() {
		onErr.Invoke(msg)
	}
}

func startKubeletStream(id string, rawURL string, header http.Header, send js.Value, onErr js.Value, query string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		streamRegistry.Close(id)
		failUpgrade(onErr, err.Error())
		return
	}
	nodeName, kubeletPath, ok := parseNodePath(u.Path)
	if !ok {
		streamRegistry.Close(id)
		failUpgrade(onErr, "bad stream path")
		return
	}
	kubeletPath, embeddedQuery := splitEmbeddedQuery(kubeletPath)
	tlsConfig, err := kubeletCredentials()
	if err != nil {
		streamRegistry.Close(id)
		failUpgrade(onErr, err.Error())
		return
	}
	dial := server.Dialer(nodeName)
	d := websocket.Dialer{}
	if proto := header.Get("Sec-WebSocket-Protocol"); proto != "" {
		d.Subprotocols = []string{proto}
	}
	rawQuery := query
	if rawQuery == "" {
		rawQuery = u.RawQuery
	}
	if rawQuery == "" {
		rawQuery = header.Get("X-Stream-Query")
	}
	if rawQuery == "" {
		rawQuery = embeddedQuery
	}
	rawQuery = kubeletStreamQuery(rawQuery)
	println("kubelet stream", kubeletPath, "q=", rawQuery)
	kubeURL := &url.URL{Scheme: "wss", Host: "127.0.0.1:10250", Path: kubeletPath, RawQuery: rawQuery}
	reqHeader := http.Header{}
	d.NetDialTLSContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		raw, err := dial(ctx, network, "127.0.0.1:10250")
		if err != nil {
			return nil, err
		}
		tconn := tls.Client(raw, tlsConfig)
		if err := tconn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		return &firstLineConn{Conn: tconn}, nil
	}
	conn, resp, err := d.Dial(kubeURL.String(), reqHeader)
	if err != nil {
		streamRegistry.Close(id)
		status := ""
		if resp != nil {
			status = resp.Status
		}
		println("kubelet dial fail", status, err.Error())
		failUpgrade(onErr, status+" "+err.Error())
		return
	}
	println("kubelet dial ok")
	queued, ok := streamRegistry.Attach(id, conn)
	if !ok {
		_ = conn.Close()
		return
	}
	for _, data := range queued {
		_ = conn.WriteMessage(websocket.BinaryMessage, data)
	}
	defer func() {
		streamRegistry.Detach(id, conn)
		_ = conn.Close()
	}()
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				println("kubelet read done")
				if onErr.Truthy() {
					onErr.Invoke("")
				}
				return
			}
			println("kubelet read fail", err.Error())
			failUpgrade(onErr, err.Error())
			return
		}
		println("kubelet read", mt, len(data))
		arr := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(arr, data)
		send.Invoke(arr)
	}
}
func handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/node/", kubeletProxy())
	mux.Handle("/dial/", dialProxy())
	return mux
}

func kubeletProxy() http.Handler {
	target := &url.URL{Scheme: "https", Host: "127.0.0.1:10250"}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = pr.In.URL.Path
			pr.Out.URL.RawPath = ""
		},
		FlushInterval: -1,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		println("kubelet proxy", r.URL.Path, "q=", r.URL.RawQuery)
		nodeName, kubeletPath, ok := parseNodePath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		path, embeddedQuery := splitEmbeddedQuery(kubeletPath)
		r.URL.Path = path
		if q := r.Header.Get("X-Stream-Query"); q != "" {
			r.URL.RawQuery = q
		} else if embeddedQuery != "" {
			r.URL.RawQuery = embeddedQuery
		}
		r.URL.RawQuery = kubeletStreamQuery(r.URL.RawQuery)
		tlsConfig, err := kubeletCredentials()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
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

func dialProxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		node, host, port, path, ok := parseDialPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		dial := server.Dialer(node)
		conn, err := dial(r.Context(), "tcp", net.JoinHostPort(host, port))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer conn.Close()
		if r.Header.Get("X-Dial-TLS") == "1" {
			cfg, err := dialTLSConfig(r.Header.Get("X-Dial-ServerName"), r.Header.Get("X-Dial-CA"), bridge.Getenv("PROXY_CLIENT_CERT"), bridge.Getenv("PROXY_CLIENT_KEY"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			tconn := tls.Client(conn, cfg)
			if err := tconn.HandshakeContext(r.Context()); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			conn = tconn
		}
		outReq := dialRequest(r, host, port, path)
		if err := outReq.Write(conn); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), outReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})
}

func dialRequest(r *http.Request, host, port, path string) *http.Request {
	outReq := r.Clone(r.Context())
	outReq.URL.Scheme = "http"
	outReq.URL.Host = net.JoinHostPort(host, port)
	outReq.URL.Path = path
	outReq.RequestURI = ""
	outReq.Header.Del("X-Dial-TLS")
	outReq.Header.Del("X-Dial-ServerName")
	outReq.Header.Del("X-Dial-CA")
	if groups := outReq.Header.Values("X-Remote-Group"); len(groups) > 0 {
		outReq.Header["X-Remote-Group"] = splitJoinedHeaderValues(groups)
	}
	return outReq
}

func splitJoinedHeaderValues(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
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

func parseDialPath(p string) (node, host, port, path string, ok bool) {
	rest, ok := strings.CutPrefix(p, "/dial/")
	if !ok {
		return "", "", "", "", false
	}
	node, rest, ok = strings.Cut(rest, "/")
	if !ok || node == "" {
		return "", "", "", "", false
	}
	host, rest, ok = strings.Cut(rest, "/")
	if !ok || host == "" {
		return "", "", "", "", false
	}
	port, path, found := strings.Cut(rest, "/")
	if !found {
		return node, host, rest, "/", rest != ""
	}
	if port == "" {
		return "", "", "", "", false
	}
	return node, host, port, "/" + path, true
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

func kubeletStreamQuery(raw string) string {
	if raw == "" {
		return raw
	}
	v, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	for _, pair := range [][2]string{{"stdin", "input"}, {"stdout", "output"}, {"stderr", "error"}, {"tty", "tty"}} {
		from, to := pair[0], pair[1]
		switch v.Get(from) {
		case "true", "True", "TRUE", "1":
			v.Set(to, "1")
		}
		if from != to {
			v.Del(from)
		}
	}
	if ports := v["ports"]; len(ports) > 0 {
		for _, p := range ports {
			v.Add("port", p)
		}
		v.Del("ports")
	}
	return v.Encode()
}

func splitEmbeddedQuery(p string) (path, query string) {
	path, q, found := strings.Cut(p, "/_q/")
	if !found {
		return p, ""
	}
	if dec, err := url.QueryUnescape(q); err == nil {
		return path, dec
	}
	return path, q
}

func kubeletCredentials() (*tls.Config, error) {
	return kubeletTLSConfig(bridge.Getenv("KUBELET_CLIENT_CERT"), bridge.Getenv("KUBELET_CLIENT_KEY"), bridge.Getenv("KUBELET_CA"))
}

func kubeletTLSConfig(certPEM, keyPEM, caPEM string) (*tls.Config, error) {
	if certPEM == "" || keyPEM == "" {
		return nil, errors.New("kubelet client certificate is not configured")
	}
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, errors.New("kubelet serving CA is not configured")
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, RootCAs: pool, ServerName: "127.0.0.1", NextProtos: []string{"http/1.1"}}, nil
}
