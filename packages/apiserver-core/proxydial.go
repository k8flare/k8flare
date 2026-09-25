package core

import (
	"net/http"
	"net/url"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/registry/rest"
)

type proxyDial struct {
	node, host, port, path, scheme string
}

func newProxyHandler(base string, dial proxyDial, rt http.RoundTripper, responder rest.Responder) (http.Handler, error) {
	loc, err := proxyUserURL(base, dial.path)
	if err != nil {
		return nil, err
	}
	inner := tunnelDialTransport{
		base:   rt,
		root:   base,
		node:   dial.node,
		host:   dial.host,
		port:   dial.port,
		secure: dial.scheme == "https",
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if shouldRedirectProxyRoot(dial.path, req) {
			target := strings.TrimRight(req.URL.Path, "/") + "/"
			if req.URL.RawQuery != "" {
				target += "?" + req.URL.RawQuery
			}
			w.Header().Set("Location", target)
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		suffix := loc.Path
		if strings.HasSuffix(req.URL.Path, "/") && !strings.HasSuffix(suffix, "/") {
			suffix += "/"
		}
		h := proxy.NewUpgradeAwareHandler(loc, &proxy.Transport{
			PathPrepend:  strings.TrimSuffix(req.URL.Path, suffix),
			RoundTripper: inner,
		}, false, false, proxy.NewErrorResponder(responder))
		h.ServeHTTP(w, req)
	}), nil
}

func proxyRootRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if (req.Method == http.MethodGet || req.Method == http.MethodHead) && isBareProxyPath(req.URL.Path) {
			target := req.URL.Path + "/"
			if req.URL.RawQuery != "" {
				target += "?" + req.URL.RawQuery
			}
			w.Header().Set("Location", target)
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func isBareProxyPath(p string) bool {
	return strings.HasSuffix(p, "/proxy") && !strings.HasSuffix(p, "/proxy/")
}

func shouldRedirectProxyRoot(userPath string, req *http.Request) bool {
	if userPath != "" && userPath != "/" {
		return false
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	path := req.URL.Path
	if req.RequestURI != "" {
		raw := req.RequestURI
		if i := strings.Index(raw, "?"); i >= 0 {
			raw = raw[:i]
		}
		if raw != "" {
			path = raw
		}
	}
	return !strings.HasSuffix(path, "/")
}

func proxyUserURL(base, path string) (*url.URL, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, apierrors.NewServiceUnavailable(err.Error())
	}
	if path != "" {
		u.Path = "/" + strings.TrimPrefix(path, "/")
	} else {
		u.Path = ""
	}
	return u, nil
}

type tunnelDialTransport struct {
	base   http.RoundTripper
	root   string
	node   string
	host   string
	port   string
	secure bool
}

func (t tunnelDialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u, err := url.Parse(t.root)
	if err != nil {
		return nil, err
	}
	u.Path = "/dial/" + t.node + "/" + t.host + "/" + t.port + req.URL.Path
	u.RawQuery = req.URL.RawQuery
	clone.URL = u
	clone.Host = u.Host
	if t.secure {
		clone.Header.Set("X-Dial-TLS", "1")
	}
	return t.base.RoundTrip(clone)
}
