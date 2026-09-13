package core

import (
	"context"
	"fmt"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

type bindingREST struct {
	pods *registry.Store
}

var _ rest.NamedCreater = bindingREST{}

func (bindingREST) New() runtime.Object     { return &corev1.Binding{} }
func (bindingREST) Destroy()                {}
func (bindingREST) NamespaceScoped() bool   { return true }
func (bindingREST) GetSingularName() string { return "binding" }

func (r bindingREST) Create(ctx context.Context, name string, obj runtime.Object, createValidation rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	binding, ok := obj.(*corev1.Binding)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("not a Binding object: %T", obj))
	}
	if name != binding.Name {
		return nil, apierrors.NewBadRequest("the name of the object does not match the name on the URL")
	}
	if createValidation != nil {
		if err := createValidation(ctx, obj.DeepCopyObject()); err != nil {
			return nil, err
		}
	}
	_, _, err := r.pods.Update(ctx, binding.Name, rest.DefaultUpdatedObjectInfo(nil, func(_ context.Context, _, old runtime.Object) (runtime.Object, error) {
		pod := old.DeepCopyObject().(*corev1.Pod)
		if pod.DeletionTimestamp != nil {
			return nil, apierrors.NewConflict(corev1.Resource("pods/binding"), pod.Name, fmt.Errorf("pod %s is being deleted, cannot be assigned to a host", pod.Name))
		}
		if pod.Spec.NodeName != "" {
			return nil, apierrors.NewConflict(corev1.Resource("pods/binding"), pod.Name, fmt.Errorf("pod %v is already assigned to node %q", pod.Name, pod.Spec.NodeName))
		}
		pod.Spec.NodeName = binding.Target.Name
		if len(binding.Annotations) > 0 {
			if pod.Annotations == nil {
				pod.Annotations = map[string]string{}
			}
			for k, v := range binding.Annotations {
				pod.Annotations[k] = v
			}
		}
		setPodScheduled(pod)
		return pod, nil
	}), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return &metav1.Status{Status: metav1.StatusSuccess}, nil
}

func setPodScheduled(pod *corev1.Pod) {
	now := metav1.Now()
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == corev1.PodScheduled {
			pod.Status.Conditions[i] = corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: now, LastProbeTime: pod.Status.Conditions[i].LastProbeTime}
			return
		}
	}
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: now})
}
