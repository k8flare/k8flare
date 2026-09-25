package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

type serviceProxyREST struct {
	services  *genericregistry.Store
	endpoints *genericregistry.Store
	pods      *genericregistry.Store
	proxy     registry.KubeletProxy
}

var (
	_ rest.Connecter = (*serviceProxyREST)(nil)
	_ rest.Storage   = (*serviceProxyREST)(nil)
)

func NewServiceProxyREST(services, endpoints, pods *genericregistry.Store, proxy registry.KubeletProxy) rest.Storage {
	return &serviceProxyREST{services: services, endpoints: endpoints, pods: pods, proxy: proxy}
}

func (serviceProxyREST) New() runtime.Object { return &corev1.ServiceProxyOptions{} }
func (serviceProxyREST) Destroy()            {}
func (serviceProxyREST) ConnectMethods() []string {
	return []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
}
func (serviceProxyREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &corev1.ServiceProxyOptions{}, true, "path"
}

func (r *serviceProxyREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	dial, err := r.dial(ctx, name, opts)
	if err != nil {
		return nil, err
	}
	return newProxyHandler(r.proxy.Base, dial, r.proxy.Transport, responder)
}

func (r *serviceProxyREST) dial(ctx context.Context, id string, opts runtime.Object) (proxyDial, error) {
	scheme, name, port, ok := utilnet.SplitSchemeNamePort(id)
	if !ok {
		return proxyDial{}, apierrors.NewBadRequest(fmt.Sprintf("invalid service request %q", id))
	}
	obj, err := r.services.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return proxyDial{}, err
	}
	svc := obj.(*corev1.Service)
	epObj, err := r.endpoints.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return proxyDial{}, err
	}
	eps := epObj.(*corev1.Endpoints)
	proxyOpts, ok := opts.(*corev1.ServiceProxyOptions)
	if !ok {
		return proxyDial{}, fmt.Errorf("invalid options object: %#v", opts)
	}
	target, err := serviceProxyDial(svc, eps, port, func(addr corev1.EndpointAddress) string {
		return endpointNode(ctx, r.pods, addr)
	})
	if err != nil {
		return proxyDial{}, err
	}
	return proxyDial{node: target.node, host: target.host, port: target.port, path: proxyOpts.Path, scheme: scheme}, nil
}

type serviceDial struct {
	node, host, port string
}

func serviceProxyDial(svc *corev1.Service, eps *corev1.Endpoints, portStr string, nodeOf func(corev1.EndpointAddress) string) (serviceDial, error) {
	wantName, err := servicePortName(svc, portStr)
	if err != nil {
		return serviceDial{}, err
	}
	for _, ss := range eps.Subsets {
		port := matchingEndpointPort(ss.Ports, wantName)
		if port == 0 {
			continue
		}
		for _, addr := range ss.Addresses {
			if net.ParseIP(addr.IP) == nil {
				continue
			}
			node := nodeOf(addr)
			if node == "" {
				continue
			}
			return serviceDial{node: node, host: addr.IP, port: strconv.Itoa(int(port))}, nil
		}
	}
	return serviceDial{}, apierrors.NewServiceUnavailable(fmt.Sprintf("no endpoints available for service %q", svc.Name))
}

func servicePortName(svc *corev1.Service, portStr string) (string, error) {
	if portStr == "" {
		return "", nil
	}
	if n, err := strconv.ParseInt(portStr, 10, 64); err == nil {
		for _, p := range svc.Spec.Ports {
			if int64(p.Port) == n {
				return p.Name, nil
			}
		}
		return "", apierrors.NewServiceUnavailable(fmt.Sprintf("no service port %s found for service %q", portStr, svc.Name))
	}
	return portStr, nil
}

func matchingEndpointPort(ports []corev1.EndpointPort, wantName string) int32 {
	if wantName == "" {
		if len(ports) == 0 {
			return 0
		}
		return ports[0].Port
	}
	for _, p := range ports {
		if p.Name == wantName {
			return p.Port
		}
	}
	return 0
}

func endpointNode(ctx context.Context, pods *genericregistry.Store, addr corev1.EndpointAddress) string {
	if addr.NodeName != nil && *addr.NodeName != "" {
		return *addr.NodeName
	}
	if pods == nil || addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" || addr.TargetRef.Name == "" {
		return ""
	}
	obj, err := pods.Get(ctx, addr.TargetRef.Name, &metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return obj.(*corev1.Pod).Spec.NodeName
}
