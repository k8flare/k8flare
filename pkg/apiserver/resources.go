package apiserver

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	policyv1 "k8s.io/api/policy/v1"
	resourcev1 "k8s.io/api/resource/v1"
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

// NewResourceClaimStore creates a ResourceStore for ResourceClaim resources (namespaced).
// Never populated with real data — see the comment on resourcev1 registration
// in scheme.go for why this exists.
func NewResourceClaimStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "resourceclaims", true,
		func() runtime.Object { return &resourcev1.ResourceClaim{} },
		func() runtime.Object {
			return &resourcev1.ResourceClaimList{TypeMeta: metav1.TypeMeta{Kind: "ResourceClaimList", APIVersion: "resource.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			rcList := list.(*resourcev1.ResourceClaimList)
			for _, item := range items {
				rcList.Items = append(rcList.Items, *item.(*resourcev1.ResourceClaim))
			}
		},
	)
}

// NewResourceSliceStore creates a ResourceStore for ResourceSlice resources (cluster-scoped).
// Never populated with real data — see the comment on resourcev1 registration
// in scheme.go for why this exists.
func NewResourceSliceStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "resourceslices", false,
		func() runtime.Object { return &resourcev1.ResourceSlice{} },
		func() runtime.Object {
			return &resourcev1.ResourceSliceList{TypeMeta: metav1.TypeMeta{Kind: "ResourceSliceList", APIVersion: "resource.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			rsList := list.(*resourcev1.ResourceSliceList)
			for _, item := range items {
				rsList.Items = append(rsList.Items, *item.(*resourcev1.ResourceSlice))
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

// NewLimitRangeStore creates a ResourceStore for LimitRange resources (namespaced).
func NewLimitRangeStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "limitranges", true,
		func() runtime.Object { return &corev1.LimitRange{} },
		func() runtime.Object {
			return &corev1.LimitRangeList{TypeMeta: metav1.TypeMeta{Kind: "LimitRangeList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			lrList := list.(*corev1.LimitRangeList)
			for _, item := range items {
				lrList.Items = append(lrList.Items, *item.(*corev1.LimitRange))
			}
		},
	)
}

// NewReplicationControllerStore creates a ResourceStore for ReplicationController
// resources (namespaced). Never populated with real data — see the comment on
// the apps/v1 and policy/v1 scheme registration in scheme.go for why this exists.
func NewReplicationControllerStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "replicationcontrollers", true,
		func() runtime.Object { return &corev1.ReplicationController{} },
		func() runtime.Object {
			return &corev1.ReplicationControllerList{TypeMeta: metav1.TypeMeta{Kind: "ReplicationControllerList", APIVersion: "v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			rcList := list.(*corev1.ReplicationControllerList)
			for _, item := range items {
				rcList.Items = append(rcList.Items, *item.(*corev1.ReplicationController))
			}
		},
	)
}

// NewResourceStores creates all supported core/v1 ResourceStore instances and returns them
// as a map keyed by resource name.
func NewResourceStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"namespaces":             NewNamespaceStore(s),
		"configmaps":             NewConfigMapStore(s),
		"secrets":                NewSecretStore(s),
		"pods":                   NewPodStore(s),
		"nodes":                  NewNodeStore(s),
		"serviceaccounts":        NewServiceAccountStore(s),
		"endpoints":              NewEndpointsStore(s),
		"services":               NewServiceStore(s),
		"events":                 NewEventStore(s),
		"limitranges":            NewLimitRangeStore(s),
		"replicationcontrollers": NewReplicationControllerStore(s),
	}
}

// NewLeaseStores creates ResourceStore instances for coordination.k8s.io/v1 resources
// and returns them as a map keyed by resource name.
func NewLeaseStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"leases": NewLeaseStore(s),
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

// NewDeviceClassStore creates a ResourceStore for DeviceClass resources (cluster-scoped).
// Never populated with real data — see the comment on resourcev1 registration
// in scheme.go for why this exists.
func NewDeviceClassStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "deviceclasses", false,
		func() runtime.Object { return &resourcev1.DeviceClass{} },
		func() runtime.Object {
			return &resourcev1.DeviceClassList{TypeMeta: metav1.TypeMeta{Kind: "DeviceClassList", APIVersion: "resource.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			dcList := list.(*resourcev1.DeviceClassList)
			for _, item := range items {
				dcList.Items = append(dcList.Items, *item.(*resourcev1.DeviceClass))
			}
		},
	)
}

// NewResourceAPIStores creates ResourceStore instances for resource.k8s.io/v1
// resources and returns them as a map keyed by resource name.
func NewResourceAPIStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"resourceclaims": NewResourceClaimStore(s),
		"resourceslices": NewResourceSliceStore(s),
		"deviceclasses":  NewDeviceClassStore(s),
	}
}

// NewReplicaSetStore creates a ResourceStore for ReplicaSet resources
// (namespaced). Real, controller-backed resource — see
// packages/etcd/src/replicaset.ts.
func NewReplicaSetStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "replicasets", true,
		func() runtime.Object { return &appsv1.ReplicaSet{} },
		func() runtime.Object {
			return &appsv1.ReplicaSetList{TypeMeta: metav1.TypeMeta{Kind: "ReplicaSetList", APIVersion: "apps/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			rsList := list.(*appsv1.ReplicaSetList)
			for _, item := range items {
				rsList.Items = append(rsList.Items, *item.(*appsv1.ReplicaSet))
			}
		},
	)
}

// NewDeploymentStore creates a ResourceStore for Deployment resources
// (namespaced). Real, controller-backed resource — see
// packages/etcd/src/deployment.ts.
func NewDeploymentStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "deployments", true,
		func() runtime.Object { return &appsv1.Deployment{} },
		func() runtime.Object {
			return &appsv1.DeploymentList{TypeMeta: metav1.TypeMeta{Kind: "DeploymentList", APIVersion: "apps/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			dList := list.(*appsv1.DeploymentList)
			for _, item := range items {
				dList.Items = append(dList.Items, *item.(*appsv1.Deployment))
			}
		},
	)
}

// NewStatefulSetStore creates a ResourceStore for StatefulSet resources (namespaced).
// Never populated with real data — see the comment on the apps/v1 and
// policy/v1 scheme registration in scheme.go for why this exists.
func NewStatefulSetStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "statefulsets", true,
		func() runtime.Object { return &appsv1.StatefulSet{} },
		func() runtime.Object {
			return &appsv1.StatefulSetList{TypeMeta: metav1.TypeMeta{Kind: "StatefulSetList", APIVersion: "apps/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			ssList := list.(*appsv1.StatefulSetList)
			for _, item := range items {
				ssList.Items = append(ssList.Items, *item.(*appsv1.StatefulSet))
			}
		},
	)
}

// NewAppsStores creates ResourceStore instances for apps/v1 resources and
// returns them as a map keyed by resource name.
func NewAppsStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"replicasets":         NewReplicaSetStore(s),
		"deployments":         NewDeploymentStore(s),
		"daemonsets":          NewDaemonSetStore(s),
		"statefulsets":        NewStatefulSetStore(s),
		"controllerrevisions": NewControllerRevisionStore(s),
	}
}

// NewControllerRevisionStore creates a ResourceStore for ControllerRevision
// resources (namespaced). Never populated with real data — see the comment
// on the apps/v1 scheme registration in scheme.go for why this exists.
func NewControllerRevisionStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "controllerrevisions", true,
		func() runtime.Object { return &appsv1.ControllerRevision{} },
		func() runtime.Object {
			return &appsv1.ControllerRevisionList{TypeMeta: metav1.TypeMeta{Kind: "ControllerRevisionList", APIVersion: "apps/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			crList := list.(*appsv1.ControllerRevisionList)
			for _, item := range items {
				crList.Items = append(crList.Items, *item.(*appsv1.ControllerRevision))
			}
		},
	)
}

// NewDaemonSetStore creates a ResourceStore for DaemonSet resources
// (namespaced). Real, controller-backed resource — see
// packages/etcd/src/daemonset.ts.
func NewDaemonSetStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "daemonsets", true,
		func() runtime.Object { return &appsv1.DaemonSet{} },
		func() runtime.Object {
			return &appsv1.DaemonSetList{TypeMeta: metav1.TypeMeta{Kind: "DaemonSetList", APIVersion: "apps/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			dsList := list.(*appsv1.DaemonSetList)
			for _, item := range items {
				dsList.Items = append(dsList.Items, *item.(*appsv1.DaemonSet))
			}
		},
	)
}

// NewPodDisruptionBudgetStore creates a ResourceStore for PodDisruptionBudget
// resources (namespaced). Never populated with real data — see the comment
// on the apps/v1 and policy/v1 scheme registration in scheme.go for why this
// exists.
func NewPodDisruptionBudgetStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "poddisruptionbudgets", true,
		func() runtime.Object { return &policyv1.PodDisruptionBudget{} },
		func() runtime.Object {
			return &policyv1.PodDisruptionBudgetList{TypeMeta: metav1.TypeMeta{Kind: "PodDisruptionBudgetList", APIVersion: "policy/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			pdbList := list.(*policyv1.PodDisruptionBudgetList)
			for _, item := range items {
				pdbList.Items = append(pdbList.Items, *item.(*policyv1.PodDisruptionBudget))
			}
		},
	)
}

// NewPolicyStores creates ResourceStore instances for policy/v1 resources and
// returns them as a map keyed by resource name.
func NewPolicyStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"poddisruptionbudgets": NewPodDisruptionBudgetStore(s),
	}
}

// NewEndpointSliceStore creates a ResourceStore for EndpointSlice resources
// (namespaced). Populated by the Endpoints/EndpointSlice controller
// (packages/etcd/src/endpoints.ts), which reconciles Service selectors
// against ready Pods -- unlike the stub types above, this one is real.
func NewEndpointSliceStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "endpointslices", true,
		func() runtime.Object { return &discoveryv1.EndpointSlice{} },
		func() runtime.Object {
			return &discoveryv1.EndpointSliceList{TypeMeta: metav1.TypeMeta{Kind: "EndpointSliceList", APIVersion: "discovery.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			epsList := list.(*discoveryv1.EndpointSliceList)
			for _, item := range items {
				epsList.Items = append(epsList.Items, *item.(*discoveryv1.EndpointSlice))
			}
		},
	)
}

// NewDiscoveryStores creates ResourceStore instances for discovery.k8s.io/v1
// resources and returns them as a map keyed by resource name.
func NewDiscoveryStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"endpointslices": NewEndpointSliceStore(s),
	}
}

// NewServiceCIDRStore creates a ResourceStore for ServiceCIDR resources
// (cluster-scoped). Never populated with real data -- see the comment on
// the networking.k8s.io/v1 scheme registration in scheme.go for why this
// exists.
func NewServiceCIDRStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "servicecidrs", false,
		func() runtime.Object { return &networkingv1.ServiceCIDR{} },
		func() runtime.Object {
			return &networkingv1.ServiceCIDRList{TypeMeta: metav1.TypeMeta{Kind: "ServiceCIDRList", APIVersion: "networking.k8s.io/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			cidrList := list.(*networkingv1.ServiceCIDRList)
			for _, item := range items {
				cidrList.Items = append(cidrList.Items, *item.(*networkingv1.ServiceCIDR))
			}
		},
	)
}

// NewNetworkingStores creates ResourceStore instances for networking.k8s.io/v1
// resources and returns them as a map keyed by resource name.
func NewNetworkingStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"servicecidrs": NewServiceCIDRStore(s),
	}
}

// NewJobStore creates a ResourceStore for Job resources (namespaced). Real,
// controller-backed resource -- see packages/etcd/src/job.ts.
func NewJobStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "jobs", true,
		func() runtime.Object { return &batchv1.Job{} },
		func() runtime.Object {
			return &batchv1.JobList{TypeMeta: metav1.TypeMeta{Kind: "JobList", APIVersion: "batch/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			jList := list.(*batchv1.JobList)
			for _, item := range items {
				jList.Items = append(jList.Items, *item.(*batchv1.Job))
			}
		},
	)
}

// NewCronJobStore creates a ResourceStore for CronJob resources (namespaced).
// Real, controller-backed resource -- see packages/etcd/src/cronjob.ts.
func NewCronJobStore(s *Storage) *ResourceStore {
	return NewResourceStore(s, "cronjobs", true,
		func() runtime.Object { return &batchv1.CronJob{} },
		func() runtime.Object {
			return &batchv1.CronJobList{TypeMeta: metav1.TypeMeta{Kind: "CronJobList", APIVersion: "batch/v1"}}
		},
		func(list runtime.Object, items []runtime.Object) {
			cjList := list.(*batchv1.CronJobList)
			for _, item := range items {
				cjList.Items = append(cjList.Items, *item.(*batchv1.CronJob))
			}
		},
	)
}

// NewBatchStores creates ResourceStore instances for batch/v1 resources and
// returns them as a map keyed by resource name.
func NewBatchStores(s *Storage) map[string]*ResourceStore {
	return map[string]*ResourceStore{
		"jobs":     NewJobStore(s),
		"cronjobs": NewCronJobStore(s),
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
