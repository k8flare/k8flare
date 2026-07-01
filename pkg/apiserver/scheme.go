package apiserver

import (
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// Scheme is the runtime.Scheme that registers core Kubernetes types
// used by this minimal API server.
var Scheme = runtime.NewScheme()

// Codecs provides encoders and decoders for the registered types.
var Codecs serializer.CodecFactory

// jsonSerializer is used internally for JSON encode/decode operations.
var jsonSerializer runtime.Serializer

func init() {
	// Register core/v1 types
	Scheme.AddKnownTypes(corev1.SchemeGroupVersion,
		&corev1.Namespace{},
		&corev1.NamespaceList{},
		&corev1.ConfigMap{},
		&corev1.ConfigMapList{},
		&corev1.Secret{},
		&corev1.SecretList{},
		&corev1.Pod{},
		&corev1.PodList{},
		&corev1.Binding{},
		&corev1.Node{},
		&corev1.NodeList{},
		&corev1.ServiceAccount{},
		&corev1.ServiceAccountList{},
		&corev1.Endpoints{},
		&corev1.EndpointsList{},
		&corev1.Service{},
		&corev1.ServiceList{},
		&corev1.Event{},
		&corev1.EventList{},
		&corev1.LimitRange{},
		&corev1.LimitRangeList{},
		// ReplicationController: never populated with real data — see the
		// stub-type comment below the resource.k8s.io/v1 block.
		&corev1.ReplicationController{},
		&corev1.ReplicationControllerList{},
	)

	// Register coordination/v1 types
	Scheme.AddKnownTypes(coordinationv1.SchemeGroupVersion,
		&coordinationv1.Lease{},
		&coordinationv1.LeaseList{},
	)

	// Register storage/v1 types
	Scheme.AddKnownTypes(storagev1.SchemeGroupVersion,
		&storagev1.CSIDriver{},
		&storagev1.CSIDriverList{},
		&storagev1.CSINode{},
		&storagev1.CSINodeList{},
	)

	// Register node/v1 types
	Scheme.AddKnownTypes(nodev1.SchemeGroupVersion,
		&nodev1.RuntimeClass{},
		&nodev1.RuntimeClassList{},
	)

	// Register resource.k8s.io/v1 types. ResourceClaim/ResourceSlice are
	// never populated with real data — they exist only so that a real
	// kube-scheduler's Dynamic Resource Allocation informers (unconditionally
	// started whenever the DRA feature gate is on, which is GA-locked as of
	// Kubernetes 1.36) can complete their initial sync against an empty list
	// instead of hanging in WaitForCacheSync forever. See
	// docs/control-plane-architecture.md.
	Scheme.AddKnownTypes(resourcev1.SchemeGroupVersion,
		&resourcev1.ResourceClaim{},
		&resourcev1.ResourceClaimList{},
		&resourcev1.ResourceSlice{},
		&resourcev1.ResourceSliceList{},
		&resourcev1.DeviceClass{},
		&resourcev1.DeviceClassList{},
	)

	// Register apps/v1 and policy/v1 types. Like the resource.k8s.io/v1 types
	// above, ReplicaSet/StatefulSet/PodDisruptionBudget (and
	// ReplicationController, registered in the core/v1 block above) are
	// never populated with real data — confirmed empirically by running the
	// real scheduler: its default InterPodAffinity/PodTopologySpread
	// (owning-controller lookups) and DefaultPreemption (PDB checks) plugins
	// start informers for these types unconditionally, the same way DRA's
	// plugin does for ResourceClaim/ResourceSlice/DeviceClass, even when no
	// pod in the cluster uses any of the corresponding features.
	Scheme.AddKnownTypes(appsv1.SchemeGroupVersion,
		&appsv1.ReplicaSet{},
		&appsv1.ReplicaSetList{},
		&appsv1.StatefulSet{},
		&appsv1.StatefulSetList{},
	)
	Scheme.AddKnownTypes(policyv1.SchemeGroupVersion,
		&policyv1.PodDisruptionBudget{},
		&policyv1.PodDisruptionBudgetList{},
	)

	// Register discovery/v1 types. EndpointSlice is populated for real by the
	// Endpoints/EndpointSlice controller (packages/etcd/src/endpoints.ts) --
	// unlike the stub types above, this one is actually written to.
	Scheme.AddKnownTypes(discoveryv1.SchemeGroupVersion,
		&discoveryv1.EndpointSlice{},
		&discoveryv1.EndpointSliceList{},
	)

	// Register networking.k8s.io/v1 ServiceCIDR. Never populated with real
	// data -- exists only because MultiCIDRServiceAllocator is GA and
	// LockToDefault: true as of Kubernetes 1.35 (pkg/features/kube_features.go),
	// so kube-proxy's server.go unconditionally creates and starts a
	// ServiceCIDR informer regardless of whether anything in the cluster
	// uses dynamic ServiceCIDR allocation. Same stub-type pattern as
	// resource.k8s.io/v1 and apps/v1 above.
	Scheme.AddKnownTypes(networkingv1.SchemeGroupVersion,
		&networkingv1.ServiceCIDR{},
		&networkingv1.ServiceCIDRList{},
	)

	// Register metav1 types (Status, ListMeta, etc.)
	metav1.AddToGroupVersion(Scheme, corev1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, coordinationv1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, storagev1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, nodev1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, resourcev1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, appsv1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, policyv1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, discoveryv1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, networkingv1.SchemeGroupVersion)

	Codecs = serializer.NewCodecFactory(Scheme)

	jsonSerializer = kjson.NewSerializerWithOptions(
		kjson.DefaultMetaFactory, Scheme, Scheme,
		kjson.SerializerOptions{
			Pretty: false,
			Strict: false,
		},
	)
}

// Encode serializes a runtime.Object to JSON bytes.
func Encode(obj runtime.Object) ([]byte, error) {
	return runtime.Encode(jsonSerializer, obj)
}

// Decode deserializes JSON bytes into a runtime.Object.
// If gvk is nil, the type is inferred from the JSON payload.
func Decode(data []byte, gvk *schema.GroupVersionKind) (runtime.Object, error) {
	obj, _, err := jsonSerializer.Decode(data, gvk, nil)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return obj, nil
}

// EncodeToStorage serializes a runtime.Object to JSON suitable for kine storage.
// It strips managed fields and status metadata to keep stored data clean.
func EncodeToStorage(obj runtime.Object) ([]byte, error) {
	// Marshal to an unstructured map so we can strip fields before storage.
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("encode to storage: marshal: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("encode to storage: unmarshal map: %w", err)
	}

	// Strip metadata fields that should not be persisted in kine storage.
	if metadata, ok := m["metadata"].(map[string]interface{}); ok {
		delete(metadata, "managedFields")
	}

	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode to storage: re-marshal: %w", err)
	}
	return out, nil
}

// DecodeFromStorage deserializes JSON bytes from kine storage into
// the provided runtime.Object.
func DecodeFromStorage(data []byte, into runtime.Object) error {
	_, _, err := jsonSerializer.Decode(data, nil, into)
	if err != nil {
		return fmt.Errorf("decode from storage: %w", err)
	}
	return nil
}
