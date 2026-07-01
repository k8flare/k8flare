package apiserver

import (
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NewNamespaceStore creates a ResourceStore for Namespace resources (cluster-scoped).
func NewNamespaceStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "namespaces", false,
		func() runtime.Object { return &corev1.Namespace{} },
		func() runtime.Object {
			return &corev1.NamespaceList{TypeMeta: metav1.TypeMeta{Kind: "NamespaceList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			nsList := list.(*corev1.NamespaceList)
			for _, item := range items {
				nsList.Items = append(nsList.Items, *item.(*corev1.Namespace))
			}
		},
	)
}

// NewConfigMapStore creates a ResourceStore for ConfigMap resources (namespaced).
func NewConfigMapStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "configmaps", true,
		func() runtime.Object { return &corev1.ConfigMap{} },
		func() runtime.Object {
			return &corev1.ConfigMapList{TypeMeta: metav1.TypeMeta{Kind: "ConfigMapList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			cmList := list.(*corev1.ConfigMapList)
			for _, item := range items {
				cmList.Items = append(cmList.Items, *item.(*corev1.ConfigMap))
			}
		},
	)
}

// NewSecretStore creates a ResourceStore for Secret resources (namespaced).
func NewSecretStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "secrets", true,
		func() runtime.Object { return &corev1.Secret{} },
		func() runtime.Object {
			return &corev1.SecretList{TypeMeta: metav1.TypeMeta{Kind: "SecretList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			secretList := list.(*corev1.SecretList)
			for _, item := range items {
				secretList.Items = append(secretList.Items, *item.(*corev1.Secret))
			}
		},
	)
}

// NewPodStore creates a ResourceStore for Pod resources (namespaced).
func NewPodStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "pods", true,
		func() runtime.Object { return &corev1.Pod{} },
		func() runtime.Object {
			return &corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			podList := list.(*corev1.PodList)
			for _, item := range items {
				podList.Items = append(podList.Items, *item.(*corev1.Pod))
			}
		},
	)
}

// NewNodeStore creates a ResourceStore for Node resources (cluster-scoped).
func NewNodeStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "nodes", false,
		func() runtime.Object { return &corev1.Node{} },
		func() runtime.Object {
			return &corev1.NodeList{TypeMeta: metav1.TypeMeta{Kind: "NodeList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			nodeList := list.(*corev1.NodeList)
			for _, item := range items {
				nodeList.Items = append(nodeList.Items, *item.(*corev1.Node))
			}
		},
	)
}

// NewServiceAccountStore creates a ResourceStore for ServiceAccount resources (namespaced).
func NewServiceAccountStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "serviceaccounts", true,
		func() runtime.Object { return &corev1.ServiceAccount{} },
		func() runtime.Object {
			return &corev1.ServiceAccountList{TypeMeta: metav1.TypeMeta{Kind: "ServiceAccountList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			saList := list.(*corev1.ServiceAccountList)
			for _, item := range items {
				saList.Items = append(saList.Items, *item.(*corev1.ServiceAccount))
			}
		},
	)
}

// NewEndpointsStore creates a ResourceStore for Endpoints resources (namespaced).
func NewEndpointsStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "endpoints", true,
		func() runtime.Object { return &corev1.Endpoints{} },
		func() runtime.Object {
			return &corev1.EndpointsList{TypeMeta: metav1.TypeMeta{Kind: "EndpointsList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			epList := list.(*corev1.EndpointsList)
			for _, item := range items {
				epList.Items = append(epList.Items, *item.(*corev1.Endpoints))
			}
		},
	)
}

// NewServiceStore creates a ResourceStore for Service resources (namespaced).
func NewServiceStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "services", true,
		func() runtime.Object { return &corev1.Service{} },
		func() runtime.Object {
			return &corev1.ServiceList{TypeMeta: metav1.TypeMeta{Kind: "ServiceList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			svcList := list.(*corev1.ServiceList)
			for _, item := range items {
				svcList.Items = append(svcList.Items, *item.(*corev1.Service))
			}
		},
	)
}

// NewLeaseStore creates a ResourceStore for Lease resources (namespaced).
func NewLeaseStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "leases", true,
		func() runtime.Object { return &coordinationv1.Lease{} },
		func() runtime.Object {
			return &coordinationv1.LeaseList{TypeMeta: metav1.TypeMeta{Kind: "LeaseList", APIVersion: "coordination.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			leaseList := list.(*coordinationv1.LeaseList)
			for _, item := range items {
				leaseList.Items = append(leaseList.Items, *item.(*coordinationv1.Lease))
			}
		},
	)
}

// NewEventStore creates a ResourceStore for Event resources (namespaced).
func NewEventStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "events", true,
		func() runtime.Object { return &corev1.Event{} },
		func() runtime.Object {
			return &corev1.EventList{
				TypeMeta: metav1.TypeMeta{Kind: "EventList", APIVersion: "v1"},
			}
		},
		func(list runtime.Object, items []runtime.Object) {
			eventList := list.(*corev1.EventList)
			for _, item := range items {
				eventList.Items = append(eventList.Items, *(item.(*corev1.Event)))
			}
		},
	)
}

// NewResourceStores creates all supported core/v1 ResourceStore instances and returns them
// as a map keyed by resource name.
func NewResourceStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"namespaces":      NewNamespaceStore(s),
		"configmaps":      NewConfigMapStore(s),
		"secrets":         NewSecretStore(s),
		"pods":            NewPodStore(s),
		"nodes":           NewNodeStore(s),
		"serviceaccounts": NewServiceAccountStore(s),
		"endpoints":       NewEndpointsStore(s),
		"services":        NewServiceStore(s),
		"events":          NewEventStore(s),
	}
}

// NewLeaseStores creates ResourceStore instances for coordination.k8s.io/v1 resources
// and returns them as a map keyed by resource name.
func NewLeaseStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"leases": NewLeaseStore(s),
	}
}

// NewEventStores creates ResourceStore instances for events.k8s.io/v1 resources
// and returns them as a map keyed by resource name.
func NewEventStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"events": NewEventStore(s),
	}
}

// NewCSIDriverStore creates a ResourceStore for CSIDriver resources (cluster-scoped).
func NewCSIDriverStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "csidrivers", false,
		func() runtime.Object { return &storagev1.CSIDriver{} },
		func() runtime.Object {
			return &storagev1.CSIDriverList{TypeMeta: metav1.TypeMeta{Kind: "CSIDriverList", APIVersion: "storage.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			csiList := list.(*storagev1.CSIDriverList)
			for _, item := range items {
				csiList.Items = append(csiList.Items, *item.(*storagev1.CSIDriver))
			}
		},
	)
}

// NewCSINodeStore creates a ResourceStore for CSINode resources (cluster-scoped).
func NewCSINodeStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "csinodes", false,
		func() runtime.Object { return &storagev1.CSINode{} },
		func() runtime.Object {
			return &storagev1.CSINodeList{TypeMeta: metav1.TypeMeta{Kind: "CSINodeList", APIVersion: "storage.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			csiNodeList := list.(*storagev1.CSINodeList)
			for _, item := range items {
				csiNodeList.Items = append(csiNodeList.Items, *item.(*storagev1.CSINode))
			}
		},
	)
}

// NewRuntimeClassStore creates a ResourceStore for RuntimeClass resources (cluster-scoped).
func NewRuntimeClassStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "runtimeclasses", false,
		func() runtime.Object { return &nodev1.RuntimeClass{} },
		func() runtime.Object {
			return &nodev1.RuntimeClassList{TypeMeta: metav1.TypeMeta{Kind: "RuntimeClassList", APIVersion: "node.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			rcList := list.(*nodev1.RuntimeClassList)
			for _, item := range items {
				rcList.Items = append(rcList.Items, *item.(*nodev1.RuntimeClass))
			}
		},
	)
}

// NewStorageStores creates ResourceStore instances for storage.k8s.io/v1 resources
// and returns them as a map keyed by resource name.
func NewStorageStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"csidrivers": NewCSIDriverStore(s),
		"csinodes":   NewCSINodeStore(s),
	}
}

// NewNodeAPIStores creates ResourceStore instances for node.k8s.io/v1 resources
// and returns them as a map keyed by resource name.
func NewNodeAPIStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"runtimeclasses": NewRuntimeClassStore(s),
	}
}

// NamespacedResourceStores collects every ResourceStore that is namespaced
// across however many store maps are passed in, de-duplicating by resource
// name (e.g. "events" may appear in more than one map pointing at the same
// underlying storage).
//
// Derived from the existing registration maps rather than hand-listed, so a
// future namespaced resource type added to any of
// NewResourceStores/NewLeaseStores/etc. is automatically swept on namespace
// deletion without a second place to remember to update.
func NamespacedResourceStores(storeMaps ...map[string]*ResourceStore) []*ResourceStore {
	seen := make(map[string]bool)
	var out []*ResourceStore
	for _, m := range storeMaps {
		for _, rs := range m {
			if !rs.namespaced || seen[rs.resource] {
				continue
			}
			seen[rs.resource] = true
			out = append(out, rs)
		}
	}
	return out
}
