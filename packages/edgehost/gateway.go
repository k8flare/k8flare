package edgehost

import (
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

const gatewayController = "k8flare.com/edge"

var gatewayAPIHosts = map[string]bool{
	"k8flare.com":     true,
	"api.k8flare.com": true,
}

type pathMatch struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	re    *regexp.Regexp
}

func AddAPIHosts(list string) {
	for _, host := range strings.Split(list, ",") {
		if h := hostname(strings.TrimSpace(host)); h != "" {
			gatewayAPIHosts[h] = true
		}
	}
}

func IsGatewayHost(host string) bool {
	h := hostname(host)
	if h == "" || h == "localhost" || net.ParseIP(h) != nil || strings.HasSuffix(h, ".workers.dev") || strings.HasSuffix(h, ".internal") || gatewayAPIHosts[h] {
		return false
	}
	_, ok := ParseServiceHost(h)
	return !ok
}

func MatchPath(pathname string, match *pathMatch) bool {
	value := "/"
	kind := "PathPrefix"
	if match != nil {
		if match.Value != "" {
			value = match.Value
		}
		if match.Type != "" {
			kind = match.Type
		}
	}
	switch kind {
	case "Exact":
		return pathname == value
	case "RegularExpression":
		re := match.re
		if re == nil {
			var err error
			if re, err = regexp.Compile(value); err != nil {
				return false
			}
		}
		return re.MatchString(pathname)
	default:
		if pathname == value {
			return true
		}
		prefix := value
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		if strings.HasPrefix(pathname, prefix) {
			return true
		}
		return value == "/" && strings.HasPrefix(pathname, "/")
	}
}

func MatchHostname(host string, hostnames []string) bool {
	if len(hostnames) == 0 {
		return true
	}
	h := strings.ToLower(strings.Split(host, ":")[0])
	for _, want := range hostnames {
		w := strings.ToLower(want)
		if strings.HasPrefix(w, "*.") {
			if strings.HasSuffix(h, w[1:]) && strings.Count(h, ".") == strings.Count(w, ".") {
				return true
			}
			continue
		}
		if h == w {
			return true
		}
	}
	return false
}

func proxyGateway(w http.ResponseWriter, r *http.Request, store *kine.Client, tunnel *http.Client) bool {
	handler := gatewayHandler(r, store, tunnel)
	if handler == nil {
		return false
	}
	handler.ServeHTTP(w, r)
	return true
}

func gatewayHandler(r *http.Request, store *kine.Client, tunnel *http.Client) http.Handler {
	host := RequestHost(r)
	if !IsGatewayHost(host) {
		return nil
	}
	table := edge.loadTable(r.Context(), store)
	if table == nil {
		return nil
	}
	rule := table.Lookup(host, r.Method, r.URL.Path, r.Header, r.URL.Query())
	if rule == nil {
		return nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rule.Invalid != "" {
			http.Error(w, rule.Invalid, http.StatusInternalServerError)
			return
		}
		out := applyFilters(rule, r)
		if out.Redirect != "" {
			w.Header().Set("Location", out.Redirect)
			w.WriteHeader(out.Status)
			return
		}
		total := totalWeight(rule.Backends)
		if total == 0 {
			http.Error(w, "no valid backend", http.StatusInternalServerError)
			return
		}
		backend := pickBackend(rule.Backends, rand.Intn(total))
		ref := Ref{Namespace: backend.Namespace, Name: backend.Name}
		svc, ok := edge.service(r, store, ref)
		if !ok {
			http.Error(w, "service not found", http.StatusInternalServerError)
			return
		}
		sel := portSel{Number: backend.Port, Name: backend.PortName}
		dialService(w, r, store, tunnel, dialSpec{
			Ref: ref, Ports: svc.Spec.Ports, Sel: sel,
			Path: out.Path + queryOf(r.URL), Host: out.Host, ForwardedHost: host,
			ResponseHeader: out.ResponseHeader,
		})
	})
}
