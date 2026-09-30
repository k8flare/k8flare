package core

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	podutil "k8s.io/kubernetes/pkg/api/pod"
	apicore "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/helper/qos"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
)

type podInternalStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

var podInternal = podInternalStrategy{legacyscheme.Scheme, names.SimpleNameGenerator}

func (podInternalStrategy) NamespaceScoped() bool { return true }

func (podInternalStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return map[fieldpath.APIVersion]*fieldpath.Set{"v1": fieldpath.NewSet(fieldpath.MakePathOrDie("status"))}
}

func (podInternalStrategy) PrepareForCreate(_ context.Context, obj runtime.Object) {
	pod := obj.(*apicore.Pod)
	pod.Generation = 1
	pod.Status = apicore.PodStatus{Phase: apicore.PodPending, QOSClass: qos.GetPodQOS(pod)}
	podutil.DropDisabledPodFields(pod, nil)
}

func (podInternalStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	pod, previous := obj.(*apicore.Pod), old.(*apicore.Pod)
	pod.Status = previous.Status
	podutil.DropDisabledPodFields(pod, previous)
	if !apiequality.Semantic.DeepEqual(pod.Spec, previous.Spec) {
		pod.Generation++
	}
}

func (podInternalStrategy) Validate(_ context.Context, obj runtime.Object) field.ErrorList {
	pod := obj.(*apicore.Pod)
	opts := podutil.GetValidationOptionsFromPodSpecAndMeta(&pod.Spec, nil, &pod.ObjectMeta, nil)
	opts.ResourceIsPod = true
	return corevalidation.ValidatePodCreate(pod, opts)
}

func (podInternalStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	pod := obj.(*apicore.Pod)
	var warnings []string
	if msgs := utilvalidation.IsDNS1123Label(pod.Name); len(msgs) != 0 {
		warnings = append(warnings, fmt.Sprintf("metadata.name: this is used in the Pod's hostname, which can result in surprising behavior; a DNS label is recommended: %v", msgs))
	}
	return append(warnings, podutil.GetWarningsForPod(ctx, pod, nil)...)
}

func (podInternalStrategy) Canonicalize(runtime.Object) {}

func (podInternalStrategy) AllowCreateOnUpdate() bool { return false }

func (podInternalStrategy) AllowUnconditionalUpdate() bool { return true }

func (podInternalStrategy) ValidateUpdate(_ context.Context, obj, old runtime.Object) field.ErrorList {
	pod, previous := obj.(*apicore.Pod), old.(*apicore.Pod)
	opts := podutil.GetValidationOptionsFromPodSpecAndMeta(&pod.Spec, &previous.Spec, &pod.ObjectMeta, &previous.ObjectMeta)
	opts.ResourceIsPod = true
	return corevalidation.ValidatePodUpdate(pod, previous, opts)
}

func (podInternalStrategy) WarningsOnUpdate(context.Context, runtime.Object, runtime.Object) []string {
	return nil
}

type podInternalStatusStrategy struct{ podInternalStrategy }

var podStatusInternal = podInternalStatusStrategy{podInternal}

func (podInternalStatusStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return map[fieldpath.APIVersion]*fieldpath.Set{"v1": fieldpath.NewSet(
		fieldpath.MakePathOrDie("spec"),
		fieldpath.MakePathOrDie("metadata", "deletionTimestamp"),
		fieldpath.MakePathOrDie("metadata", "ownerReferences"),
	)}
}

func (podInternalStatusStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	pod, previous := obj.(*apicore.Pod), old.(*apicore.Pod)
	pod.Spec = previous.Spec
	pod.DeletionTimestamp = nil
	pod.OwnerReferences = previous.OwnerReferences
	if pod.Status.QOSClass == "" {
		pod.Status.QOSClass = previous.Status.QOSClass
	}
	podutil.DropDisabledPodFields(pod, previous)
}

func (podInternalStatusStrategy) ValidateUpdate(_ context.Context, obj, old runtime.Object) field.ErrorList {
	pod, previous := obj.(*apicore.Pod), old.(*apicore.Pod)
	opts := podutil.GetValidationOptionsFromPodSpecAndMeta(&pod.Spec, &previous.Spec, &pod.ObjectMeta, &previous.ObjectMeta)
	opts.ResourceIsPod = true
	return corevalidation.ValidatePodStatusUpdate(pod, previous, opts)
}

func (podInternalStatusStrategy) WarningsOnUpdate(_ context.Context, obj, _ runtime.Object) []string {
	pod := obj.(*apicore.Pod)
	var warnings []string
	for i, podIP := range pod.Status.PodIPs {
		warnings = append(warnings, utilvalidation.GetWarningsForIP(field.NewPath("status", "podIPs").Index(i).Child("ip"), podIP.IP)...)
	}
	for i, hostIP := range pod.Status.HostIPs {
		warnings = append(warnings, utilvalidation.GetWarningsForIP(field.NewPath("status", "hostIPs").Index(i).Child("ip"), hostIP.IP)...)
	}
	return warnings
}

type nodeInternalStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

var nodeInternal = nodeInternalStrategy{legacyscheme.Scheme, names.SimpleNameGenerator}

func (nodeInternalStrategy) NamespaceScoped() bool { return false }

func (nodeInternalStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return map[fieldpath.APIVersion]*fieldpath.Set{"v1": fieldpath.NewSet(fieldpath.MakePathOrDie("status"))}
}

func (nodeInternalStrategy) AllowCreateOnUpdate() bool { return false }

func (nodeInternalStrategy) AllowUnconditionalUpdate() bool { return true }

func (nodeInternalStrategy) PrepareForCreate(_ context.Context, obj runtime.Object) {
	node := obj.(*apicore.Node)
	node.Spec.ConfigSource = nil
	node.Status.Config = nil
}

func (nodeInternalStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	node, previous := obj.(*apicore.Node), old.(*apicore.Node)
	node.Status = previous.Status
	if previous.Spec.ConfigSource == nil {
		node.Spec.ConfigSource = nil
	}
}

func (nodeInternalStrategy) Validate(_ context.Context, obj runtime.Object) field.ErrorList {
	return corevalidation.ValidateNode(obj.(*apicore.Node))
}

func (nodeInternalStrategy) WarningsOnCreate(context.Context, runtime.Object) []string { return nil }

func (nodeInternalStrategy) Canonicalize(runtime.Object) {}

func (nodeInternalStrategy) ValidateUpdate(_ context.Context, obj, old runtime.Object) field.ErrorList {
	node, previous := obj.(*apicore.Node), old.(*apicore.Node)
	return append(corevalidation.ValidateNode(node), corevalidation.ValidateNodeUpdate(node, previous)...)
}

func (nodeInternalStrategy) WarningsOnUpdate(context.Context, runtime.Object, runtime.Object) []string {
	return nil
}

type nodeInternalStatusStrategy struct{ nodeInternalStrategy }

var nodeStatusInternal = nodeInternalStatusStrategy{nodeInternal}

func (nodeInternalStatusStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return map[fieldpath.APIVersion]*fieldpath.Set{"v1": fieldpath.NewSet(fieldpath.MakePathOrDie("spec"))}
}

func (nodeInternalStatusStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	node, previous := obj.(*apicore.Node), old.(*apicore.Node)
	node.Spec = previous.Spec
	if previous.Status.Config == nil {
		node.Status.Config = nil
	}
}
