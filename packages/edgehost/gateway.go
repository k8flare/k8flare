package edgehost

import (
	"net/http"
	"regexp"
	"strconv"
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
}

type httpRoute struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		ParentRefs []struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"parentRefs"`
		Hostnames []string `json:"hostnames"`
		Rules     []struct {
			Matches []struct {
				Path *pathMatch `json:"path"`
			} `json:"matches"`
			BackendRefs []struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				Port      *int32 `json:"port"`
			} `json:"backendRefs"`
		} `json:"rules"`
	} `json:"spec"`
}

func IsGatewayHost(host string) bool {
	h := strings.ToLower(strings.Split(host, ":")[0])
	if h == "" || strings.HasSuffix(h, ".workers.dev") || strings.HasSuffix(h, ".internal") || gatewayAPIHosts[h] {
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
		re, err := regexp.Compile(value)
		if err != nil {
			return false
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
	host := RequestHost(r)
	if !IsGatewayHost(host) {
		return false
	}
	kvs, _, _, err := store.List(r.Context(), "/registry/gateway.networking.k8s.io/httproutes/", "", 500)
	if err != nil {
		http.Error(w, "gateway unavailable", http.StatusBadGateway)
		return true
	}
	for _, kv := range kvs {
		var route httpRoute
		if !decodeKV(kv.Value, &route) || !ownsRoute(r, store, route) || !MatchHostname(host, route.Spec.Hostnames) {
			continue
		}
		ns := route.Metadata.Namespace
		if ns == "" {
			ns = "default"
		}
		for _, rule := range route.Spec.Rules {
			matches := rule.Matches
			if len(matches) == 0 {
				matches = []struct {
					Path *pathMatch `json:"path"`
				}{{Path: &pathMatch{Type: "PathPrefix", Value: "/"}}}
			}
			hit := false
			for _, m := range matches {
				if MatchPath(r.URL.Path, m.Path) {
					hit = true
					break
				}
			}
			if !hit || len(rule.BackendRefs) == 0 || rule.BackendRefs[0].Name == "" {
				continue
			}
			backend := rule.BackendRefs[0]
			ref := Ref{Namespace: backend.Namespace, Name: backend.Name}
			if ref.Namespace == "" {
				ref.Namespace = ns
			}
			if backend.Port != nil && *backend.Port > 0 {
				r.Header.Set("X-K8flare-Port", strconv.Itoa(int(*backend.Port)))
			}
			svc, ok := getService(r, store, ref)
			if !ok {
				http.Error(w, "service not found", http.StatusNotFound)
				return true
			}
			path := r.URL.Path
			if r.URL.RawQuery != "" {
				path += "?" + r.URL.RawQuery
			}
			return dialService(w, r, store, tunnel, ref, svc.Spec.Ports, path)
		}
	}
	return false
}

func ownsRoute(r *http.Request, store *kine.Client, route httpRoute) bool {
	nsDefault := route.Metadata.Namespace
	if nsDefault == "" {
		nsDefault = "default"
	}
	for _, parent := range route.Spec.ParentRefs {
		if parent.Name == "" {
			continue
		}
		ns := parent.Namespace
		if ns == "" {
			ns = nsDefault
		}
		var gw struct {
			Spec struct {
				GatewayClassName string `json:"gatewayClassName"`
			} `json:"spec"`
		}
		if !loadJSON(r, store, "/registry/gateway.networking.k8s.io/gateways/"+ns+"/"+parent.Name, &gw) || gw.Spec.GatewayClassName == "" {
			continue
		}
		var cls struct {
			Spec struct {
				ControllerName string `json:"controllerName"`
			} `json:"spec"`
		}
		if loadJSON(r, store, "/registry/gateway.networking.k8s.io/gatewayclasses/"+gw.Spec.GatewayClassName, &cls) && cls.Spec.ControllerName == gatewayController {
			return true
		}
	}
	return false
}
