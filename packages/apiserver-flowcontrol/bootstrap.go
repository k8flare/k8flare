package flowcontrol

import (
	"context"
	"errors"
	"net/http"
	"sync"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	flowcontrolbootstrap "k8s.io/apiserver/pkg/apis/flowcontrol/bootstrap"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
	registry.Middleware = append(registry.Middleware, func(stores map[string]*registry.Store) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return bootstrapAPF(stores["prioritylevelconfigurations"], stores["flowschemas"], next)
		}
	})
}

func bootstrapAPF(plcs, schemas *genericregistry.Store, next http.Handler) http.Handler {
	if plcs == nil || schemas == nil {
		return next
	}
	var once sync.Once
	hash := flowcontrolBootstrapHash()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ctx := context.WithoutCancel(r.Context())
			_ = registry.RunBootstrap(ctx, plcs, "flowcontrol", hash, func(ctx context.Context) error {
				return ensureAPF(ctx, plcs, schemas)
			})
		})
		next.ServeHTTP(w, r)
	})
}

func flowcontrolBootstrapHash() string {
	return registry.HashObjects(
		flowcontrolbootstrap.MandatoryPriorityLevelConfigurations,
		flowcontrolbootstrap.SuggestedPriorityLevelConfigurations,
		flowcontrolbootstrap.MandatoryFlowSchemas,
		flowcontrolbootstrap.SuggestedFlowSchemas,
	)
}

func ensureAPF(ctx context.Context, plcs, schemas *genericregistry.Store) error {
	var failed error
	for _, plc := range append(append([]*flowcontrolv1.PriorityLevelConfiguration{}, flowcontrolbootstrap.MandatoryPriorityLevelConfigurations...), flowcontrolbootstrap.SuggestedPriorityLevelConfigurations...) {
		failed = errors.Join(failed, ensurePLC(ctx, plcs, plc))
	}
	for _, fs := range append(append([]*flowcontrolv1.FlowSchema{}, flowcontrolbootstrap.MandatoryFlowSchemas...), flowcontrolbootstrap.SuggestedFlowSchemas...) {
		failed = errors.Join(failed, ensureFlowSchema(ctx, schemas, fs))
	}
	return failed
}

func ensurePLC(ctx context.Context, store *genericregistry.Store, plc *flowcontrolv1.PriorityLevelConfiguration) error {
	obj := plc.DeepCopy()
	ctx = genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "flowcontrol.apiserver.k8s.io",
		APIVersion:        "v1",
		Resource:          "prioritylevelconfigurations",
		Name:              obj.Name,
	})
	if _, err := store.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap prioritylevelconfiguration:", obj.Name, err.Error())
		return err
	}
	return nil
}

func ensureFlowSchema(ctx context.Context, store *genericregistry.Store, fs *flowcontrolv1.FlowSchema) error {
	obj := fs.DeepCopy()
	ctx = genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "flowcontrol.apiserver.k8s.io",
		APIVersion:        "v1",
		Resource:          "flowschemas",
		Name:              obj.Name,
	})
	if _, err := store.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap flowschema:", obj.Name, err.Error())
		return err
	}
	return nil
}
