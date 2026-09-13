package core

import (
	"context"
	"net"
	"net/http"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam/cidrset"
)

func init() {
	registry.Customizers["pods"] = func(store *registry.Store, _ registry.Deps) {
		store.DeleteStrategy = podStrategy{store.DeleteStrategy}
		store.BeginCreate = func(ctx context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			return pokeSchedulerFor(obj), nil
		}
		store.BeginUpdate = func(ctx context.Context, obj, _ runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			return pokeSchedulerFor(obj), nil
		}
	}
	registry.Customizers["nodes"] = func(store *registry.Store, deps registry.Deps) {
		store.BeginCreate = func(ctx context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			if err := assignPodCIDR(ctx, store.Storage.Storage, obj.(*corev1.Node), deps.ClusterCIDR); err != nil {
				return nil, err
			}
			return pokeScheduler, nil
		}
		store.BeginUpdate = func(context.Context, runtime.Object, runtime.Object, *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			return pokeScheduler, nil
		}
	}
	registry.Subresources["pods/log"] = func(stores map[string]*registry.Store, deps registry.Deps) rest.Storage {
		return NewLogREST(stores["pods"], stores["nodes"], deps.Kubelet)
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

func pokeSchedulerFor(obj runtime.Object) genericregistry.FinishFunc {
	if obj.(*corev1.Pod).Spec.NodeName != "" {
		return func(context.Context, bool) {}
	}
	return pokeScheduler
}

func pokeScheduler(ctx context.Context, success bool) {
	if success && registry.Poke != nil {
		registry.Poke(ctx)
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
func assignPodCIDR(ctx context.Context, s storage.Interface, node *corev1.Node, clusterCIDR *net.IPNet) error {
	if node.Spec.PodCIDR != "" {
		return nil
	}
	list := &corev1.NodeList{}
	if err := s.GetList(ctx, "/nodes", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return err
	}
	set, err := cidrset.NewCIDRSet(clusterCIDR, 24)
	if err != nil {
		return err
	}
	for _, n := range list.Items {
		if _, used, err := net.ParseCIDR(n.Spec.PodCIDR); err == nil {
			if err := set.Occupy(used); err != nil {
				return err
			}
		}
	}
	cidr, err := set.AllocateNext()
	if err != nil {
		return err
	}
	node.Spec.PodCIDR = cidr.String()
	node.Spec.PodCIDRs = []string{cidr.String()}
	return nil
}
