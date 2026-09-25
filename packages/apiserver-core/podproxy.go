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

type podProxyREST struct {
	pods  *genericregistry.Store
	proxy registry.KubeletProxy
}

var (
	_ rest.Connecter = (*podProxyREST)(nil)
	_ rest.Storage   = (*podProxyREST)(nil)
)

func NewPodProxyREST(pods *genericregistry.Store, proxy registry.KubeletProxy) rest.Storage {
	return &podProxyREST{pods: pods, proxy: proxy}
}

func (podProxyREST) New() runtime.Object { return &corev1.PodProxyOptions{} }
func (podProxyREST) Destroy()            {}
func (podProxyREST) ConnectMethods() []string {
	return []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
}
func (podProxyREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &corev1.PodProxyOptions{}, true, "path"
}

func (r *podProxyREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	dial, err := r.dial(ctx, name, opts)
	if err != nil {
		return nil, err
	}
	return newProxyHandler(r.proxy.Base, dial, r.proxy.Transport, responder)
}

func (r *podProxyREST) dial(ctx context.Context, id string, opts runtime.Object) (proxyDial, error) {
	scheme, name, port, ok := utilnet.SplitSchemeNamePort(id)
	if !ok {
		return proxyDial{}, apierrors.NewBadRequest(fmt.Sprintf("invalid pod request %q", id))
	}
	obj, err := r.pods.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return proxyDial{}, err
	}
	pod := obj.(*corev1.Pod)
	proxyOpts, ok := opts.(*corev1.PodProxyOptions)
	if !ok {
		return proxyDial{}, fmt.Errorf("invalid options object: %#v", opts)
	}
	if pod.Spec.NodeName == "" {
		return proxyDial{}, apierrors.NewBadRequest(fmt.Sprintf("pod %s is not scheduled", pod.Name))
	}
	ip := pod.Status.PodIP
	if net.ParseIP(ip) == nil {
		return proxyDial{}, apierrors.NewBadRequest("address not allowed")
	}
	if port == "" {
		port = firstContainerPort(pod)
	}
	if port == "" {
		port = "80"
	}
	return proxyDial{node: pod.Spec.NodeName, host: ip, port: port, path: proxyOpts.Path, scheme: scheme}, nil
}

func firstContainerPort(pod *corev1.Pod) string {
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.ContainerPort > 0 {
				return strconv.Itoa(int(p.ContainerPort))
			}
		}
	}
	return ""
}
