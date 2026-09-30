package core

import (
	"context"
	"net/http"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
)

var bindingPods = struct{ store *registry.Store }{}

func init() {
	registry.Resources["bindings"] = func(_ schema.GroupVersion, _ metav1.APIResource, _ registry.Deps) rest.Storage {
		return legacyBindingREST{}
	}
	registry.Resources["componentstatuses"] = func(_ schema.GroupVersion, _ metav1.APIResource, deps registry.Deps) rest.Storage {
		return newComponentStatusREST(deps.Kine)
	}
	registry.Customizers["pods"] = func(store *registry.Store, _ registry.Deps) {
		store.CreateStrategy = podCreateStrategy{store.CreateStrategy}
		store.UpdateStrategy = podUpdateStrategy{store.UpdateStrategy}
		store.DeleteStrategy = podStrategy{store.DeleteStrategy}
	}
	registry.Customizers["replicationcontrollers"] = func(store *registry.Store, _ registry.Deps) {
		store.DeleteStrategy = registry.OrphanByDefault{RESTDeleteStrategy: store.DeleteStrategy}
	}
	registry.Customizers["namespaces"] = func(store *registry.Store, deps registry.Deps) {
		store.CreateStrategy = namespaceCreateStrategy{store.CreateStrategy}
		store.UpdateStrategy = namespaceUpdateStrategy{store.UpdateStrategy}
		store.ShouldDeleteDuringUpdate = shouldDeleteNamespaceDuringUpdate
		store.BeginCreate = namespaceBeginCreate
		nsAccounts.kine = deps.Kine
	}
	registry.Deleters["namespaces"] = func(store *registry.Store) rest.GracefulDeleter { return namespaceDeleter{store} }
	registry.Subresources["namespaces/finalize"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return registry.NewUpdateOnlyREST(stores["namespaces"], namespaceFinalizeStrategy{stores["namespaces"].UpdateStrategy})
	}
	registry.Subresources["nodes/proxy"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewNodeProxyREST(stores["nodes"], deps.Kubelet)
	}
	registry.Subresources["pods/log"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewLogREST(stores["pods"], stores["nodes"], deps.Kubelet)
	}
	registry.Subresources["pods/exec"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewExecREST(stores["pods"], deps.Kubelet)
	}
	registry.Subresources["pods/attach"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewAttachREST(stores["pods"], deps.Kubelet)
	}
	registry.Subresources["pods/portforward"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewPortForwardREST(stores["pods"], deps.Kubelet)
	}
	registry.Subresources["pods/proxy"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewPodProxyREST(stores["pods"], deps.Kubelet)
	}
	registry.Subresources["services/proxy"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewServiceProxyREST(stores["services"], stores["endpoints"], stores["pods"], deps.Kubelet)
	}
	registry.Subresources["pods/status"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return registry.NewUpdateOnlyREST(stores["pods"], podStatusStrategy{registry.StatusStrategyFor(stores["pods"])})
	}
	registry.Subresources["pods/binding"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return bindingREST{stores["pods"]}
	}
	registry.Subresources["pods/eviction"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return newEvictionREST(stores["pods"], deps.Kine)
	}
	registry.Subresources["pods/ephemeralcontainers"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return registry.NewUpdateOnlyREST(stores["pods"], ephemeralContainersStrategy{stores["pods"].UpdateStrategy})
	}
	registry.Subresources["pods/resize"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return registry.NewUpdateOnlyREST(stores["pods"], resizeStrategy{stores["pods"].UpdateStrategy})
	}
	registry.Subresources["replicationcontrollers/scale"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
		return registry.NewScaleREST(stores["replicationcontrollers"])
	}
	registry.Middleware = append(registry.Middleware, func(stores map[string]*registry.Store) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			bindingPods.store = stores["pods"]
			bindNamespaceAccounts(stores, nsAccounts.kine)
			bindKubernetesEndpoints(stores)
			return bootstrapCluster(stores["namespaces"], stores["services"], proxyRootRedirect(next))
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
