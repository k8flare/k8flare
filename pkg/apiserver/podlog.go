package apiserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

// KubeletProxy says how the apiserver reaches a node's kubelet API.
type KubeletProxy struct {
	// HTTP performs the request; in the Worker it is the plain fetch.
	HTTP *http.Client
	// Port is the kubelet port to dial on the node's InternalIP; Scheme is
	// http or https. Production will route this through the node tunnel.
	Scheme string
	Port   int
	// Token authenticates the apiserver to the kubelet, which checks it
	// back with a TokenReview.
	Token string
}

// logREST serves pods/log by proxying the kubelet's /containerLogs, the
// way upstream's LogREST does.
type logREST struct {
	pods  *genericregistry.Store
	nodes *genericregistry.Store
	proxy KubeletProxy
}

var _ rest.Connecter = (*logREST)(nil)

func (r *logREST) New() runtime.Object { return &corev1.Pod{} }
func (r *logREST) Destroy()            {}
func (r *logREST) ConnectMethods() []string {
	return []string{"GET"}
}
func (r *logREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &corev1.PodLogOptions{}, false, ""
}

func (r *logREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
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
	container := logOpts.Container
	if container == "" {
		if len(pod.Spec.Containers) != 1 {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("a container name must be specified for pod %s", name))
		}
		container = pod.Spec.Containers[0].Name
	}
	nodeObj, err := r.nodes.Get(ctx, pod.Spec.NodeName, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	host := ""
	for _, addr := range nodeObj.(*corev1.Node).Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			host = addr.Address
			break
		}
	}
	if host == "" {
		return nil, apierrors.NewServiceUnavailable(fmt.Sprintf("node %s has no InternalIP", pod.Spec.NodeName))
	}
	target := url.URL{
		Scheme:   r.proxy.Scheme,
		Host:     fmt.Sprintf("%s:%d", host, r.proxy.Port),
		Path:     fmt.Sprintf("/containerLogs/%s/%s/%s", pod.Namespace, pod.Name, container),
		RawQuery: logQuery(logOpts).Encode(),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upstream, err := http.NewRequestWithContext(req.Context(), http.MethodGet, target.String(), nil)
		if err != nil {
			responder.Error(err)
			return
		}
		upstream.Header.Set("Authorization", "Bearer "+r.proxy.Token)
		resp, err := r.proxy.HTTP.Do(upstream)
		if err != nil {
			responder.Error(apierrors.NewServiceUnavailable(fmt.Sprintf("kubelet %s: %v", target.Host, err)))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			responder.Error(apierrors.NewGenericServerResponse(resp.StatusCode, "get", corev1.Resource("pods/log"), name, string(body), 0, true))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(resp.StatusCode)
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				return
			}
		}
	}), nil
}

func logQuery(o *corev1.PodLogOptions) url.Values {
	q := url.Values{}
	if o.Follow {
		q.Set("follow", "true")
	}
	if o.Previous {
		q.Set("previous", "true")
	}
	if o.Timestamps {
		q.Set("timestamps", "true")
	}
	if o.SinceSeconds != nil {
		q.Set("sinceSeconds", strconv.FormatInt(*o.SinceSeconds, 10))
	}
	if o.SinceTime != nil {
		q.Set("sinceTime", o.SinceTime.Format("2006-01-02T15:04:05Z07:00"))
	}
	if o.TailLines != nil {
		q.Set("tailLines", strconv.FormatInt(*o.TailLines, 10))
	}
	if o.LimitBytes != nil {
		q.Set("limitBytes", strconv.FormatInt(*o.LimitBytes, 10))
	}
	return q
}
