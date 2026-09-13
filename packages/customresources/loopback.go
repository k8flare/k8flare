package customresources

import (
	"io"
	"net/http"
	"sync"

	"k8s.io/apiserver/pkg/authentication/user"
)

type loopback struct {
	handler http.Handler
}

func (l loopback) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Remote-User", user.APIServerUser)
	req.Header.Set("X-Remote-Group", user.SystemPrivilegedGroup)
	pr, pw := io.Pipe()
	w := &pipeResponse{header: http.Header{}, body: pw, started: make(chan struct{}), closed: make(chan bool, 1)}
	go func() {
		l.handler.ServeHTTP(w, req)
		w.start(http.StatusOK)
		pw.Close()
	}()
	go func() {
		<-req.Context().Done()
		w.closed <- true
		pr.CloseWithError(req.Context().Err())
	}()
	<-w.started
	return &http.Response{
		StatusCode:    w.status,
		Status:        http.StatusText(w.status),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        w.header,
		Body:          pr,
		ContentLength: -1,
		Request:       req,
	}, nil
}

type pipeResponse struct {
	header  http.Header
	body    *io.PipeWriter
	status  int
	once    sync.Once
	started chan struct{}
	closed  chan bool
}

func (p *pipeResponse) Header() http.Header { return p.header }

func (p *pipeResponse) start(code int) {
	p.once.Do(func() {
		p.status = code
		close(p.started)
	})
}

func (p *pipeResponse) WriteHeader(code int) { p.start(code) }

func (p *pipeResponse) Write(b []byte) (int, error) {
	p.start(http.StatusOK)
	return p.body.Write(b)
}

func (p *pipeResponse) Flush() { p.start(http.StatusOK) }

func (p *pipeResponse) CloseNotify() <-chan bool { return p.closed }
