package apiserver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

// bindingREST serves pods/binding: the path the real kube-scheduler uses to
// place a Pod, by POSTing a Binding rather than writing spec.nodeName
// directly. Without it registered the scheduler's Bind step fails and no Pod
// is ever placed, which is what the KCM lane caught.
//
// rest.NamedCreater rather than rest.Creater: the Pod's name comes from the
// URL, and the Binding in the body names only the target.
type bindingREST struct {
	store *registry.Store
}

func newBindingREST(parent *registry.Store) *bindingREST { return &bindingREST{store: parent} }

func (r *bindingREST) New() runtime.Object { return &corev1.Binding{} }

func (r *bindingREST) Destroy() {}

func (r *bindingREST) GroupVersionKind(schema.GroupVersion) schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Binding")
}

func (r *bindingREST) Create(ctx context.Context, name string, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	binding, ok := obj.(*corev1.Binding)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("request body is %T, expected a Binding", obj))
	}
	if binding.Target.Name == "" {
		return nil, apierrors.NewBadRequest("binding target name is empty")
	}
	if createValidation != nil {
		if err := createValidation(ctx, binding); err != nil {
			return nil, err
		}
	}

	_, _, err := r.store.Update(ctx, name, rest.DefaultUpdatedObjectInfo(nil,
		func(_ context.Context, _, oldObj runtime.Object) (runtime.Object, error) {
			pod, ok := oldObj.DeepCopyObject().(*corev1.Pod)
			if !ok {
				return nil, apierrors.NewInternalError(fmt.Errorf("binding target is %T, expected a Pod", oldObj))
			}
			if pod.Spec.NodeName != "" && pod.Spec.NodeName != binding.Target.Name {
				return nil, apierrors.NewConflict(
					corev1.Resource("pods"), name,
					fmt.Errorf("pod is already assigned to node %q", pod.Spec.NodeName))
			}
			pod.Spec.NodeName = binding.Target.Name
			return pod, nil
		}), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false,
		&metav1.UpdateOptions{DryRun: options.DryRun, FieldManager: options.FieldManager})
	if err != nil {
		return nil, err
	}
	return binding, nil
}
