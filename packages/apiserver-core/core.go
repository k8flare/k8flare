package core

import (
	"context"
	"net/http"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
	registry.Customizers["pods"] = func(store *registry.Store, _ registry.Deps) {
		store.CreateStrategy = podCreateStrategy{store.CreateStrategy}
		store.DeleteStrategy = podStrategy{store.DeleteStrategy}
	}
	registry.Customizers["replicationcontrollers"] = func(store *registry.Store, _ registry.Deps) {
		store.DeleteStrategy = registry.OrphanByDefault{RESTDeleteStrategy: store.DeleteStrategy}
	}
	registry.Customizers["namespaces"] = func(store *registry.Store, _ registry.Deps) {
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
