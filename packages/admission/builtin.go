package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/kubernetes/plugin/pkg/admission/serviceaccount"
)

func applyServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	return runServiceAccount(ctx, s, req)
}

func validateServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	return runServiceAccount(ctx, s, req)
}

func runServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPluginWithClient(serviceaccount.Register, serviceaccount.PluginName, f, &serviceAccountStoreClient{ctx: ctx, store: s})
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}

type serviceAccountStoreClient struct {
	kubernetes.Interface
	ctx   context.Context
	store *store
}

func (c *serviceAccountStoreClient) CoreV1() corev1client.CoreV1Interface {
	return &serviceAccountStoreCoreV1{ctx: c.ctx, store: c.store}
}

type serviceAccountStoreCoreV1 struct {
	corev1client.CoreV1Interface
	ctx   context.Context
	store *store
}

func (c *serviceAccountStoreCoreV1) ServiceAccounts(namespace string) corev1client.ServiceAccountInterface {
	return &serviceAccountStoreGetter{ctx: c.ctx, store: c.store, namespace: namespace}
}

type serviceAccountStoreGetter struct {
	corev1client.ServiceAccountInterface
	ctx       context.Context
	store     *store
	namespace string
}

func (g *serviceAccountStoreGetter) Get(_ context.Context, name string, _ metav1.GetOptions) (*corev1.ServiceAccount, error) {
	sa, ok, err := g.store.serviceAccount(g.ctx, g.namespace, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierrors.NewNotFound(corev1.Resource("serviceaccounts"), name)
	}
	return &sa, nil
}
