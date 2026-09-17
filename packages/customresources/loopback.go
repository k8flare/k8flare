package customresources

import (
	"bytes"
	"io"
	"net/http"

	"k8s.io/apiserver/pkg/authentication/user"
)

type loopback struct {
	handler http.Handler
}

func (l loopback) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Remote-User", user.APIServerUser)
	req.Header.Set("X-Remote-Group", user.SystemPrivilegedGroup)
	w := &bufferResponse{header: http.Header{}, status: http.StatusOK}
	l.handler.ServeHTTP(w, req)
	return &http.Response{
		StatusCode:    w.status,
		Status:        http.StatusText(w.status),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        w.header,
		Body:          io.NopCloser(bytes.NewReader(w.body.Bytes())),
		ContentLength: int64(w.body.Len()),
		Request:       req,
	}, nil
}

type bufferResponse struct {
	header  http.Header
	body    bytes.Buffer
	status  int
	written bool
}

func (b *bufferResponse) Header() http.Header { return b.header }

func (b *bufferResponse) WriteHeader(code int) {
	if !b.written {
		b.status = code
		b.written = true
	}
}

func (b *bufferResponse) Write(p []byte) (int, error) {
	b.WriteHeader(http.StatusOK)
	return b.body.Write(p)
}

func (b *bufferResponse) Flush() { b.WriteHeader(http.StatusOK) }
