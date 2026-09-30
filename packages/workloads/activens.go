package workloads

import (
	v1 "k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type activeNamespaces struct{ coreinformers.NamespaceInformer }

func (a activeNamespaces) Informer() cache.SharedIndexInformer {
	return activeNamespaceEvents{a.NamespaceInformer.Informer()}
}

type activeNamespaceEvents struct{ cache.SharedIndexInformer }

func (e activeNamespaceEvents) AddEventHandler(h cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return e.SharedIndexInformer.AddEventHandler(cache.FilteringResourceEventHandler{FilterFunc: namespaceActive, Handler: h})
}

func namespaceActive(obj interface{}) bool {
	ns, ok := obj.(*v1.Namespace)
	return ok && ns.DeletionTimestamp == nil && ns.Status.Phase != v1.NamespaceTerminating
}

func (d Deps) ActiveNamespaces() coreinformers.NamespaceInformer {
	return activeNamespaces{d.Factory.Core().V1().Namespaces()}
}
