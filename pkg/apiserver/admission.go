package apiserver

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/admission"
)

// k8flareAdmission is the create-time behaviour that used to sit inline in
// handler.go's POST case, moved to the extension point upstream reserves for
// it so that it runs for every route the real installer builds rather than
// only for requests that happen to pass through a hand-written handler.
//
// Order is the order it ran in before, and it matters: the namespace read
// feeds compute-class routing, LimitRange defaulting sizes the Pod that
// AssignContainersNode then measures.
type k8flareAdmission struct {
	namespaces       *ResourceStore
	priorityClasses  *ResourceStore
	limitRanges      *ResourceStore
	namespacedStores []*ResourceStore
}

func (*k8flareAdmission) Handles(op admission.Operation) bool { return op == admission.Create }

func (a *k8flareAdmission) Admit(ctx context.Context, attrs admission.Attributes, _ admission.ObjectInterfaces) error {
	if attrs.GetOperation() != admission.Create || attrs.GetSubresource() != "" {
		return nil
	}
	obj := attrs.GetObject()
	if obj == nil {
		return nil
	}
	namespace := attrs.GetNamespace()

	// Upstream's NamespaceLifecycle plugin: a namespaced object needs its
	// namespace to exist. Without it, anything can be created into a deleted
	// namespace -- seen live 2026-07-25 as KCM resurrecting Events into a
	// namespace kubectl had just swept, leaving rows for a 404 namespace.
	// The Namespace object is kept because compute-class routing reads its
	// labels, so this stays the single namespace read on the create path.
	var ns *corev1.Namespace
	if namespace != "" && attrs.GetResource().Resource != "namespaces" && a.namespaces != nil {
		found, err := a.namespaces.Get(ctx, "", namespace)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return apierrors.NewNotFound(corev1.Resource("namespaces"), namespace)
			}
			return err
		}
		ns, _ = found.(*corev1.Namespace)
	}

	ApplyDefaults(obj)

	if msg := RejectCreateWithTerminatingController(ctx, a.namespacedStores, namespace, obj); msg != "" {
		return admission.NewForbidden(attrs, errors.New(msg))
	}

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	if a.priorityClasses != nil {
		if err := ResolvePodPriority(ctx, a.priorityClasses, pod); err != nil {
			return admission.NewForbidden(attrs, err)
		}
	}

	var nsLabels map[string]string
	if ns != nil {
		nsLabels = ns.Labels
	}
	wantsContainers := PodWantsContainers(pod, nsLabels)
	if wantsContainers {
		MutatePodForComputeClass(pod)
	}

	if a.limitRanges != nil {
		if list, err := a.limitRanges.List(ctx, namespace, "", ""); err == nil {
			ranges := list.(*corev1.LimitRangeList).Items
			ApplyLimitRangeDefaults(pod, ranges)
			if err := ValidateLimitRange(pod, ranges); err != nil {
				return admission.NewForbidden(attrs, err)
			}
		}
	}

	if wantsContainers {
		if err := AssignContainersNode(pod); err != nil {
			return admission.NewForbidden(attrs, err)
		}
	}
	return nil
}
