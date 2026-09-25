package edgehost

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

var hopByHop = map[string]bool{
	"connection": true, "keep-alive": true, "proxy-authenticate": true, "proxy-authorization": true,
	"te": true, "trailers": true, "transfer-encoding": true, "upgrade": true,
}

func Proxy(w http.ResponseWriter, r *http.Request, store *kine.Client, tunnel *http.Client) bool {
	if store == nil || tunnel == nil {
		return false
	}
	if ref, ok := ParseServiceRoute(r); ok {
		return proxyLoadBalancer(w, r, store, tunnel, ref)
	}
	return proxyGateway(w, r, store, tunnel)
}

func proxyLoadBalancer(w http.ResponseWriter, r *http.Request, store *kine.Client, tunnel *http.Client, ref Ref) bool {
	svc, ok := getService(r, store, ref)
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return true
	}
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer || !OwnsClass(deref(svc.Spec.LoadBalancerClass)) {
		http.Error(w, "not a load balancer", http.StatusNotFound)
		return true
	}
	return dialService(w, r, store, tunnel, ref, svc.Spec.Ports, ServicePath(r, ref))
}

func dialService(w http.ResponseWriter, r *http.Request, store *kine.Client, tunnel *http.Client, ref Ref, ports []corev1.ServicePort, path string) bool {
	node, host, port := resolveDial(r, store, ref, ports)
	if node == "" || host == "" || port == "" {
		http.Error(w, "no ready endpoints", http.StatusServiceUnavailable)
		return true
	}
	target := "https://nodetunnel.internal/dial/" + node + "/" + host + "/" + port + path
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return true
	}
	for k, vs := range r.Header {
		lk := strings.ToLower(k)
		if hopByHop[lk] || strings.HasPrefix(lk, "cf-") {
			continue
		}
		for _, v := range vs {
			out.Header.Add(k, v)
		}
	}
	out.Host = IngressHostname(ref.Namespace, ref.Name)
	out.Header.Set("Host", out.Host)
	out.Header.Set("X-Forwarded-Proto", "https")
	if client := r.Header.Get("CF-Connecting-IP"); client != "" {
		out.Header.Set("X-Forwarded-For", client)
	}
	resp, err := tunnel.Do(out)
	if err != nil {
		http.Error(w, "tunnel unavailable", http.StatusBadGateway)
		return true
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return true
}

func getService(r *http.Request, store *kine.Client, ref Ref) (corev1.Service, bool) {
	var svc corev1.Service
	if !loadJSON(r, store, "/registry/services/"+ref.Namespace+"/"+ref.Name, &svc) {
		return corev1.Service{}, false
	}
	return svc, true
}

func resolveDial(r *http.Request, store *kine.Client, ref Ref, ports []corev1.ServicePort) (node, host, port string) {
	want := pickPort(ports, r.Header.Get("X-K8flare-Port"))
	var ep corev1.Endpoints
	if loadJSON(r, store, "/registry/endpoints/"+ref.Namespace+"/"+ref.Name, &ep) {
		for _, subset := range ep.Subsets {
			p := subsetPort(subset.Ports, want)
			for _, addr := range subset.Addresses {
				n := nodeOf(r, store, deref(addr.NodeName), addr.TargetRef)
				if addr.IP != "" && n != "" && p != "" {
					return n, addr.IP, p
				}
			}
		}
	}
	kvs, _, _, err := store.List(r.Context(), "/registry/endpointslices/"+ref.Namespace+"/", "", 500)
	if err != nil {
		return "", "", ""
	}
	for _, kv := range kvs {
		var slice discoveryv1.EndpointSlice
		if !decodeKV(kv.Value, &slice) {
			continue
		}
		if slice.Labels["kubernetes.io/service-name"] != ref.Name {
			continue
		}
		p := slicePort(slice.Ports, want)
		for _, endpoint := range slice.Endpoints {
			if len(endpoint.Addresses) == 0 {
				continue
			}
			nodeName := ""
			if endpoint.NodeName != nil {
				nodeName = *endpoint.NodeName
			}
			n := nodeOf(r, store, nodeName, endpoint.TargetRef)
			if n != "" && p != "" {
				return n, endpoint.Addresses[0], p
			}
		}
	}
	return "", "", ""
}

func nodeOf(r *http.Request, store *kine.Client, nodeName string, ref *corev1.ObjectReference) string {
	if nodeName != "" {
		return nodeName
	}
	if ref == nil || ref.Kind != "Pod" || ref.Namespace == "" || ref.Name == "" {
		return ""
	}
	var pod corev1.Pod
	if !loadJSON(r, store, "/registry/pods/"+ref.Namespace+"/"+ref.Name, &pod) {
		return ""
	}
	return pod.Spec.NodeName
}

func pickPort(ports []corev1.ServicePort, named string) int32 {
	if n, err := strconv.Atoi(named); err == nil && n > 0 {
		return int32(n)
	}
	for _, p := range ports {
		if p.Name == "http" || p.Port == 80 {
			return p.Port
		}
	}
	for _, p := range ports {
		if p.Name == "https" || p.Port == 443 {
			return p.Port
		}
	}
	if len(ports) > 0 {
		return ports[0].Port
	}
	return 0
}

func subsetPort(ports []corev1.EndpointPort, want int32) string {
	if want != 0 {
		for _, p := range ports {
			if p.Port == want {
				return strconv.Itoa(int(p.Port))
			}
		}
	}
	for _, p := range ports {
		if p.Port != 0 {
			return strconv.Itoa(int(p.Port))
		}
	}
	if want != 0 {
		return strconv.Itoa(int(want))
	}
	return ""
}

func slicePort(ports []discoveryv1.EndpointPort, want int32) string {
	if want != 0 {
		for _, p := range ports {
			if p.Port != nil && *p.Port == want {
				return strconv.Itoa(int(*p.Port))
			}
		}
	}
	for _, p := range ports {
		if p.Port != nil && *p.Port != 0 {
			return strconv.Itoa(int(*p.Port))
		}
	}
	if want != 0 {
		return strconv.Itoa(int(want))
	}
	return ""
}

func loadJSON(r *http.Request, store *kine.Client, key string, dest any) bool {
	kv, _, err := store.Get(r.Context(), key)
	if err != nil || kv == nil {
		return false
	}
	return decodeKV(kv.Value, dest)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func decodeKV(value string, dest any) bool {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		raw = []byte(value)
	}
	return json.Unmarshal(raw, dest) == nil
}
