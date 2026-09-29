package workloads

import (
	"context"
	"net/http"
	"sync/atomic"
)

type syncHolder struct {
	ctx context.Context
}

var currentSync atomic.Pointer[syncHolder]

func bindSync(ctx context.Context) func() {
	currentSync.Store(&syncHolder{ctx: ctx})
	return func() { currentSync.Store(nil) }
}

type SyncTransport struct {
	Base http.RoundTripper
}

func (t SyncTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if held := currentSync.Load(); held != nil && held.ctx != nil {
		req = req.WithContext(held.ctx)
	}
	return t.Base.RoundTrip(req)
}
