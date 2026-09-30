package edgehost

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	networkingv1beta1 "k8s.io/api/networking/v1beta1"
)

const (
	gatewayGroup   = "gateway.networking.k8s.io"
	ingressKindTag = "Ingress"
	routeKindTag   = "HTTPRoute"
)

type ingPort struct {
	Name   string `json:"name"`
	Number int32  `json:"number"`
}

type ingService struct {
	Name string  `json:"name"`
	Port ingPort `json:"port"`
}

type ingBackend struct {
	Service  *ingService     `json:"service"`
	Resource json.RawMessage `json:"resource"`
}

type ingPath struct {
	Path     string     `json:"path"`
	PathType string     `json:"pathType"`
	Backend  ingBackend `json:"backend"`
}

type ingHTTP struct {
	Paths []ingPath `json:"paths"`
}

type ingRule struct {
	Host string   `json:"host"`
	HTTP *ingHTTP `json:"http"`
}

type ingSpec struct {
	IngressClassName *string     `json:"ingressClassName"`
	DefaultBackend   *ingBackend `json:"defaultBackend"`
	Rules            []ingRule   `json:"rules"`
}

type ingObject struct {
	Key    string
	Rev    int64
	Obj    map[string]json.RawMessage
	Meta   gwMeta
	Spec   ingSpec
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	}
}

type ingClass struct {
	Meta gwMeta
	Spec struct {
		Controller string `json:"controller"`
	}
}

type svcPort struct {
	Name string `json:"name"`
	Port int32  `json:"port"`
}

type gwGrant struct {
	Meta gwMeta
	Spec struct {
		From []struct {
			Group     string `json:"group"`
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
		} `json:"from"`
		To []struct {
			Group string `json:"group"`
			Kind  string `json:"kind"`
			Name  string `json:"name"`
		} `json:"to"`
	}
}

func (m gwMeta) namespace() string {
	if m.Namespace == "" {
		return "default"
	}
	return m.Namespace
}

func ingressClassOf(ing ingObject) string {
	if ing.Spec.IngressClassName != nil {
		return *ing.Spec.IngressClassName
	}
	return ing.Meta.Annotations[networkingv1beta1.AnnotationIngressClass]
}

func ownsIngress(ing ingObject, classes []ingClass) bool {
	name := ingressClassOf(ing)
	for _, cls := range classes {
		if cls.Meta.Name == name && name != "" && cls.Spec.Controller == gatewayController {
			return true
		}
	}
	return false
}

func ingressBackend(ns string, b ingBackend) ([]Backend, string) {
	if b.Service == nil {
		return nil, "resource backends are not supported"
	}
	return []Backend{{Namespace: ns, Name: b.Service.Name, Port: b.Service.Port.Number, PortName: b.Service.Port.Name, Weight: 1}}, ""
}

func compileIngress(ing ingObject) []Rule {
	ns := ing.Meta.namespace()
	source := ingressKindTag + "/" + ns + "/" + ing.Meta.Name
	var rules []Rule
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			out := Rule{Source: source, Created: ing.Meta.CreationTimestamp, Host: strings.ToLower(rule.Host)}
			out.Path = &pathMatch{Type: "PathPrefix", Value: strings.TrimSuffix(p.Path, "/")}
			if p.PathType == "Exact" {
				out.Path = &pathMatch{Type: "Exact", Value: p.Path}
			} else if out.Path.Value == "" {
				out.Path.Value = "/"
			}
			out.Backends, out.Invalid = ingressBackend(ns, p.Backend)
			rules = append(rules, out)
		}
	}
	if ing.Spec.DefaultBackend != nil {
		out := Rule{Source: source, Created: ing.Meta.CreationTimestamp, Fallback: true, Path: &pathMatch{Type: "PathPrefix", Value: "/"}}
		out.Backends, out.Invalid = ingressBackend(ns, *ing.Spec.DefaultBackend)
		rules = append(rules, out)
	}
	return rules
}

func ingressHostnames(ing ingObject) []string {
	seen := map[string]bool{}
	var hosts []string
	for _, rule := range ing.Spec.Rules {
		h := strings.ToLower(rule.Host)
		if h == "" || strings.HasPrefix(h, "*.") || seen[h] {
			continue
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	return hosts
}

type gwListener struct {
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	Protocol      string `json:"protocol"`
	AllowedRoutes struct {
		Namespaces struct {
			From string `json:"from"`
		} `json:"namespaces"`
		Kinds []struct {
			Group string `json:"group"`
			Kind  string `json:"kind"`
		} `json:"kinds"`
	} `json:"allowedRoutes"`
}

type gwParentRef struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Group       string `json:"group"`
	Kind        string `json:"kind"`
	SectionName string `json:"sectionName"`
}

type gwBackendRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	Port      *int32 `json:"port"`
	Weight    *int32 `json:"weight"`
}

type gwMatch struct {
	Path        *pathMatch   `json:"path"`
	Headers     []ValueMatch `json:"headers"`
	QueryParams []ValueMatch `json:"queryParams"`
	Method      string       `json:"method"`
}

type gwRouteRule struct {
	Matches     []gwMatch      `json:"matches"`
	Filters     []Filter       `json:"filters"`
	BackendRefs []gwBackendRef `json:"backendRefs"`
}

type attachment struct {
	Listeners []string
	Hosts     []string
	Accepted  bool
	Reason    string
	Message   string
}

func hostsIntersect(routeHost, listenerHost string) (string, bool) {
	if listenerHost == "" {
		return routeHost, true
	}
	if MatchHostname(routeHost, []string{listenerHost}) {
		return routeHost, true
	}
	if MatchHostname(listenerHost, []string{routeHost}) {
		return listenerHost, true
	}
	return "", false
}

func listenerAllows(l gwListener, gw gwObject, route gwRoute) bool {
	if l.Protocol != "HTTP" && l.Protocol != "HTTPS" {
		return false
	}
	if len(l.AllowedRoutes.Kinds) > 0 {
		found := false
		for _, k := range l.AllowedRoutes.Kinds {
			if k.Kind == routeKindTag && (k.Group == "" || k.Group == gatewayGroup) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	switch l.AllowedRoutes.Namespaces.From {
	case "All":
		return true
	case "Selector":
		return false
	}
	return route.Meta.namespace() == gw.Meta.namespace()
}

func attach(route gwRoute, ref gwParentRef, gw gwObject) attachment {
	out := attachment{Reason: "NoMatchingParent", Message: "no listener matches the parentRef"}
	seen := map[string]bool{}
	for _, l := range gw.Spec.Listeners {
		if ref.SectionName != "" && ref.SectionName != l.Name {
			continue
		}
		out.Reason, out.Message = "NotAllowedByListeners", "no listener allows this route"
		if !listenerAllows(l, gw, route) {
			continue
		}
		out.Reason, out.Message = "NoMatchingListenerHostname", "no listener hostname matches the route hostnames"
		if len(route.Spec.Hostnames) == 0 {
			out.Listeners = append(out.Listeners, l.Name)
			if l.Hostname != "" && !seen[l.Hostname] {
				seen[l.Hostname] = true
				out.Hosts = append(out.Hosts, strings.ToLower(l.Hostname))
			}
			continue
		}
		matched := false
		for _, h := range route.Spec.Hostnames {
			if host, ok := hostsIntersect(strings.ToLower(h), strings.ToLower(l.Hostname)); ok {
				matched = true
				if !seen[host] {
					seen[host] = true
					out.Hosts = append(out.Hosts, host)
				}
			}
		}
		if matched {
			out.Listeners = append(out.Listeners, l.Name)
		}
	}
	if len(out.Listeners) > 0 {
		out.Accepted, out.Reason, out.Message = true, "Accepted", "Route attached"
	}
	return out
}

func serviceKey(ns, name string) string { return ns + "/" + name }

func backendIssue(route gwRoute, ref gwBackendRef, grants []gwGrant, services map[string][]svcPort) (reason, message string) {
	if (ref.Group != "" && ref.Group != "core") || (ref.Kind != "" && ref.Kind != "Service") {
		return "InvalidKind", "only Service backends are supported"
	}
	ns := ref.Namespace
	if ns == "" {
		ns = route.Meta.namespace()
	}
	if ns != route.Meta.namespace() && !granted(grants, route.Meta.namespace(), ns, ref.Name) {
		return "RefNotPermitted", "no ReferenceGrant allows the reference to " + ns + "/" + ref.Name
	}
	ports, ok := services[serviceKey(ns, ref.Name)]
	if !ok {
		return "BackendNotFound", "service " + ns + "/" + ref.Name + " not found"
	}
	if ref.Port != nil {
		for _, p := range ports {
			if p.Port == *ref.Port {
				return "", ""
			}
		}
		return "BackendNotFound", "service " + ns + "/" + ref.Name + " has no port " + strconv.Itoa(int(*ref.Port))
	}
	return "", ""
}

func granted(grants []gwGrant, fromNS, toNS, name string) bool {
	for _, g := range grants {
		if g.Meta.namespace() != toNS {
			continue
		}
		fromOK := false
		for _, f := range g.Spec.From {
			if f.Group == gatewayGroup && f.Kind == routeKindTag && f.Namespace == fromNS {
				fromOK = true
			}
		}
		for _, to := range g.Spec.To {
			if fromOK && to.Group == "" && to.Kind == "Service" && (to.Name == "" || to.Name == name) {
				return true
			}
		}
	}
	return false
}

func resolvedRefs(route gwRoute, grants []gwGrant, services map[string][]svcPort) (reason, message string) {
	for _, rule := range route.Spec.Rules {
		if msg := unsupportedFilter(rule.Filters); msg != "" {
			return "UnsupportedValue", msg
		}
		for _, ref := range rule.BackendRefs {
			if reason, msg := backendIssue(route, ref, grants, services); reason != "" {
				return reason, msg
			}
		}
	}
	return "ResolvedRefs", "Backend refs resolved"
}

func compileRoute(route gwRoute, hosts []string, grants []gwGrant, services map[string][]svcPort) []Rule {
	source := routeKindTag + "/" + route.Meta.namespace() + "/" + route.Meta.Name
	if len(hosts) == 0 {
		hosts = []string{""}
	}
	var out []Rule
	for _, rule := range route.Spec.Rules {
		matches := rule.Matches
		if len(matches) == 0 {
			matches = []gwMatch{{}}
		}
		invalid := unsupportedFilter(rule.Filters)
		var backends []Backend
		for _, ref := range rule.BackendRefs {
			if reason, _ := backendIssue(route, ref, grants, services); reason != "" {
				continue
			}
			ns := ref.Namespace
			if ns == "" {
				ns = route.Meta.namespace()
			}
			weight := int32(1)
			if ref.Weight != nil {
				weight = *ref.Weight
			}
			b := Backend{Namespace: ns, Name: ref.Name, Weight: weight}
			if ref.Port != nil {
				b.Port = *ref.Port
			}
			backends = append(backends, b)
		}
		for _, host := range hosts {
			for _, m := range matches {
				path := &pathMatch{Type: "PathPrefix", Value: "/"}
				if m.Path != nil {
					copied := *m.Path
					if copied.Type == "" || copied.Type == "PathPrefix" {
						copied.Value = strings.TrimSuffix(copied.Value, "/")
						if copied.Value == "" {
							copied.Value = "/"
						}
					}
					path = &copied
				}
				out = append(out, Rule{
					Source: source, Created: route.Meta.CreationTimestamp, Host: host,
					Path: path, Method: m.Method, Headers: m.Headers, Query: m.QueryParams,
					Filters: rule.Filters, Backends: backends, Invalid: invalid,
				})
			}
		}
	}
	return out
}

type edgeState struct {
	Classes    []gwClass
	Gateways   []gwObject
	Routes     []gwRoute
	Grants     []gwGrant
	IngClasses []ingClass
	Ingresses  []ingObject
	Services   map[string][]svcPort
}

type statusUpdate struct {
	Key    string
	Rev    int64
	Obj    map[string]json.RawMessage
	Status any
}

type edgeResult struct {
	Table    Table
	Statuses []statusUpdate
}

func findClass(classes []gwClass, name string) *gwClass {
	for i := range classes {
		if classes[i].Meta.Name == name {
			return &classes[i]
		}
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
