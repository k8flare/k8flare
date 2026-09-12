package apiserver

import (
	"fmt"
	"reflect"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// scaleFromObject builds an autoscaling/v1.Scale view of obj (a Deployment,
// ReplicaSet, or StatefulSet) via reflection on the Spec.Replicas/
// Status.Replicas/Spec.Selector fields every one of those types shares,
// matching upstream's per-type convertToScale (e.g.
// pkg/registry/apps/deployment/storage/storage.go) without a copy of it per
// type.
func scaleFromObject(obj runtime.Object) (*autoscalingv1.Scale, error) {
	objMeta := getObjectMeta(obj)
	if objMeta == nil {
		return nil, fmt.Errorf("%T does not implement ObjectMetaAccessor", obj)
	}

	v := reflect.ValueOf(obj).Elem()
	specVal := v.FieldByName("Spec")
	statusVal := v.FieldByName("Status")
	if !specVal.IsValid() || !statusVal.IsValid() {
		return nil, fmt.Errorf("%T has no Spec/Status field", obj)
	}

	var specReplicas int32
	if replicasField := specVal.FieldByName("Replicas"); replicasField.IsValid() && !replicasField.IsNil() {
		specReplicas = int32(replicasField.Elem().Int())
	}

	var statusReplicas int32
	if replicasField := statusVal.FieldByName("Replicas"); replicasField.IsValid() {
		statusReplicas = int32(replicasField.Int())
	}

	var selectorStr string
	if selectorField := specVal.FieldByName("Selector"); selectorField.IsValid() && !selectorField.IsNil() {
		if selector, ok := selectorField.Interface().(*metav1.LabelSelector); ok {
			if s, err := metav1.LabelSelectorAsSelector(selector); err == nil {
				selectorStr = s.String()
			}
		}
	}

	return &autoscalingv1.Scale{
		// Encode (scheme.go) is a bare json.Marshal underneath -- it does
		// not consult Scheme to fill in TypeMeta the way a full versioning
		// codec would, so every response this apiserver writes is
		// responsible for setting its own kind/apiVersion if it needs one.
		// Most callers get away without it (typed client-go Get/List calls
		// already know their expected type and never look at the response's
		// kind), but kubectl's `scale` command decodes this specific
		// response with a kind-aware (effectively dynamic/unstructured)
		// client and hard-errors with "Object 'Kind' is missing" without
		// this -- found via real kubectl, not just client-go, against this
		// handler.
		TypeMeta: metav1.TypeMeta{Kind: "Scale", APIVersion: "autoscaling/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              objMeta.Name,
			Namespace:         objMeta.Namespace,
			UID:               objMeta.UID,
			ResourceVersion:   objMeta.ResourceVersion,
			CreationTimestamp: objMeta.CreationTimestamp,
		},
		Spec:   autoscalingv1.ScaleSpec{Replicas: specReplicas},
		Status: autoscalingv1.ScaleStatus{Replicas: statusReplicas, Selector: selectorStr},
	}, nil
}

// applyReplicasToObject sets obj's Spec.Replicas (a Deployment, ReplicaSet,
// or StatefulSet) to replicas, in place. The counterpart write half of
// scaleFromObject above.
func applyReplicasToObject(obj runtime.Object, replicas int32) error {
	v := reflect.ValueOf(obj).Elem()
	specVal := v.FieldByName("Spec")
	if !specVal.IsValid() {
		return fmt.Errorf("%T has no Spec field", obj)
	}
	replicasField := specVal.FieldByName("Replicas")
	if !replicasField.IsValid() || replicasField.Type() != reflect.TypeOf((*int32)(nil)) {
		return fmt.Errorf("%T.Spec.Replicas is not *int32", obj)
	}
	replicasField.Set(reflect.ValueOf(&replicas))
	return nil
}
