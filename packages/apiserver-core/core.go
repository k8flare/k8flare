package core

import (
	"context"
	"net/http"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
	registry.Customizers["pods"] = func(store *registry.Store, _ registry.Deps) {
		store.CreateStrategy = podCreateStrategy{store.CreateStrategy}
		store.DeleteStrategy = podStrategy{store.DeleteStrategy}
		registry.PokeControllersOn(store)
		store.BeginCreate = func(ctx context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			return pokeBothFor(obj), nil
		}
		store.BeginUpdate = func(ctx context.Context, obj, _ runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			return pokeBothFor(obj), nil
		}
		store.AfterDelete = func(runtime.Object, *metav1.DeleteOptions) {
			pokeBoth(context.Background(), true)
		}
	}
	registry.Customizers["nodes"] = func(store *registry.Store, _ registry.Deps) {
		registry.PokeControllersOn(store)
		store.BeginCreate = func(context.Context, runtime.Object, *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			return pokeBoth, nil
		}
		store.BeginUpdate = func(_ context.Context, obj, old runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			if nodeChanged(old.(*corev1.Node), obj.(*corev1.Node)) {
				return pokeBoth, nil
			}
			return func(context.Context, bool) {}, nil
		}
	}
	for _, resource := range []string{"services", "endpoints", "replicationcontrollers", "serviceaccounts"} {
		registry.Customizers[resource] = func(store *registry.Store, _ registry.Deps) { registry.PokeControllersOn(store) }
	}
	registry.Customizers["namespaces"] = func(store *registry.Store, _ registry.Deps) {
		registry.PokeControllersOn(store)
		store.CreateStrategy = namespaceCreateStrategy{store.CreateStrategy}
		store.UpdateStrategy = namespaceUpdateStrategy{store.UpdateStrategy}
		store.ShouldDeleteDuringUpdate = shouldDeleteNamespaceDuringUpdate
	}
	registry.Deleters["namespaces"] = func(store *registry.Store) rest.GracefulDeleter { return namespaceDeleter{store} }
	registry.Subresources["namespaces/finalize"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return registry.NewUpdateOnlyREST(stores["namespaces"], namespaceFinalizeStrategy{stores["namespaces"].UpdateStrategy})
	}
	registry.Subresources["pods/log"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewLogREST(stores["pods"], stores["nodes"], deps.Kubelet)
	}
	registry.Subresources["pods/status"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return registry.NewUpdateOnlyREST(stores["pods"], podStatusStrategy{registry.StatusOnly(stores["pods"].UpdateStrategy)})
	}
	registry.Subresources["pods/binding"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return bindingREST{stores["pods"]}
	}
	registry.Middleware = append(registry.Middleware, func(stores map[string]*registry.Store) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return bootstrapCluster(stores["namespaces"], stores["services"], next)
		}
	})
}

func nodeChanged(old, node *corev1.Node) bool {
	return !equality.Semantic.DeepEqual(old.Spec, node.Spec) ||
		!equality.Semantic.DeepEqual(old.Labels, node.Labels) ||
		!equality.Semantic.DeepEqual(old.Status.Allocatable, node.Status.Allocatable) ||
		nodeReady(old) != nodeReady(node)
}

func nodeReady(node *corev1.Node) corev1.ConditionStatus {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status
		}
	}
	return corev1.ConditionUnknown
}

func pokeBothFor(obj runtime.Object) genericregistry.FinishFunc {
	if obj.(*corev1.Pod).Spec.NodeName != "" {
		return pokeControllersOnly
	}
	return pokeBoth
}

func pokeBoth(ctx context.Context, success bool) {
	if success && registry.Poke != nil {
		registry.Poke(ctx)
	}
	pokeControllersOnly(ctx, success)
}

func pokeControllersOnly(ctx context.Context, success bool) {
	if success && registry.PokeControllers != nil {
		registry.PokeControllers(ctx)
	}
}

type podStrategy struct{ rest.RESTDeleteStrategy }

func (podStrategy) CheckGracefulDelete(_ context.Context, obj runtime.Object, options *metav1.DeleteOptions) bool {
	if options == nil {
		return false
	}
	pod := obj.(*corev1.Pod)
	period := int64(0)
	if options.GracePeriodSeconds != nil {
		period = *options.GracePeriodSeconds
	} else if pod.Spec.TerminationGracePeriodSeconds != nil {
		period = *pod.Spec.TerminationGracePeriodSeconds
	}
	if pod.Spec.NodeName == "" {
		period = 0
	}
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		period = 0
	}
	if period < 0 {
		period = 1
	}
	options.GracePeriodSeconds = &period
	return true
}
