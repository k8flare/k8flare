package apiregistration

import (
	"context"
	"errors"
	"net/http"
	"sync"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	helper "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1/helper"
)

func init() {
	registry.Customizers["apiservices"] = func(store *registry.Store, deps registry.Deps) {
		kineClient = deps.Kine
		tunnel = deps.Kubelet.Transport
		if deps.Hooks != nil {
			hooks = deps.Hooks.Transport
		}
		store.BeginCreate = markLocalAvailable
		store.BeginUpdate = markLocalAvailableUpdate
		store.Decorator = applyAvailability
	}
	registry.Middleware = append(registry.Middleware, func(stores map[string]*registry.Store) func(http.Handler) http.Handler {
		locals := bootstrapLocalAPIServices(stores["apiservices"])
		return func(next http.Handler) http.Handler {
			return locals(proxyRemote()(next))
		}
	})
}

func markLocalAvailable(_ context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
	markLocal(obj)
	return func(context.Context, bool) {}, nil
}

func markLocalAvailableUpdate(_ context.Context, obj, _ runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
	markLocal(obj)
	return func(context.Context, bool) {}, nil
}

func markLocal(obj runtime.Object) {
	svc, ok := obj.(*apiregistrationv1.APIService)
	if !ok || isRemote(svc) {
		return
	}
	helper.SetAPIServiceCondition(svc, helper.NewLocalAvailableAPIServiceCondition())
}

func bootstrapLocalAPIServices(store *genericregistry.Store) func(http.Handler) http.Handler {
	var once sync.Once
	hash := localAPIServicesHash()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			once.Do(func() {
				ctx := context.WithoutCancel(r.Context())
				_ = registry.RunBootstrap(ctx, store, "apiregistration", hash, func(ctx context.Context) error {
					var failed error
					reqCtx := genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
					for _, sgv := range registry.Served {
						failed = errors.Join(failed, createLocal(reqCtx, store, sgv.GV.Group, sgv.GV.Version))
					}
					return failed
				})
			})
			next.ServeHTTP(w, r)
		})
	}
}

func localAPIServicesHash() string {
	return registry.HashObjects(registry.Served)
}

func createLocal(ctx context.Context, store *genericregistry.Store, group, version string) error {
	name := version + "." + group
	obj := &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apiregistrationv1.APIServiceSpec{
			Group:                group,
			Version:              version,
			GroupPriorityMinimum: 18000,
			VersionPriority:      15,
		},
	}
	helper.SetAPIServiceCondition(obj, helper.NewLocalAvailableAPIServiceCondition())
	if _, err := store.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap", name, ":", err.Error())
		return err
	}
	return nil
}
