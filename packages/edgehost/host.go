package edgehost

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	zoneSuffix   = ".k8flare.com"
	legacySuffix = ".svc.k8flare.com"
	LBClass      = "k8flare.com/edge"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type Ref struct {
	Namespace string
	Name      string
}

func IngressHostname(namespace, name string) string {
	return name + "--" + namespace + zoneSuffix
}

func OwnsClass(cls string) bool {
	return cls == "" || cls == LBClass
}

func ParseServiceHost(host string) (Ref, bool) {
	h := strings.ToLower(strings.Split(host, ":")[0])
	if strings.HasSuffix(h, legacySuffix) {
		labels := strings.Split(strings.TrimSuffix(h, legacySuffix), ".")
		if len(labels) == 2 && dnsLabel.MatchString(labels[0]) && dnsLabel.MatchString(labels[1]) {
			return Ref{Name: labels[0], Namespace: labels[1]}, true
		}
		if len(labels) == 1 {
			return dashRef(labels[0])
		}
		return Ref{}, false
	}
	if strings.HasSuffix(h, zoneSuffix) && !strings.Contains(strings.TrimSuffix(h, zoneSuffix), ".") {
		return dashRef(strings.TrimSuffix(h, zoneSuffix))
	}
	return Ref{}, false
}

func dashRef(label string) (Ref, bool) {
	i := strings.LastIndex(label, "--")
	if i <= 0 {
		return Ref{}, false
	}
	name, namespace := label[:i], label[i+2:]
	if dnsLabel.MatchString(name) && dnsLabel.MatchString(namespace) {
		return Ref{Name: name, Namespace: namespace}, true
	}
	return Ref{}, false
}

func RequestHost(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host == "" && r.URL != nil {
		host = r.URL.Hostname()
	}
	return hostname(host)
}

func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

func ParseServiceRoute(r *http.Request) (Ref, bool) {
	if ref, ok := ParseServiceHost(RequestHost(r)); ok {
		return ref, true
	}
	parts := strings.Split(r.URL.Path, "/")
	if !IsGatewayHost(RequestHost(r)) && len(parts) >= 4 && parts[1] == "svc" && dnsLabel.MatchString(parts[2]) && dnsLabel.MatchString(parts[3]) {
		return Ref{Namespace: parts[2], Name: parts[3]}, true
	}
	return Ref{}, false
}

func ServicePath(r *http.Request, ref Ref) string {
	u := r.URL
	if _, ok := ParseServiceHost(RequestHost(r)); ok {
		return u.Path + queryOf(u)
	}
	prefix := "/svc/" + ref.Namespace + "/" + ref.Name
	path := u.Path
	if path == prefix || strings.HasPrefix(path, prefix+"/") {
		path = strings.TrimPrefix(path, prefix)
	}
	if path == "" {
		path = "/"
	}
	return path + queryOf(u)
}

func queryOf(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}
	return "?" + u.RawQuery
}
