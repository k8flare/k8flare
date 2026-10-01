package customresources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiserver/pkg/util/webhook"
	"k8s.io/client-go/rest"
)

const (
	workerURLHost = "k8flare.com"
	workerURLPath = "/worker/"
	workerAnnot   = "k8flare.com/worker"
	hooksBaseHost = "hooks.internal"
	hooksBasePath = "/hook/"
)

type conversionResolver struct {
	kine     *kine.Client
	tunnel   http.RoundTripper
	outbound http.RoundTripper
	hooks    http.RoundTripper
}

func newConversionResolver(client *kine.Client, tunnel, outbound, hooks *http.Client) *conversionResolver {
	r := &conversionResolver{kine: client}
	if tunnel != nil {
		r.tunnel = tunnel.Transport
	}
	if outbound != nil {
		r.outbound = outbound.Transport
	}
	if hooks != nil {
		r.hooks = hooks.Transport
	}
	return r
}

func workerNameFromRequest(req *http.Request) (string, bool) {
	if req == nil || req.URL == nil || req.URL.Hostname() != workerURLHost {
		return "", false
	}
	name := strings.Trim(strings.TrimPrefix(req.URL.Path, workerURLPath), "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

func routeConversionToWorker(crd *apiextensionsv1.CustomResourceDefinition) {
	name := strings.Trim(strings.TrimSpace(crd.Annotations[workerAnnot]), "/")
	if name == "" || strings.Contains(name, "/") {
		return
	}
	conversion := crd.Spec.Conversion
	if conversion == nil || conversion.Strategy != apiextensionsv1.WebhookConverter || conversion.Webhook == nil || conversion.Webhook.ClientConfig == nil {
		return
	}
	workerURL := "https://" + workerURLHost + workerURLPath + name
	conversion.Webhook.ClientConfig.URL = &workerURL
	conversion.Webhook.ClientConfig.Service = nil
}

func (r *conversionResolver) urlTransport() http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		if name, ok := workerNameFromRequest(req); ok && r.hooks != nil {
			u := *req.URL
			u.Scheme = "https"
			u.Host = hooksBaseHost
			u.Path = hooksBasePath + name
			u.RawPath = ""
			req.URL = &u
			return r.hooks.RoundTrip(req)
		}
		if r.outbound == nil {
			return nil, fmt.Errorf("conversion webhook: no outbound client")
		}
		return r.outbound.RoundTrip(req)
	})
}

func (r *conversionResolver) install() webhook.AuthenticationInfoResolverWrapper {
	webhook.WebhookTransportSetup = r.setup
	return func(delegate webhook.AuthenticationInfoResolver) webhook.AuthenticationInfoResolver {
		return &webhook.AuthenticationInfoResolverDelegator{
			ClientConfigForFunc: func(hostPort string) (*rest.Config, error) {
				cfg, err := delegate.ClientConfigFor(hostPort)
				if err != nil {
					return nil, err
				}
				cfg.Transport = r.urlTransport()
				return cfg, nil
			},
			ClientConfigForServiceFunc: func(name, ns string, port int) (*rest.Config, error) {
				cfg, err := delegate.ClientConfigForService(name, ns, port)
				if err != nil {
					return nil, err
				}
				if r.tunnel != nil {
					cfg.Transport = &serviceDialTransport{kine: r.kine, next: r.tunnel}
				}
				return cfg, nil
			},
		}
	}
}

func (r *conversionResolver) setup(cfg *rest.Config, serverName string, ca []byte) {
	inner := cfg.Transport
	if inner == nil {
		return
	}
	cfg.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		if _, ok := workerNameFromRequest(req); ok {
			return inner.RoundTrip(req)
		}
		req.Header.Set("X-Dial-TLS", "1")
		req.Header.Set("X-Dial-ServerName", serverName)
		if len(ca) > 0 {
			req.Header.Set("X-Dial-CA", base64.StdEncoding.EncodeToString(ca))
		}
		return inner.RoundTrip(req)
	})
}

type serviceDialTransport struct {
	kine *kine.Client
	next http.RoundTripper
}

func (t *serviceDialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	name, ns, port, err := splitServiceHost(req.URL.Host)
	if err != nil {
		return nil, err
	}
	target, node, err := resolveService(req.Context(), t.kine, ns, name, port)
	if err != nil {
		return nil, err
	}
	out := req.Clone(req.Context())
	out.RequestURI = ""
	u := *req.URL
	u.Scheme = "https"
	u.Host = "nodetunnel.internal"
	u.Path = "/dial/" + node + "/" + target.host + "/" + target.port + req.URL.Path
	u.RawPath = ""
	out.URL = &u
	return t.next.RoundTrip(out)
}

type dialTarget struct{ host, port string }

func splitServiceHost(hostport string) (name, ns string, port int32, err error) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", "", 0, err
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 {
		return "", "", 0, fmt.Errorf("invalid service port %q", portStr)
	}
	host = strings.TrimSuffix(host, ".svc")
	parts := strings.Split(host, ".")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", 0, fmt.Errorf("invalid service host %q", hostport)
	}
	return parts[0], parts[1], int32(p), nil
}

func resolveService(ctx context.Context, client *kine.Client, ns, name string, port int32) (dialTarget, string, error) {
	if ep, ok, err := getJSON[corev1.Endpoints](ctx, client, "/registry/endpoints/"+ns+"/"+name); err != nil {
		return dialTarget{}, "", err
	} else if ok {
		for _, subset := range ep.Subsets {
			for _, addr := range subset.Addresses {
				p := port
				for _, sp := range subset.Ports {
					if sp.Port != 0 {
						p = sp.Port
						break
					}
				}
				node := ""
				if addr.NodeName != nil {
					node = *addr.NodeName
				}
				if node == "" && addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
					pod, found, err := getJSON[corev1.Pod](ctx, client, "/registry/pods/"+addr.TargetRef.Namespace+"/"+addr.TargetRef.Name)
					if err != nil {
						return dialTarget{}, "", err
					}
					if found {
						node = pod.Spec.NodeName
					}
				}
				if addr.IP != "" && node != "" {
					return dialTarget{host: addr.IP, port: strconv.Itoa(int(p))}, node, nil
				}
			}
		}
	}
	slices, err := listJSON[discoveryv1.EndpointSlice](ctx, client, "/registry/endpointslices/"+ns+"/")
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
			node := ""
			if endpoint.NodeName != nil {
				node = *endpoint.NodeName
			}
			if node == "" && endpoint.TargetRef != nil && endpoint.TargetRef.Kind == "Pod" {
				pod, found, err := getJSON[corev1.Pod](ctx, client, "/registry/pods/"+endpoint.TargetRef.Namespace+"/"+endpoint.TargetRef.Name)
				if err != nil {
					return dialTarget{}, "", err
				}
				if found {
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

func getJSON[T any](ctx context.Context, client *kine.Client, key string) (T, bool, error) {
	var zero T
	kv, _, err := client.Get(ctx, key)
	if err == kine.ErrNotFound {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return zero, false, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return zero, false, err
	}
	return v, true, nil
}

func listJSON[T any](ctx context.Context, client *kine.Client, prefix string) ([]T, error) {
	kvs, _, _, err := client.List(ctx, prefix, "", 0)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
