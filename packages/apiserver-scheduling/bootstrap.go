package scheduling

import (
	"context"
	"net/http"
	"sync"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	schedhelpers "k8s.io/kubernetes/pkg/apis/scheduling/v1"
)

func init() {
	registry.Middleware = append(registry.Middleware, func(stores map[string]*registry.Store) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return bootstrapPriorityClasses(stores["priorityclasses"], next)
		}
	})
}

func bootstrapPriorityClasses(store *genericregistry.Store, next http.Handler) http.Handler {
	if store == nil {
		return next
	}
	var once sync.Once
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ensureSystemPriorityClasses(r.Context(), store)
		})
		next.ServeHTTP(w, r)
	})
}

func ensureSystemPriorityClasses(ctx context.Context, store *genericregistry.Store) {
	for _, pc := range schedhelpers.SystemPriorityClasses() {
		ensurePriorityClass(ctx, store, pc)
	}
}

func ensurePriorityClass(ctx context.Context, store *genericregistry.Store, pc *schedulingv1.PriorityClass) {
	ctx = genericapirequest.WithNamespace(ctx, metav1.NamespaceNone)
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{
		IsResourceRequest: true,
		Verb:              "create",
		APIGroup:          "scheduling.k8s.io",
		APIVersion:        "v1",
		Resource:          "priorityclasses",
		Name:              pc.Name,
	})
	if _, err := store.Create(ctx, pc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		println("apiserver: bootstrap priorityclass:", pc.Name, err.Error())
	}
}
