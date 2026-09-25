package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/proxy"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

type streamREST struct {
	kind  string
	pods  *genericregistry.Store
	proxy registry.KubeletProxy
}

var (
	_ rest.Connecter = (*streamREST)(nil)
	_ rest.Storage   = (*streamREST)(nil)
)

func NewExecREST(pods *genericregistry.Store, proxy registry.KubeletProxy) rest.Storage {
	return &streamREST{kind: "exec", pods: pods, proxy: proxy}
}

func NewAttachREST(pods *genericregistry.Store, proxy registry.KubeletProxy) rest.Storage {
	return &streamREST{kind: "attach", pods: pods, proxy: proxy}
}

func NewPortForwardREST(pods *genericregistry.Store, proxy registry.KubeletProxy) rest.Storage {
	return &streamREST{kind: "portForward", pods: pods, proxy: proxy}
}

func (r *streamREST) New() runtime.Object {
	switch r.kind {
	case "attach":
		return &corev1.PodAttachOptions{}
	case "portForward":
		return &corev1.PodPortForwardOptions{}
	default:
		return &corev1.PodExecOptions{}
	}
}

func (r *streamREST) Destroy() {}

func (r *streamREST) ConnectMethods() []string { return []string{"GET", "POST"} }

func (r *streamREST) NewConnectOptions() (runtime.Object, bool, string) {
	return r.New(), false, ""
}

func (r *streamREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	loc, err := r.location(ctx, name, opts)
	if err != nil {
		return nil, err
	}
	return proxy.NewUpgradeAwareHandler(loc, r.proxy.Transport, false, true, proxy.NewErrorResponder(responder)), nil
}

func (r *streamREST) location(ctx context.Context, name string, opts runtime.Object) (*url.URL, error) {
	obj, err := r.pods.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	pod := obj.(*corev1.Pod)
	if pod.Spec.NodeName == "" {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("pod %s is not scheduled", name))
	}
	container, err := streamContainer(r.kind, pod, opts)
	if err != nil {
		return nil, err
	}
	query := kubeletStreamParams(opts)
	base, err := url.Parse(r.proxy.Base)
	if err != nil {
		return nil, apierrors.NewServiceUnavailable(err.Error())
	}
	switch r.kind {
	case "portForward":
		base.Path = fmt.Sprintf("/node/%s/portForward/%s/%s", pod.Spec.NodeName, pod.Namespace, pod.Name)
	default:
		base.Path = fmt.Sprintf("/node/%s/%s/%s/%s/%s", pod.Spec.NodeName, r.kind, pod.Namespace, pod.Name, container)
	}
	base.RawQuery = query.Encode()
	return base, nil
}

func kubeletStreamParams(opts runtime.Object) url.Values {
	q := url.Values{}
	switch o := opts.(type) {
	case *corev1.PodExecOptions:
		setStreamFlag(q, "input", o.Stdin)
		setStreamFlag(q, "output", o.Stdout)
		setStreamFlag(q, "error", o.Stderr)
		setStreamFlag(q, "tty", o.TTY)
		for _, c := range o.Command {
			q.Add("command", c)
		}
	case *corev1.PodAttachOptions:
		setStreamFlag(q, "input", o.Stdin)
		setStreamFlag(q, "output", o.Stdout)
		setStreamFlag(q, "error", o.Stderr)
		setStreamFlag(q, "tty", o.TTY)
	case *corev1.PodPortForwardOptions:
		for _, p := range o.Ports {
			q.Add("port", fmt.Sprintf("%d", p))
		}
	}
	return q
}

func setStreamFlag(q url.Values, key string, on bool) {
	if on {
		q.Set(key, "1")
	}
}

func streamContainer(kind string, pod *corev1.Pod, opts runtime.Object) (string, error) {
	if kind == "portForward" {
		return "", nil
	}
	container := ""
	switch o := opts.(type) {
	case *corev1.PodExecOptions:
		container = o.Container
	case *corev1.PodAttachOptions:
		container = o.Container
	}
	if container != "" {
		return container, nil
	}
	if len(pod.Spec.Containers) != 1 {
		return "", apierrors.NewBadRequest(fmt.Sprintf("a container name must be specified for pod %s", pod.Name))
	}
	return pod.Spec.Containers[0].Name, nil
}
