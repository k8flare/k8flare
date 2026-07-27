// Package apidef is the single hand-written source of truth for every API
// resource this apiserver serves: its GroupVersion, Kind, plural resource
// name, scope, verbs, subresources, and (for types that exist only so a real
// controller's informer can complete WaitForCacheSync against an empty list)
// why it's never populated with real data.
//
// Everything else -- pkg/apiserver's Scheme registration, ResourceStore
// construction, HTTP discovery documents, and cmd/k8flare-gen's generated
// artifacts -- is derived from Table at either compile time (this package is
// imported directly) or generation time (cmd/k8flare-gen imports it too).
// Adding a resource here is the only step needed to register it everywhere;
// before this package existed, the same facts were hand-duplicated across
// resources.go (store construction), scheme.go (Scheme registration), and
// discovery.go (HTTP discovery documents), and could -- and did -- drift out
// of sync (see docs/general-purpose-k8s-plan.md's k8flare-gen runbook for an
// example: apps/v1 and batch/v1's /status subresources were implemented in
// pkg/apiserver/subresource.go but never advertised in discovery).
//
// New/NewList are real closures over the concrete k8s.io/api types (e.g.
// `func() runtime.Object { return &corev1.Pod{} }`), not strings or
// reflection-by-name, so a type that's renamed or removed upstream fails
// `go build` here instead of failing silently at runtime.
package apidef

import (
	k8flarev1alpha1 "github.com/k8flare/k8flare/pkg/apis/k8flare/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Subresource describes one subresource of a ResourceDef (e.g. "status",
// "binding"), for discovery purposes. Request handling for anything beyond
// the generic "status" pattern (get whole object / copy .Status field on
// PUT / patch) is still resource-specific Go code -- see
// pkg/apiserver/subresource.go -- since subresources like pods/binding have
// no generic cross-type shape to derive from a table.
type Subresource struct {
	// Name is the subresource path segment, e.g. "status" for pods/status.
	Name string
	// Verbs are the HTTP-method-derived verbs this subresource accepts.
	Verbs []string
	// Kind is the subresource's own Kind if it differs from the parent
	// ResourceDef's Kind (e.g. "Binding" for pods/binding). Empty means
	// the subresource shares the parent's Kind (e.g. pods/status is still
	// Kind "Pod").
	Kind string
}

// ResourceDef describes one API resource (GroupVersionResource) served by
// this apiserver.
type ResourceDef struct {
	// GroupVersion identifies the API group+version, e.g.
	// corev1.SchemeGroupVersion or appsv1.SchemeGroupVersion.
	GroupVersion schema.GroupVersion
	// Kind is the resource's Kind, e.g. "Pod".
	Kind string
	// Resource is the plural, lowercase resource name used in URLs and
	// storage keys, e.g. "pods".
	Resource string
	// Singular is the singular form used in discovery documents, e.g.
	// "pod". Empty for resources upstream also leaves unset.
	Singular string
	// ShortNames are kubectl shorthand aliases, e.g. []string{"po"} for pods.
	ShortNames []string
	// Namespaced is true for namespace-scoped resources, false for
	// cluster-scoped ones.
	Namespaced bool
	// Verbs are the HTTP-method-derived verbs this resource's top-level
	// endpoint accepts. Nil means StandardVerbs (see below) -- every
	// resource in this table is served by the same generic CRUD+watch
	// handler (pkg/apiserver/handler.go's HandleResource), so unless a
	// future resource needs a narrower surface there is nothing to
	// differentiate.
	Verbs []string
	// Subresources lists this resource's subresources, if any.
	Subresources []Subresource

	// StubReason, if non-empty, documents why this resource is registered
	// but deliberately never populated with real data (e.g. so a real
	// controller/scheduler informer can complete WaitForCacheSync against
	// an empty list instead of hanging forever). Empty means the type is
	// real and controller- or apiserver-backed.
	StubReason string

	// New returns an empty object of this resource's type, e.g.
	// &corev1.Pod{}.
	New func() runtime.Object
	// NewList returns an empty list object of this resource's type, e.g.
	// &corev1.PodList{}. Its TypeMeta is pre-populated (Kind+APIVersion)
	// since list objects must self-describe even before any items are
	// added -- ResourceStore.List (pkg/apiserver/store.go) never sets it.
	NewList func() runtime.Object
}

// StandardVerbs is the verb set assumed for any ResourceDef that leaves
// Verbs nil. pkg/apiserver/handler.go's HandleResource implements GET
// (get/list), POST (create), PUT (update), DELETE (delete when a name is
// given, deletecollection when it isn't), PATCH, and watch identically for
// every resource routed to it -- there is no per-resource gating in the
// handler. Before this table existed, discovery.go hand-listed a narrower,
// inconsistent verb set per resource (most core/v1 types were missing
// "patch"; only limitranges advertised "deletecollection", though every
// resource supports it) that had drifted from what the handler actually
// does. This is the corrected, uniform set; see CLAUDE.md rule 4 on
// recording corrections.
var StandardVerbs = []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}

// standardStatusSubresourceVerbs is the verb set for a "status" subresource:
// pkg/apiserver/subresource.go's generic status handler supports GET (return
// the whole object), PUT (replace just .Status), and PATCH (patch the whole
// object, but only .Status is expected to differ) -- no create/delete/list.
var standardStatusSubresourceVerbs = []string{"get", "patch", "update"}

// statusSubresource returns the standard "status" Subresource entry shared
// by every resource below that has one.
func statusSubresource() Subresource {
	return Subresource{Name: "status", Verbs: standardStatusSubresourceVerbs}
}

// standardScaleSubresourceVerbs is the verb set for a "scale" subresource:
// pkg/apiserver/subresource.go's generic scale handler supports GET (read
// replica count/selector as an autoscaling/v1.Scale), PUT (replace replica
// count), and PATCH -- no create/delete/list, matching upstream's
// pkg/registry/*/*/storage.go ScaleREST for these types.
var standardScaleSubresourceVerbs = []string{"get", "patch", "update"}

// scaleSubresource returns the standard "scale" Subresource entry shared by
// every resource below that has one. Its Kind is always "Scale"
// (autoscaling/v1.Scale), not the parent resource's Kind.
func scaleSubresource() Subresource {
	return Subresource{Name: "scale", Verbs: standardScaleSubresourceVerbs, Kind: "Scale"}
}

// Table is the full list of API resources this apiserver serves, grouped by
// GroupVersion in the same order main.go registers them.
var Table = []ResourceDef{
	// ---- core/v1 (the legacy "/api/v1" group) ----
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "Namespace", Resource: "namespaces",
		Singular: "namespace", ShortNames: []string{"ns"}, Namespaced: false,
		New: func() runtime.Object { return &corev1.Namespace{} },
		NewList: func() runtime.Object {
			return &corev1.NamespaceList{TypeMeta: metav1.TypeMeta{Kind: "NamespaceList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "ConfigMap", Resource: "configmaps",
		Singular: "configmap", ShortNames: []string{"cm"}, Namespaced: true,
		New: func() runtime.Object { return &corev1.ConfigMap{} },
		NewList: func() runtime.Object {
			return &corev1.ConfigMapList{TypeMeta: metav1.TypeMeta{Kind: "ConfigMapList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "Secret", Resource: "secrets",
		Singular: "secret", Namespaced: true,
		New: func() runtime.Object { return &corev1.Secret{} },
		NewList: func() runtime.Object {
			return &corev1.SecretList{TypeMeta: metav1.TypeMeta{Kind: "SecretList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "Pod", Resource: "pods",
		Singular: "pod", ShortNames: []string{"po"}, Namespaced: true,
		Subresources: []Subresource{
			statusSubresource(),
			{Name: "binding", Verbs: []string{"create"}, Kind: "Binding"},
			// log/exec/attach are handled entirely by the JS gateway layer
			// (kubelet proxy over VPC, WebSocket bridging) -- the Go
			// handler only returns 501 as a fallback for direct/test
			// callers that bypass it. See pkg/apiserver/subresource.go.
			{Name: "log", Verbs: []string{"get"}},
			{Name: "exec", Verbs: []string{"create", "get"}},
			{Name: "attach", Verbs: []string{"create", "get"}},
		},
		New: func() runtime.Object { return &corev1.Pod{} },
		NewList: func() runtime.Object {
			return &corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "Node", Resource: "nodes",
		Singular: "node", ShortNames: []string{"no"}, Namespaced: false,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &corev1.Node{} },
		NewList: func() runtime.Object {
			return &corev1.NodeList{TypeMeta: metav1.TypeMeta{Kind: "NodeList", APIVersion: "v1"}}
		},
	},
	{
		// Cluster-scoped, like Node -- upstream PersistentVolume is never
		// namespaced even though the claims that bind to it are. Generic
		// CRUD only: this apiserver has no dynamic provisioner (the
		// R2-backed one was removed 2026-07-08, see git history, pending
		// a real CSI-driver-based replacement), so a PersistentVolumeClaim
		// stays Pending exactly like a real cluster with no matching
		// StorageClass/provisioner.
		GroupVersion: corev1.SchemeGroupVersion, Kind: "PersistentVolume", Resource: "persistentvolumes",
		Singular: "persistentvolume", ShortNames: []string{"pv"}, Namespaced: false,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &corev1.PersistentVolume{} },
		NewList: func() runtime.Object {
			return &corev1.PersistentVolumeList{TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeList", APIVersion: "v1"}}
		},
	},
	{
		// Namespaced -- see the PersistentVolume entry above.
		GroupVersion: corev1.SchemeGroupVersion, Kind: "PersistentVolumeClaim", Resource: "persistentvolumeclaims",
		Singular: "persistentvolumeclaim", ShortNames: []string{"pvc"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &corev1.PersistentVolumeClaim{} },
		NewList: func() runtime.Object {
			return &corev1.PersistentVolumeClaimList{TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaimList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "ServiceAccount", Resource: "serviceaccounts",
		Singular: "serviceaccount", ShortNames: []string{"sa"}, Namespaced: true,
		New: func() runtime.Object { return &corev1.ServiceAccount{} },
		NewList: func() runtime.Object {
			return &corev1.ServiceAccountList{TypeMeta: metav1.TypeMeta{Kind: "ServiceAccountList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "Endpoints", Resource: "endpoints",
		Singular: "endpoint", ShortNames: []string{"ep"}, Namespaced: true,
		New: func() runtime.Object { return &corev1.Endpoints{} },
		NewList: func() runtime.Object {
			return &corev1.EndpointsList{TypeMeta: metav1.TypeMeta{Kind: "EndpointsList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "Service", Resource: "services",
		Singular: "service", ShortNames: []string{"svc"}, Namespaced: true,
		New: func() runtime.Object { return &corev1.Service{} },
		NewList: func() runtime.Object {
			return &corev1.ServiceList{TypeMeta: metav1.TypeMeta{Kind: "ServiceList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "Event", Resource: "events",
		Singular: "event", ShortNames: []string{"ev"}, Namespaced: true,
		New: func() runtime.Object { return &corev1.Event{} },
		NewList: func() runtime.Object {
			return &corev1.EventList{TypeMeta: metav1.TypeMeta{Kind: "EventList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "LimitRange", Resource: "limitranges",
		Singular: "limitrange", ShortNames: []string{"limits"}, Namespaced: true,
		New: func() runtime.Object { return &corev1.LimitRange{} },
		NewList: func() runtime.Object {
			return &corev1.LimitRangeList{TypeMeta: metav1.TypeMeta{Kind: "LimitRangeList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "ResourceQuota", Resource: "resourcequotas",
		Singular: "resourcequota", ShortNames: []string{"quota"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &corev1.ResourceQuota{} },
		NewList: func() runtime.Object {
			return &corev1.ResourceQuotaList{TypeMeta: metav1.TypeMeta{Kind: "ResourceQuotaList", APIVersion: "v1"}}
		},
	},
	{
		GroupVersion: corev1.SchemeGroupVersion, Kind: "ReplicationController", Resource: "replicationcontrollers",
		Singular: "replicationcontroller", ShortNames: []string{"rc"}, Namespaced: true,
		StubReason:   "Never populated with real data. The real kube-scheduler's InterPodAffinity/PodTopologySpread plugins do owning-controller lookups against ReplicationController unconditionally, and DefaultPreemption checks PodDisruptionBudgets unconditionally, the same way the DRA plugin does for ResourceClaim/ResourceSlice/DeviceClass below -- registered only so those informers complete WaitForCacheSync against an empty list instead of hanging forever.",
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &corev1.ReplicationController{} },
		NewList: func() runtime.Object {
			return &corev1.ReplicationControllerList{TypeMeta: metav1.TypeMeta{Kind: "ReplicationControllerList", APIVersion: "v1"}}
		},
	},

	// ---- coordination.k8s.io/v1 ----
	{
		GroupVersion: coordinationv1.SchemeGroupVersion, Kind: "Lease", Resource: "leases",
		Singular: "lease", Namespaced: true,
		New: func() runtime.Object { return &coordinationv1.Lease{} },
		NewList: func() runtime.Object {
			return &coordinationv1.LeaseList{TypeMeta: metav1.TypeMeta{Kind: "LeaseList", APIVersion: "coordination.k8s.io/v1"}}
		},
	},

	// ---- storage.k8s.io/v1 ----
	{
		// Generic CRUD only -- no StorageClass is bootstrapped or marked
		// default (the R2-backed dynamic provisioner that used to do so
		// was removed 2026-07-08, see git history, pending a real
		// CSI-driver-based replacement).
		GroupVersion: storagev1.SchemeGroupVersion, Kind: "StorageClass", Resource: "storageclasses",
		Singular: "storageclass", ShortNames: []string{"sc"}, Namespaced: false,
		New: func() runtime.Object { return &storagev1.StorageClass{} },
		NewList: func() runtime.Object {
			return &storagev1.StorageClassList{TypeMeta: metav1.TypeMeta{Kind: "StorageClassList", APIVersion: "storage.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: storagev1.SchemeGroupVersion, Kind: "CSIDriver", Resource: "csidrivers",
		Singular: "csidriver", Namespaced: false,
		StubReason: "Never populated with real data; registered so a real scheduler/controller informer for this type can complete WaitForCacheSync against an empty list rather than hang -- same pattern as ResourceClaim/ResourceSlice/DeviceClass below (docs/control-plane-architecture.md).",
		New:        func() runtime.Object { return &storagev1.CSIDriver{} },
		NewList: func() runtime.Object {
			return &storagev1.CSIDriverList{TypeMeta: metav1.TypeMeta{Kind: "CSIDriverList", APIVersion: "storage.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: storagev1.SchemeGroupVersion, Kind: "CSINode", Resource: "csinodes",
		Singular: "csinode", Namespaced: false,
		StubReason: "Never populated with real data; same WaitForCacheSync reason as CSIDriver above.",
		New:        func() runtime.Object { return &storagev1.CSINode{} },
		NewList: func() runtime.Object {
			return &storagev1.CSINodeList{TypeMeta: metav1.TypeMeta{Kind: "CSINodeList", APIVersion: "storage.k8s.io/v1"}}
		},
	},

	// ---- node.k8s.io/v1 ----
	{
		GroupVersion: nodev1.SchemeGroupVersion, Kind: "RuntimeClass", Resource: "runtimeclasses",
		Singular: "runtimeclass", Namespaced: false,
		StubReason: "Never populated with real data; no controller in this project writes RuntimeClass objects today.",
		New:        func() runtime.Object { return &nodev1.RuntimeClass{} },
		NewList: func() runtime.Object {
			return &nodev1.RuntimeClassList{TypeMeta: metav1.TypeMeta{Kind: "RuntimeClassList", APIVersion: "node.k8s.io/v1"}}
		},
	},

	// ---- scheduling.k8s.io/v1 ----
	{
		// Phase 5 quick win (docs/general-purpose-k8s-plan.md). Cluster-scoped,
		// like RuntimeClass above -- unlike RuntimeClass, this one is
		// consumed for real: pkg/apiserver/priority.go resolves a Pod's
		// spec.priorityClassName against this store at Pod-create admission
		// (handler.go), the apiserver's job per real upstream's
		// plugin/pkg/admission/priority/admission.go, not the scheduler's.
		// The real, unmodified kube-scheduler binary (cmd/scheduler) then
		// preempts on the resolved spec.priority exactly as it already does
		// for a Pod that sets spec.priority directly.
		GroupVersion: schedulingv1.SchemeGroupVersion, Kind: "PriorityClass", Resource: "priorityclasses",
		Singular: "priorityclass", ShortNames: []string{"pc"}, Namespaced: false,
		New: func() runtime.Object { return &schedulingv1.PriorityClass{} },
		NewList: func() runtime.Object {
			return &schedulingv1.PriorityClassList{TypeMeta: metav1.TypeMeta{Kind: "PriorityClassList", APIVersion: "scheduling.k8s.io/v1"}}
		},
	},

	// ---- resource.k8s.io/v1 (Dynamic Resource Allocation) ----
	{
		GroupVersion: resourcev1.SchemeGroupVersion, Kind: "ResourceClaim", Resource: "resourceclaims",
		Singular: "resourceclaim", Namespaced: true,
		StubReason: "Never populated with real data -- exists only so a real kube-scheduler's Dynamic Resource Allocation informers (unconditionally started whenever the DRA feature gate is on, which is GA-locked as of Kubernetes 1.36) can complete their initial sync against an empty list instead of hanging in WaitForCacheSync forever. See docs/control-plane-architecture.md.",
		New:        func() runtime.Object { return &resourcev1.ResourceClaim{} },
		NewList: func() runtime.Object {
			return &resourcev1.ResourceClaimList{TypeMeta: metav1.TypeMeta{Kind: "ResourceClaimList", APIVersion: "resource.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: resourcev1.SchemeGroupVersion, Kind: "ResourceSlice", Resource: "resourceslices",
		Singular: "resourceslice", Namespaced: false,
		StubReason: "Never populated with real data; same DRA WaitForCacheSync reason as ResourceClaim above.",
		New:        func() runtime.Object { return &resourcev1.ResourceSlice{} },
		NewList: func() runtime.Object {
			return &resourcev1.ResourceSliceList{TypeMeta: metav1.TypeMeta{Kind: "ResourceSliceList", APIVersion: "resource.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: resourcev1.SchemeGroupVersion, Kind: "DeviceClass", Resource: "deviceclasses",
		Singular: "deviceclass", Namespaced: false,
		StubReason: "Never populated with real data; same DRA WaitForCacheSync reason as ResourceClaim above.",
		New:        func() runtime.Object { return &resourcev1.DeviceClass{} },
		NewList: func() runtime.Object {
			return &resourcev1.DeviceClassList{TypeMeta: metav1.TypeMeta{Kind: "DeviceClassList", APIVersion: "resource.k8s.io/v1"}}
		},
	},

	// ---- apps/v1 ----
	{
		GroupVersion: appsv1.SchemeGroupVersion, Kind: "ReplicaSet", Resource: "replicasets",
		Singular: "replicaset", ShortNames: []string{"rs"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource(), scaleSubresource()},
		New:          func() runtime.Object { return &appsv1.ReplicaSet{} },
		NewList: func() runtime.Object {
			return &appsv1.ReplicaSetList{TypeMeta: metav1.TypeMeta{Kind: "ReplicaSetList", APIVersion: "apps/v1"}}
		},
	},
	{
		GroupVersion: appsv1.SchemeGroupVersion, Kind: "Deployment", Resource: "deployments",
		Singular: "deployment", ShortNames: []string{"deploy"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource(), scaleSubresource()},
		New:          func() runtime.Object { return &appsv1.Deployment{} },
		NewList: func() runtime.Object {
			return &appsv1.DeploymentList{TypeMeta: metav1.TypeMeta{Kind: "DeploymentList", APIVersion: "apps/v1"}}
		},
	},
	{
		GroupVersion: appsv1.SchemeGroupVersion, Kind: "DaemonSet", Resource: "daemonsets",
		Singular: "daemonset", ShortNames: []string{"ds"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &appsv1.DaemonSet{} },
		NewList: func() runtime.Object {
			return &appsv1.DaemonSetList{TypeMeta: metav1.TypeMeta{Kind: "DaemonSetList", APIVersion: "apps/v1"}}
		},
	},
	{
		GroupVersion: appsv1.SchemeGroupVersion, Kind: "StatefulSet", Resource: "statefulsets",
		Singular: "statefulset", ShortNames: []string{"sts"}, Namespaced: true,
		// Populated for real by the real, unmodified kube-controller-manager's
		// statefulset controller (pkg/controller/statefulset, enabled via both
		// cmd/controller-manager's --controllers default and the WASM
		// Controllers DO's pkg/controllers.RunControllerManager) -- unlike the
		// stub types elsewhere in this table, this one is actually written to.
		// See docs/general-purpose-k8s-plan.md's Phase 3 StatefulSet entry for
		// what was verified end-to-end.
		Subresources: []Subresource{statusSubresource(), scaleSubresource()},
		New:          func() runtime.Object { return &appsv1.StatefulSet{} },
		NewList: func() runtime.Object {
			return &appsv1.StatefulSetList{TypeMeta: metav1.TypeMeta{Kind: "StatefulSetList", APIVersion: "apps/v1"}}
		},
	},
	{
		GroupVersion: appsv1.SchemeGroupVersion, Kind: "ControllerRevision", Resource: "controllerrevisions",
		Singular: "controllerrevision", Namespaced: true,
		// Originally registered only so DaemonSet/StatefulSet controllers'
		// ControllerRevision informer could complete WaitForCacheSync against
		// an empty list -- but both the real DaemonSet controller (Phase 3)
		// and now the real StatefulSet controller actually write real
		// ControllerRevision objects here (hash-based revision naming,
		// confirmed end-to-end: `kubectl get controllerrevisions` shows real
		// entries after creating either type). The doc comment previously on
		// this entry claimed it was "never populated with real data," which
		// was already inaccurate for DaemonSet before this StatefulSet work;
		// corrected per CLAUDE.md's honest-correction rule rather than left
		// in place.
		New: func() runtime.Object { return &appsv1.ControllerRevision{} },
		NewList: func() runtime.Object {
			return &appsv1.ControllerRevisionList{TypeMeta: metav1.TypeMeta{Kind: "ControllerRevisionList", APIVersion: "apps/v1"}}
		},
	},

	// ---- policy/v1 ----
	{
		GroupVersion: policyv1.SchemeGroupVersion, Kind: "PodDisruptionBudget", Resource: "poddisruptionbudgets",
		Singular: "poddisruptionbudget", ShortNames: []string{"pdb"}, Namespaced: true,
		StubReason: "Never populated with real data -- the real scheduler's DefaultPreemption plugin does PDB checks unconditionally, the same WaitForCacheSync reason as ReplicationController above.",
		New:        func() runtime.Object { return &policyv1.PodDisruptionBudget{} },
		NewList: func() runtime.Object {
			return &policyv1.PodDisruptionBudgetList{TypeMeta: metav1.TypeMeta{Kind: "PodDisruptionBudgetList", APIVersion: "policy/v1"}}
		},
	},

	// ---- discovery.k8s.io/v1 ----
	{
		GroupVersion: discoveryv1.SchemeGroupVersion, Kind: "EndpointSlice", Resource: "endpointslices",
		Singular: "endpointslice", Namespaced: true,
		// Populated for real by the Endpoints/EndpointSlice controller
		// (real kube-controller-manager, --controllers=+endpoint,endpointslice)
		// -- unlike the stub types above, this one is actually written to.
		New: func() runtime.Object { return &discoveryv1.EndpointSlice{} },
		NewList: func() runtime.Object {
			return &discoveryv1.EndpointSliceList{TypeMeta: metav1.TypeMeta{Kind: "EndpointSliceList", APIVersion: "discovery.k8s.io/v1"}}
		},
	},

	// ---- networking.k8s.io/v1 ----
	{
		GroupVersion: networkingv1.SchemeGroupVersion, Kind: "ServiceCIDR", Resource: "servicecidrs",
		Singular: "servicecidr", Namespaced: false,
		StubReason: "Never populated with real data -- MultiCIDRServiceAllocator is GA and LockToDefault: true as of Kubernetes 1.35 (pkg/features/kube_features.go), so kube-proxy's server.go unconditionally creates and starts a ServiceCIDR informer regardless of whether anything in the cluster uses dynamic ServiceCIDR allocation. Same stub-type pattern as resource.k8s.io/v1 and apps/v1 above.",
		New:        func() runtime.Object { return &networkingv1.ServiceCIDR{} },
		NewList: func() runtime.Object {
			return &networkingv1.ServiceCIDRList{TypeMeta: metav1.TypeMeta{Kind: "ServiceCIDRList", APIVersion: "networking.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: networkingv1.SchemeGroupVersion, Kind: "Ingress", Resource: "ingresses",
		Singular: "ingress", ShortNames: []string{"ing"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &networkingv1.Ingress{} },
		NewList: func() runtime.Object {
			return &networkingv1.IngressList{TypeMeta: metav1.TypeMeta{Kind: "IngressList", APIVersion: "networking.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: networkingv1.SchemeGroupVersion, Kind: "IngressClass", Resource: "ingressclasses",
		Singular: "ingressclass", Namespaced: false,
		New: func() runtime.Object { return &networkingv1.IngressClass{} },
		NewList: func() runtime.Object {
			return &networkingv1.IngressClassList{TypeMeta: metav1.TypeMeta{Kind: "IngressClassList", APIVersion: "networking.k8s.io/v1"}}
		},
	},
	{
		// CRUD+watch only, like Role/RoleBinding above -- nothing in this
		// project enforces NetworkPolicy today (no CNI plugin runs; pod
		// networking here is BYO VM + k3s embed, see CLAUDE.md's
		// architecture section), so a stored NetworkPolicy is not yet acted
		// on. Registered so `kubectl apply -f networkpolicy.yaml` doesn't
		// 404 outright.
		GroupVersion: networkingv1.SchemeGroupVersion, Kind: "NetworkPolicy", Resource: "networkpolicies",
		Singular: "networkpolicy", ShortNames: []string{"netpol"}, Namespaced: true,
		New: func() runtime.Object { return &networkingv1.NetworkPolicy{} },
		NewList: func() runtime.Object {
			return &networkingv1.NetworkPolicyList{TypeMeta: metav1.TypeMeta{Kind: "NetworkPolicyList", APIVersion: "networking.k8s.io/v1"}}
		},
	},

	// ---- batch/v1 ----
	{
		GroupVersion: batchv1.SchemeGroupVersion, Kind: "Job", Resource: "jobs",
		Singular: "job", Namespaced: true,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &batchv1.Job{} },
		NewList: func() runtime.Object {
			return &batchv1.JobList{TypeMeta: metav1.TypeMeta{Kind: "JobList", APIVersion: "batch/v1"}}
		},
	},
	{
		GroupVersion: batchv1.SchemeGroupVersion, Kind: "CronJob", Resource: "cronjobs",
		Singular: "cronjob", ShortNames: []string{"cj"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &batchv1.CronJob{} },
		NewList: func() runtime.Object {
			return &batchv1.CronJobList{TypeMeta: metav1.TypeMeta{Kind: "CronJobList", APIVersion: "batch/v1"}}
		},
	},

	// ---- rbac.authorization.k8s.io/v1 ----
	// Plain CRUD+watch like every other resource here -- this project's
	// authorization is still all-or-nothing per bearer token (see
	// pkg/apiserver/auth.go), so these objects aren't consulted to make any
	// access decision yet. Registered so `kubectl get roles`/`kubectl apply
	// -f role.yaml` etc. work (and so real controllers that read RBAC
	// objects, if ever embedded, have somewhere to read them from) -- not
	// because anything in this apiserver enforces them today.
	{
		GroupVersion: rbacv1.SchemeGroupVersion, Kind: "Role", Resource: "roles",
		Singular: "role", Namespaced: true,
		New: func() runtime.Object { return &rbacv1.Role{} },
		NewList: func() runtime.Object {
			return &rbacv1.RoleList{TypeMeta: metav1.TypeMeta{Kind: "RoleList", APIVersion: "rbac.authorization.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: rbacv1.SchemeGroupVersion, Kind: "RoleBinding", Resource: "rolebindings",
		Singular: "rolebinding", Namespaced: true,
		New: func() runtime.Object { return &rbacv1.RoleBinding{} },
		NewList: func() runtime.Object {
			return &rbacv1.RoleBindingList{TypeMeta: metav1.TypeMeta{Kind: "RoleBindingList", APIVersion: "rbac.authorization.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: rbacv1.SchemeGroupVersion, Kind: "ClusterRole", Resource: "clusterroles",
		Singular: "clusterrole", Namespaced: false,
		New: func() runtime.Object { return &rbacv1.ClusterRole{} },
		NewList: func() runtime.Object {
			return &rbacv1.ClusterRoleList{TypeMeta: metav1.TypeMeta{Kind: "ClusterRoleList", APIVersion: "rbac.authorization.k8s.io/v1"}}
		},
	},
	{
		GroupVersion: rbacv1.SchemeGroupVersion, Kind: "ClusterRoleBinding", Resource: "clusterrolebindings",
		Singular: "clusterrolebinding", Namespaced: false,
		New: func() runtime.Object { return &rbacv1.ClusterRoleBinding{} },
		NewList: func() runtime.Object {
			return &rbacv1.ClusterRoleBindingList{TypeMeta: metav1.TypeMeta{Kind: "ClusterRoleBindingList", APIVersion: "rbac.authorization.k8s.io/v1"}}
		},
	},

	// ---- autoscaling/v2 ----
	// v2, not v1: v2 is the version `kubectl autoscale`/`kubectl get hpa`
	// prefer and the one real HPA controllers target today; v1 is kept
	// upstream only for old clients and isn't worth a second registration
	// here. CRUD+watch only -- no controller in pkg/controllers reconciles
	// HPA's target replica count yet (the real
	// k8s.io/kubernetes/pkg/controller/podautoscaler package needs a
	// metrics client this project doesn't wire up), so a created HPA is
	// stored but not acted on, the same gap RBAC/NetworkPolicy above have.
	{
		GroupVersion: autoscalingv2.SchemeGroupVersion, Kind: "HorizontalPodAutoscaler", Resource: "horizontalpodautoscalers",
		Singular: "horizontalpodautoscaler", ShortNames: []string{"hpa"}, Namespaced: true,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &autoscalingv2.HorizontalPodAutoscaler{} },
		NewList: func() runtime.Object {
			return &autoscalingv2.HorizontalPodAutoscalerList{TypeMeta: metav1.TypeMeta{Kind: "HorizontalPodAutoscalerList", APIVersion: "autoscaling/v2"}}
		},
	},

	// ---- k8flare.com/v1alpha1 ----
	// k8flare's own management API (pkg/apis/k8flare/v1alpha1), not an
	// upstream group: a Cluster object is one tenant cluster, and creating
	// or deleting one is how a cluster is provisioned or torn down. It is a
	// compile-time built-in group, not a CRD -- one table entry buys the
	// same generic registry, discovery, watch, kubectl Table printing and
	// graceful deletion every upstream type here gets. See
	// docs/cluster-api-design.md. The only place this group is NOT treated
	// like the others is OpenAPI: upstream ships no schema document for it,
	// so cmd/k8flare-gen/openapi.go skips it (kubectl therefore does no
	// client-side validation of Cluster objects; the server accepts them
	// either way, since strict field validation is opt-in).
	//
	// The reconciling cluster-operator is P2; at P1 a Cluster is stored and
	// served but nothing acts on it.
	{
		GroupVersion: k8flarev1alpha1.SchemeGroupVersion, Kind: "Cluster", Resource: "clusters",
		Singular: "cluster", Namespaced: false,
		Subresources: []Subresource{statusSubresource()},
		New:          func() runtime.Object { return &k8flarev1alpha1.Cluster{} },
		NewList: func() runtime.Object {
			return &k8flarev1alpha1.ClusterList{TypeMeta: metav1.TypeMeta{Kind: "ClusterList", APIVersion: "k8flare.com/v1alpha1"}}
		},
	},
}

// EffectiveVerbs returns d.Verbs, or StandardVerbs if d.Verbs is nil.
func (d ResourceDef) EffectiveVerbs() []string {
	if d.Verbs != nil {
		return d.Verbs
	}
	return StandardVerbs
}

// HasSubresource reports whether resource has a subresource named
// subresourceName in Table. Used by pkg/apiserver/subresource.go's
// HandleSubresource to reject a subresource request for a resource that
// doesn't declare it, instead of dispatching by subresource name alone
// (which would let e.g. "status" reach any resource with a Go .Status
// field, whether or not the table -- and therefore discovery.go's
// advertised API surface -- says it has one).
func HasSubresource(resource, subresourceName string) bool {
	for _, def := range Table {
		if def.Resource != resource {
			continue
		}
		for _, sub := range def.Subresources {
			if sub.Name == subresourceName {
				return true
			}
		}
		return false
	}
	return false
}

// ForGroupVersion returns every ResourceDef in Table for the given
// GroupVersion, in table order.
func ForGroupVersion(gv schema.GroupVersion) []ResourceDef {
	var out []ResourceDef
	for _, def := range Table {
		if def.GroupVersion == gv {
			out = append(out, def)
		}
	}
	return out
}

// GroupVersions returns every distinct GroupVersion in Table, in the order
// each first appears.
func GroupVersions() []schema.GroupVersion {
	var out []schema.GroupVersion
	seen := make(map[schema.GroupVersion]bool)
	for _, def := range Table {
		if !seen[def.GroupVersion] {
			seen[def.GroupVersion] = true
			out = append(out, def.GroupVersion)
		}
	}
	return out
}

// APIPrefix returns gv's REST API path prefix: "/api/v1/" for the legacy
// core group, "/apis/{group}/{version}/" for a named group. Matches the
// mux pattern shape workers/apiserver/main.go registers one route per
// GroupVersion with.
func APIPrefix(gv schema.GroupVersion) string {
	if gv.Group == "" {
		return "/api/" + gv.Version + "/"
	}
	return "/apis/" + gv.Group + "/" + gv.Version + "/"
}
