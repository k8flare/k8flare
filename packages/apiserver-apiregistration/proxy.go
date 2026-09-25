package apiregistration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/proxy"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/transport"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	helper "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1/helper"
	"k8s.io/streaming/pkg/httpstream"
)

const tunnelDialBase = "https://nodetunnel.internal/dial/"

var (
	kineClient *kine.Client
	tunnel     http.RoundTripper
)

type proxyResponder struct{ w http.ResponseWriter }

func (r proxyResponder) Object(statusCode int, obj runtime.Object) {
	http.Error(r.w, fmt.Sprintf("%T", obj), statusCode)
}

func (r proxyResponder) Error(_ http.ResponseWriter, _ *http.Request, err error) {
	http.Error(r.w, err.Error(), http.StatusServiceUnavailable)
}

func proxyRemote() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/apis/apiregistration.k8s.io/") || r.URL.Path == "/apis/apiregistration.k8s.io" {
				next.ServeHTTP(w, r)
				return
			}
			group, version := splitGroupVersion(r.URL.Path)
			svc, ok := loadStoredAPIService(r.Context(), version+"."+group)
			if !ok || svc.Spec.Service == nil {
				http.NotFound(w, r)
				return
			}
			if err := serveAggregated(w, r, svc); err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
			}
		})
	}
}

func splitGroupVersion(path string) (group, version string) {
	rest := strings.TrimPrefix(path, "/apis/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) > 0 {
		group = parts[0]
	}
	if len(parts) > 1 {
		version = parts[1]
	}
	return group, version
}

func loadStoredAPIService(ctx context.Context, name string) (*apiregistrationv1.APIService, bool) {
	if kineClient == nil || name == "" {
		return nil, false
	}
	kv, _, err := kineClient.Get(ctx, "/registry/apiservices/"+name)
	if err != nil || kv == nil {
		return nil, false
	}
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, false
	}
	var svc apiregistrationv1.APIService
	if err := json.Unmarshal(data, &svc); err != nil {
		return nil, false
	}
	return &svc, true
}

func serveAggregated(w http.ResponseWriter, r *http.Request, svc *apiregistrationv1.APIService) error {
	if tunnel == nil {
		return fmt.Errorf("service unavailable")
	}
	ref := svc.Spec.Service
	port := int32(443)
	if ref.Port != nil && *ref.Port != 0 {
		port = *ref.Port
	}
	target, node, err := resolveService(r.Context(), ref.Namespace, ref.Name, port)
	if err != nil {
		return err
	}
	location, err := url.Parse(tunnelDialBase + node + "/" + target.host + "/" + target.port + r.URL.RequestURI())
	if err != nil {
		return err
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.URL = location
	out.Header.Set("X-Dial-TLS", "1")
	out.Header.Set("X-Dial-ServerName", ref.Name+"."+ref.Namespace+".svc")
	if len(svc.Spec.CABundle) > 0 {
		out.Header.Set("X-Dial-CA", base64.StdEncoding.EncodeToString(svc.Spec.CABundle))
	}
	if u, ok := genericapirequest.UserFrom(r.Context()); ok {
		transport.SetAuthProxyHeaders(out, u.GetName(), u.GetUID(), u.GetGroups(), u.GetExtra())
	}
	handler := proxy.NewUpgradeAwareHandler(location, tunnel, true, httpstream.IsUpgradeRequest(r), proxyResponder{w: w})
	handler.ServeHTTP(w, out)
	return nil
}

type dialTarget struct{ host, port string }

func resolveService(ctx context.Context, ns, name string, port int32) (dialTarget, string, error) {
	if ep, ok := getJSON[corev1.Endpoints](ctx, "/registry/endpoints/"+ns+"/"+name); ok {
		for _, subset := range ep.Subsets {
			for _, addr := range subset.Addresses {
				p := port
				for _, sp := range subset.Ports {
					if sp.Port != 0 {
						p = sp.Port
						break
					}
				}
				node := deref(addr.NodeName)
				if node == "" && addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
					if pod, found := getJSON[corev1.Pod](ctx, "/registry/pods/"+addr.TargetRef.Namespace+"/"+addr.TargetRef.Name); found {
						node = pod.Spec.NodeName
					}
				}
				if addr.IP != "" && node != "" {
					return dialTarget{host: addr.IP, port: strconv.Itoa(int(p))}, node, nil
				}
			}
		}
	}
	slices, err := listJSON[discoveryv1.EndpointSlice](ctx, "/registry/endpointslices/"+ns+"/")
	if err != nil {
		return dialTarget{}, "", err
	}
	for _, slice := range slices {
		if slice.Labels["kubernetes.io/service-name"] != name {
			continue
		}
		for _, endpoint := range slice.Endpoints {
			if len(endpoint.Addresses) == 0 {
				continue
			}
			node := deref(endpoint.NodeName)
			if node == "" && endpoint.TargetRef != nil && endpoint.TargetRef.Kind == "Pod" {
				if pod, found := getJSON[corev1.Pod](ctx, "/registry/pods/"+endpoint.TargetRef.Namespace+"/"+endpoint.TargetRef.Name); found {
					node = pod.Spec.NodeName
				}
			}
			p := port
			if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
				p = *slice.Ports[0].Port
			}
			if node != "" {
				return dialTarget{host: endpoint.Addresses[0], port: strconv.Itoa(int(p))}, node, nil
			}
		}
	}
	return dialTarget{}, "", fmt.Errorf("no ready endpoints for service %s/%s", ns, name)
}

func applyAvailability(obj runtime.Object) {
	if list, ok := obj.(*apiregistrationv1.APIServiceList); ok {
		for i := range list.Items {
			applyAvailability(&list.Items[i])
		}
		return
	}
	svc, ok := obj.(*apiregistrationv1.APIService)
	if !ok {
		return
	}
	if svc.Spec.Service == nil {
		helper.SetAPIServiceCondition(svc, helper.NewLocalAvailableAPIServiceCondition())
		return
	}
	port := int32(443)
	if svc.Spec.Service.Port != nil && *svc.Spec.Service.Port != 0 {
		port = *svc.Spec.Service.Port
	}
	_, _, err := resolveService(context.Background(), svc.Spec.Service.Namespace, svc.Spec.Service.Name, port)
	cond := apiregistrationv1.APIServiceCondition{Type: apiregistrationv1.Available, LastTransitionTime: metav1.Now()}
	if err != nil {
		cond.Status = apiregistrationv1.ConditionFalse
		cond.Reason = "MissingEndpoints"
		cond.Message = err.Error()
	} else {
		cond.Status = apiregistrationv1.ConditionTrue
		cond.Reason = "Passed"
		cond.Message = "all checks passed"
	}
	helper.SetAPIServiceCondition(svc, cond)
}

func getJSON[T any](ctx context.Context, key string) (T, bool) {
	var zero T
	if kineClient == nil {
		return zero, false
	}
	kv, _, err := kineClient.Get(ctx, key)
	if err != nil || kv == nil {
		return zero, false
	}
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return zero, false
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return zero, false
	}
	return v, true
}

func listJSON[T any](ctx context.Context, prefix string) ([]T, error) {
	if kineClient == nil {
		return nil, nil
	}
	kvs, _, _, err := kineClient.List(ctx, prefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(kvs))
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
