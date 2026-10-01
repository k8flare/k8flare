package networking

import (
	"context"
	"net/http"
	"sync"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

const defaultServiceCIDRName = "kubernetes"

func init() {
	registry.Middleware = append(registry.Middleware, func(stores map[string]*registry.Store) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return bootstrapServiceCIDR(stores["servicecidrs"], next)
		}
	})
}

func bootstrapServiceCIDR(store *genericregistry.Store, next http.Handler) http.Handler {
	if store == nil {
		return next
	}
	var once sync.Once
	hash := networkingBootstrapHash()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ctx := context.WithoutCancel(r.Context())
			_ = registry.RunBootstrap(ctx, store, "networking", hash, func(ctx context.Context) error {
				return ensureDefaultServiceCIDR(ctx, store)
			})
		})
		next.ServeHTTP(w, r)
	})
}

func networkingBootstrapHash() string {
	return registry.HashObjects(defaultServiceCIDRName, supervisor.ServiceCIDR.String())
}

func ensureDefaultServiceCIDR(ctx context.Context, store *genericregistry.Store) error {
	ctx = genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "networking.k8s.io",
		APIVersion:        "v1",
		Resource:          "servicecidrs",
		Name:              defaultServiceCIDRName,
	})
	if _, err := store.Create(ctx, defaultServiceCIDR(), rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap servicecidr:", err.Error())
		return err
	}
	return nil
}

func defaultServiceCIDR() runtime.Object {
	now := metav1.Now()
	return &networkingv1.ServiceCIDR{
		ObjectMeta: metav1.ObjectMeta{Name: defaultServiceCIDRName},
		Spec:       networkingv1.ServiceCIDRSpec{CIDRs: []string{supervisor.ServiceCIDR.String()}},
		Status: networkingv1.ServiceCIDRStatus{
			Conditions: []metav1.Condition{{
				Type:               networkingv1.ServiceCIDRConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Ready",
				Message:            "Kubernetes default Service CIDR is ready",
				LastTransitionTime: now,
			}},
		},
	}
}
