package core

import (
	"context"
	"fmt"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	genericrest "k8s.io/apiserver/pkg/registry/generic/rest"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/transport"
	nodeutil "k8s.io/kubernetes/pkg/util/node"
)

type logREST struct {
	pods  *genericregistry.Store
	nodes *genericregistry.Store
	proxy registry.KubeletProxy
}

var _ rest.GetterWithOptions = (*logREST)(nil)

func NewLogREST(pods, nodes *genericregistry.Store, proxy registry.KubeletProxy) rest.Storage {
	return &logREST{pods: pods, nodes: nodes, proxy: proxy}
}

func (r *logREST) New() runtime.Object { return &corev1.Pod{} }
func (r *logREST) Destroy()            {}
func (r *logREST) NewGetOptions() (runtime.Object, bool, string) {
	return &corev1.PodLogOptions{}, false, ""
}

func (r *logREST) Get(ctx context.Context, name string, opts runtime.Object) (runtime.Object, error) {
	logOpts, ok := opts.(*corev1.PodLogOptions)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	obj, err := r.pods.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	pod := obj.(*corev1.Pod)
	if pod.Spec.NodeName == "" {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("pod %s is not scheduled", name))
	}
	if logOpts.Container == "" {
		if len(pod.Spec.Containers) != 1 {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("a container name must be specified for pod %s", name))
		}
		logOpts.Container = pod.Spec.Containers[0].Name
	}
	nodeObj, err := r.nodes.Get(ctx, pod.Spec.NodeName, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	host, err := nodeutil.GetPreferredNodeAddress(nodeObj.(*corev1.Node), []corev1.NodeAddressType{corev1.NodeInternalIP})
	if err != nil {
		return nil, apierrors.NewServiceUnavailable(err.Error())
	}
	query, err := scheme.ParameterCodec.EncodeParameters(logOpts, corev1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	query.Del("container")
	return &genericrest.LocationStreamer{
		Location: &url.URL{
			Scheme:   r.proxy.Scheme,
			Host:     fmt.Sprintf("%s:%d", host, r.proxy.Port),
			Path:     fmt.Sprintf("/containerLogs/%s/%s/%s", pod.Namespace, pod.Name, logOpts.Container),
			RawQuery: query.Encode(),
		},
		Transport:       transport.NewBearerAuthRoundTripper(r.proxy.Token, http.DefaultTransport),
		ContentType:     "text/plain",
		Flush:           logOpts.Follow,
		ResponseChecker: genericrest.NewGenericHttpResponseChecker(corev1.Resource("pods/log"), name),
		RedirectChecker: genericrest.PreventRedirects,
	}, nil
}
