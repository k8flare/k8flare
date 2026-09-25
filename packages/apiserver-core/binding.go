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
	"k8s.io/apiserver/pkg/storage"
	storageerr "k8s.io/apiserver/pkg/storage/errors"
	"k8s.io/apiserver/pkg/util/dryrun"
)

type bindingREST struct {
	pods *registry.Store
}

var _ rest.NamedCreater = bindingREST{}

func (bindingREST) New() runtime.Object     { return &corev1.Binding{} }
func (bindingREST) Destroy()                {}
func (bindingREST) NamespaceScoped() bool   { return true }
func (bindingREST) GetSingularName() string { return "binding" }

func (r bindingREST) Create(ctx context.Context, name string, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
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
	key, err := r.pods.KeyFunc(ctx, name)
	if err != nil {
		return nil, err
	}
	if options == nil {
		options = &metav1.CreateOptions{}
	}
	out := r.pods.NewFunc()
	err = r.pods.Storage.GuaranteedUpdate(ctx, key, out, false, nil, storage.SimpleUpdate(func(existing runtime.Object) (runtime.Object, error) {
		pod := existing.DeepCopyObject().(*corev1.Pod)
		if pod.DeletionTimestamp != nil {
			return nil, apierrors.NewConflict(corev1.Resource("pods/binding"), pod.Name, fmt.Errorf("pod %s is being deleted, cannot be assigned to a host", pod.Name))
		}
		if pod.Spec.NodeName != "" {
			return nil, apierrors.NewConflict(corev1.Resource("pods/binding"), pod.Name, fmt.Errorf("pod %v is already assigned to node %q", pod.Name, pod.Spec.NodeName))
		}
		if len(pod.Spec.SchedulingGates) != 0 {
			return nil, apierrors.NewConflict(corev1.Resource("pods/binding"), pod.Name, fmt.Errorf("pod %v has non-empty .spec.schedulingGates", pod.Name))
		}
		pod.Spec.NodeName = binding.Target.Name
		pod.Status.NominatedNodeName = ""
		if len(binding.Annotations) > 0 {
			if pod.Annotations == nil {
				pod.Annotations = map[string]string{}
			}
			for k, v := range binding.Annotations {
				pod.Annotations[k] = v
			}
		}
		if len(binding.Labels) > 0 {
			if pod.Labels == nil {
				pod.Labels = map[string]string{}
			}
			for k, v := range binding.Labels {
				pod.Labels[k] = v
			}
		}
		setPodScheduled(pod)
		return pod, nil
	}), dryrun.IsDryRun(options.DryRun), nil)
	if err != nil {
		err = storageerr.InterpretGetError(err, corev1.Resource("pods/binding"), name)
		err = storageerr.InterpretUpdateError(err, corev1.Resource("pods/binding"), name)
		if _, ok := err.(*apierrors.StatusError); !ok {
			err = apierrors.NewInternalError(err)
		}
		return nil, err
	}
	return &metav1.Status{Status: metav1.StatusSuccess}, nil
}

type legacyBindingREST struct{}

var (
	_ rest.Creater              = legacyBindingREST{}
	_ rest.Scoper               = legacyBindingREST{}
	_ rest.SingularNameProvider = legacyBindingREST{}
)

func (legacyBindingREST) New() runtime.Object     { return &corev1.Binding{} }
func (legacyBindingREST) Destroy()                {}
func (legacyBindingREST) NamespaceScoped() bool   { return true }
func (legacyBindingREST) GetSingularName() string { return "binding" }

func (legacyBindingREST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	binding, ok := obj.(*corev1.Binding)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("not a Binding object: %T", obj))
	}
	if bindingPods.store == nil {
		return nil, apierrors.NewServiceUnavailable("pods store is not ready")
	}
	return bindingREST{bindingPods.store}.Create(ctx, binding.Name, obj, createValidation, options)
}

func setPodScheduled(pod *corev1.Pod) {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type != corev1.PodScheduled {
			continue
		}
		if pod.Status.Conditions[i].Status == corev1.ConditionTrue {
			return
		}
		pod.Status.Conditions[i].Status = corev1.ConditionTrue
		pod.Status.Conditions[i].LastTransitionTime = metav1.Now()
		pod.Status.Conditions[i].Reason = ""
		pod.Status.Conditions[i].Message = ""
		return
	}
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()})
}
