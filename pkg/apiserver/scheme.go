package apiserver

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kjson "k8s.io/apimachinery/pkg/runtime/serializer/json"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// Scheme is the runtime.Scheme that registers core Kubernetes types
// used by this minimal API server.
var Scheme = runtime.NewScheme()

// Codecs provides encoders and decoders for the registered types.
var Codecs serializer.CodecFactory

// jsonSerializer is used internally for JSON encode/decode operations.
var jsonSerializer runtime.Serializer

func init() {
	// Register every apidef.Table entry's type + list type, and metav1's
	// shared types (Status, ListMeta, ...), once per GroupVersion. This
	// used to be one hand-written Scheme.AddKnownTypes call per group here,
	// each duplicating the per-resource type list already implied by
	// resources.go's store constructors and discovery.go's advertised
	// resources -- see apidef.Table's doc comment (pkg/apiserver/apidef/table.go)
	// for the drift that caused (e.g. apps/v1 and batch/v1's /status
	// subresources were implemented but never advertised in discovery).
	for _, gv := range apidef.GroupVersions() {
		var objs []runtime.Object
		for _, def := range apidef.ForGroupVersion(gv) {
			objs = append(objs, def.New(), def.NewList())
		}
		Scheme.AddKnownTypes(gv, objs...)
		metav1.AddToGroupVersion(Scheme, gv)
	}

	// Binding is a special non-resource type: it only ever appears as the
	// POST body for the pods/binding subresource (subresource.go), so it
	// isn't listable/gettable/watchable and doesn't fit apidef.ResourceDef's
	// shape (which assumes New+NewList). Registered directly rather than
	// stretching the table for one entry.
	Scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Binding{})

	// Register every API group's real upstream versioned defaulting
	// functions (Deployment/DaemonSet/ReplicaSet's strategy defaults,
	// Job/CronJob's completionMode, core/v1's Pod/Service/... defaults,
	// etc.) onto Scheme, rather than hand-reimplementing them. Generated
	// from apidef.Table -- see zz_generated_defaulters.go and
	// cmd/k8flare-gen/defaulters.go. Scheme.Default(obj) (called from
	// ApplyDefaults, defaults.go) then applies whichever of these is
	// registered for obj's concrete type. Discovered necessary when
	// embedding the real kube-controller-manager: its deployment
	// controller hard-errors ("unexpected deployment strategy type") on a
	// Deployment whose spec.strategy.type is empty, since real clients rely
	// on apiserver-side admission defaulting to fill it in before it's ever
	// stored -- this apiserver has no admission chain, so ApplyDefaults is
	// the only place left to do it.
	if err := registerVersionedDefaults(); err != nil {
		panic(fmt.Sprintf("register versioned defaults: %v", err))
	}

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
