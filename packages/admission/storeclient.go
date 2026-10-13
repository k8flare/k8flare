package admission

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

type storeClient struct {
	kubernetes.Interface
	ctx   context.Context
	store *store
}

func (c *storeClient) CoreV1() corev1client.CoreV1Interface {
	return &storeCoreV1{ctx: c.ctx, store: c.store}
}

type storeCoreV1 struct {
	corev1client.CoreV1Interface
	ctx   context.Context
	store *store
}

func (c *storeCoreV1) ServiceAccounts(namespace string) corev1client.ServiceAccountInterface {
	return &storeServiceAccounts{ctx: c.ctx, store: c.store, namespace: namespace}
}

type storeServiceAccounts struct {
	corev1client.ServiceAccountInterface
	ctx       context.Context
	store     *store
	namespace string
}

func (g *storeServiceAccounts) Get(_ context.Context, name string, _ metav1.GetOptions) (*corev1.ServiceAccount, error) {
	sa, ok, err := g.store.serviceAccount(g.ctx, g.namespace, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierrors.NewNotFound(corev1.Resource("serviceaccounts"), name)
	}
	return &sa, nil
}

func (c *storeCoreV1) Namespaces() corev1client.NamespaceInterface {
	return &storeNamespaces{ctx: c.ctx, store: c.store}
}

type storeNamespaces struct {
	corev1client.NamespaceInterface
	ctx   context.Context
	store *store
}

func (g *storeNamespaces) Get(_ context.Context, name string, _ metav1.GetOptions) (*corev1.Namespace, error) {
	ns, ok, err := g.store.namespace(g.ctx, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierrors.NewNotFound(corev1.Resource("namespaces"), name)
	}
	return &ns, nil
}
