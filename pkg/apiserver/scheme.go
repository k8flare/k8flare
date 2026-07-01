package apiserver

import (
	"encoding/json"
	"fmt"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	nodev1 "k8s.io/api/node/v1"
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
	)

	// Register events/v1 types
	Scheme.AddKnownTypes(eventsv1.SchemeGroupVersion,
		&eventsv1.Event{},
		&eventsv1.EventList{},
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

	// Register metav1 types (Status, ListMeta, etc.)
	metav1.AddToGroupVersion(Scheme, corev1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, eventsv1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, coordinationv1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, storagev1.SchemeGroupVersion)
	metav1.AddToGroupVersion(Scheme, nodev1.SchemeGroupVersion)

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
