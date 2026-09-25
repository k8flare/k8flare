package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/proxy"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

type nodeProxyREST struct {
	nodes *genericregistry.Store
	proxy registry.KubeletProxy
}

var (
	_ rest.Connecter = (*nodeProxyREST)(nil)
	_ rest.Storage   = (*nodeProxyREST)(nil)
)

func NewNodeProxyREST(nodes *genericregistry.Store, proxy registry.KubeletProxy) rest.Storage {
	return &nodeProxyREST{nodes: nodes, proxy: proxy}
}

func (nodeProxyREST) New() runtime.Object { return &corev1.NodeProxyOptions{} }
func (nodeProxyREST) Destroy()            {}
func (nodeProxyREST) ConnectMethods() []string {
	return []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
}
func (nodeProxyREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &corev1.NodeProxyOptions{}, true, "path"
}

func (r *nodeProxyREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	loc, err := r.location(ctx, name, opts)
	if err != nil {
		return nil, err
	}
	return proxy.NewUpgradeAwareHandler(loc, r.proxy.Transport, true, false, proxy.NewErrorResponder(responder)), nil
}

func (r *nodeProxyREST) location(ctx context.Context, id string, opts runtime.Object) (*url.URL, error) {
	name, _, err := nodeProxyID(id)
	if err != nil {
		return nil, err
	}
	if _, err := r.nodes.Get(ctx, name, &metav1.GetOptions{}); err != nil {
		return nil, err
	}
	proxyOpts, ok := opts.(*corev1.NodeProxyOptions)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	return nodeProxyURL(r.proxy.Base, name, proxyOpts.Path)
}

func nodeProxyID(id string) (name, port string, err error) {
	_, name, port, ok := utilnet.SplitSchemeNamePort(id)
	if !ok {
		return "", "", apierrors.NewBadRequest(fmt.Sprintf("invalid node request %q", id))
	}
	return name, port, nil
}

func nodeProxyURL(base, node, path string) (*url.URL, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, apierrors.NewServiceUnavailable(err.Error())
	}
	u.Path = "/node/" + node + "/" + strings.TrimPrefix(path, "/")
	return u, nil
}
