package authz

import (
	"net/http"
	"strings"
)

type rewriteHost struct {
	base string
	next http.RoundTripper
}

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	u.Scheme = "http"
	u.Host = strings.TrimPrefix(strings.TrimPrefix(r.base, "https://"), "http://")
	next := req.Clone(req.Context())
	next.URL = &u
	next.Host = u.Host
	return r.next.RoundTrip(next)
}
