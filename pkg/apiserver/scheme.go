package apiserver

import (
	"encoding/json"
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
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

// strictJSONSerializer is jsonSerializer's strict-decoding twin, used only
// by DecodeStrict (fieldvalidation.go) to implement `?fieldValidation=Strict`
// /`Warn`. Strict:true is upstream's own real mechanism (its unmarshal calls
// sigs.k8s.io/json's UnmarshalStrict, the same function real
// kube-apiserver's strict field validation is built on), not a
// reimplementation -- see fieldvalidation.go's doc comment for why a second
// Serializer value is needed instead of a flag on the existing one
// (kjson.SerializerOptions is documented as immutable once passed to
// NewSerializerWithOptions).
var strictJSONSerializer runtime.Serializer

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

	// autoscaling/v1.Scale is the response/request body for every
	// resource's "scale" subresource (deployments/replicasets/statefulsets
	// in apidef.Table) -- registered directly for the same reason Binding
	// is: it's not itself a listable/gettable top-level resource, so it
	// doesn't fit apidef.ResourceDef's shape. See subresource.go's
	// handleScaleSubresource.
	Scheme.AddKnownTypes(autoscalingv1.SchemeGroupVersion, &autoscalingv1.Scale{})

	// authorization.k8s.io/v1.SelfSubjectAccessReview is the body `kubectl
	// auth can-i` POSTs and reads back. Registered directly, same reason as
	// Scale/Binding above: it's a compute-on-request type with no
	// ResourceStore behind it (see selfsubjectaccessreview.go), so it isn't
	// in apidef.Table either. SubjectAccessReview and TokenReview join it
	// for the kubelet's webhook authorizer/authenticator (the per-Pod
	// microVM nodes' logs/metrics bridge -- see tokenreview.go).
	Scheme.AddKnownTypes(authorizationv1.SchemeGroupVersion,
		&authorizationv1.SelfSubjectAccessReview{},
		&authorizationv1.SubjectAccessReview{})
	// TokenRequest joins TokenReview here for the same reason -- the
	// ServiceAccount TokenRequest subresource (serviceaccounttoken.go),
	// not part of apidef.Table since it's a subresource of ServiceAccount
	// rather than its own top-level resource.
	Scheme.AddKnownTypes(authenticationv1.SchemeGroupVersion,
		&authenticationv1.TokenReview{},
		&authenticationv1.TokenRequest{})

	// metav1.Table is the meta.k8s.io/v1 response body kubectl requests via
	// "Accept: application/json;as=Table;v=v1;g=meta.k8s.io" for its default
	// human-readable "get" output. AddMetaToScheme is the real upstream
	// registration call (registers Table/TableOptions/PartialObjectMetadata
	// under meta.k8s.io/v1) -- reused rather than hand-listing these types.
	// See table.go's ConvertToTable.
	if err := metav1.AddMetaToScheme(Scheme); err != nil {
		panic(fmt.Sprintf("register meta.k8s.io/v1 types: %v", err))
	}

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
	strictJSONSerializer = kjson.NewSerializerWithOptions(
		kjson.DefaultMetaFactory, Scheme, Scheme,
		kjson.SerializerOptions{
			Pretty: false,
			Strict: true,
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

// DecodeStrict deserializes JSON bytes into a runtime.Object the same way
// Decode does, but additionally reports every unknown or duplicate field
// found (fieldvalidation.go's `?fieldValidation=Strict`/`Warn` support). obj
// is still populated even when strictErrs is non-empty -- matching
// strictJSONSerializer's own contract (see its doc comment) -- callers
// decide whether that's a hard failure (Strict) or a warning (Warn).
//
// Only JSON bodies can be checked this way: protobuf has no equivalent
// "unknown field" concept in this project's decode path (Codecs's protobuf
// serializer doesn't track it), so a protobuf body must go through the
// existing lenient Decode/decodeBody instead -- see fieldvalidation.go's
// isJSONBody.
func DecodeStrict(data []byte, gvk *schema.GroupVersionKind) (obj runtime.Object, strictErrs []error, err error) {
	obj, _, err = strictJSONSerializer.Decode(data, gvk, nil)
	if err != nil {
		if sde, ok := runtime.AsStrictDecodingError(err); ok {
			return obj, sde.Errors(), nil
		}
		return nil, nil, fmt.Errorf("decode: %w", err)
	}
	return obj, nil, nil
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
