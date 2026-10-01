package core

import (
	"context"
	"net/http"
	"sync"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
	utilnet "k8s.io/utils/net"
)

var systemNamespaces = []string{"default", "kube-system", "kube-public", "kube-node-lease"}

func bootstrapCluster(namespaces, services *genericregistry.Store, next http.Handler) http.Handler {
	var once sync.Once
	hash := coreBootstrapHash()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ctx := context.WithoutCancel(r.Context())
			_ = registry.RunBootstrap(ctx, namespaces, "core", hash, func(ctx context.Context) error {
				nsCtx := genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
				for _, name := range systemNamespaces {
					if err := create(nsCtx, namespaces, systemNamespace(name)); err != nil {
						return err
					}
				}
				if err := create(genericapirequest.WithNamespace(ctx, metav1.NamespaceDefault), services, kubernetesService()); err != nil {
					return err
				}
				reconcileKubernetesEndpoints(ctx)
				ensureExtensionAuth(ctx)
				return nil
			})
		})
		next.ServeHTTP(w, r)
	})
}

func coreBootstrapHash() string {
	return registry.HashObjects(systemNamespaces, kubernetesService())
}

func create(ctx context.Context, store *genericregistry.Store, obj metav1.Object) error {
	if _, err := store.Create(ctx, bootstrapObject(obj.(runtime.Object)), rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap", obj.GetName(), ":", err.Error())
		return err
	}
	return nil
}

func bootstrapObject(obj runtime.Object) runtime.Object {
	scheme.Scheme.Default(obj)
	return obj
}

func systemNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
}

func kubernetesService() *corev1.Service {
	clusterIP, _ := utilnet.GetIndexedIP(supervisor.ServiceCIDR, 1)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: metav1.NamespaceDefault, Labels: map[string]string{"component": "apiserver", "provider": "kubernetes"}},
		Spec: corev1.ServiceSpec{
			Type:            corev1.ServiceTypeClusterIP,
			ClusterIP:       clusterIP.String(),
			ClusterIPs:      []string{clusterIP.String()},
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports:           []corev1.ServicePort{{Name: "https", Protocol: corev1.ProtocolTCP, Port: 443, TargetPort: intstr.FromInt32(6443)}},
		},
	}
}
