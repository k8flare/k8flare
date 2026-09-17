package core

import (
	"context"
	"fmt"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	storageerr "k8s.io/apiserver/pkg/storage/errors"
	"k8s.io/apiserver/pkg/util/dryrun"
)

var namespaceResource = corev1.Resource("namespaces")

func shouldDeleteNamespaceDuringUpdate(ctx context.Context, key string, obj, existing runtime.Object) bool {
	return len(obj.(*corev1.Namespace).Spec.Finalizers) == 0 && genericregistry.ShouldDeleteDuringUpdate(ctx, key, obj, existing)
}

type namespaceDeleter struct{ store *registry.Store }

func (d namespaceDeleter) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	obj, err := d.store.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}
	namespace := obj.(*corev1.Namespace)
	if options == nil {
		options = metav1.NewDeleteOptions(0)
	}
	if options.Preconditions == nil {
		options.Preconditions = &metav1.Preconditions{}
	}
	if options.Preconditions.UID == nil {
		options.Preconditions.UID = &namespace.UID
	} else if *options.Preconditions.UID != namespace.UID {
		return nil, false, apierrors.NewConflict(namespaceResource, name, fmt.Errorf("Precondition failed: UID in precondition: %v, UID in object meta: %v", *options.Preconditions.UID, namespace.UID))
	}
	if options.Preconditions.ResourceVersion != nil && *options.Preconditions.ResourceVersion != namespace.ResourceVersion {
		return nil, false, apierrors.NewConflict(namespaceResource, name, fmt.Errorf("Precondition failed: ResourceVersion in precondition: %v, ResourceVersion in object meta: %v", *options.Preconditions.ResourceVersion, namespace.ResourceVersion))
	}
	if namespace.DeletionTimestamp.IsZero() {
		return d.beginTermination(ctx, name, deleteValidation, options)
	}
	if len(namespace.Spec.Finalizers) != 0 {
		return namespace, false, nil
	}
	return d.store.Delete(ctx, name, deleteValidation, options)
}

func (d namespaceDeleter) beginTermination(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	key, err := d.store.KeyFunc(ctx, name)
	if err != nil {
		return nil, false, err
	}
	preconditions := storage.Preconditions{UID: options.Preconditions.UID, ResourceVersion: options.Preconditions.ResourceVersion}
	out := d.store.NewFunc()
	err = d.store.Storage.GuaranteedUpdate(ctx, key, out, false, &preconditions, storage.SimpleUpdate(func(existing runtime.Object) (runtime.Object, error) {
		namespace := existing.(*corev1.Namespace)
		if err := deleteValidation(ctx, namespace); err != nil {
			return nil, err
		}
		if namespace.DeletionTimestamp.IsZero() {
			now := metav1.Now()
			namespace.DeletionTimestamp = &now
		}
		namespace.Status.Phase = corev1.NamespaceTerminating
		namespace.Finalizers = dependentsFinalizers(options, namespace.Finalizers)
		return namespace, nil
	}), dryrun.IsDryRun(options.DryRun), nil)
	if err != nil {
		err = storageerr.InterpretGetError(err, namespaceResource, name)
		err = storageerr.InterpretUpdateError(err, namespaceResource, name)
		if _, ok := err.(*apierrors.StatusError); !ok {
			err = apierrors.NewInternalError(err)
		}
		return nil, false, err
	}
	return out, false, nil
}

func dependentsFinalizers(options *metav1.DeleteOptions, current []string) []string {
	have := map[string]bool{}
	for _, f := range current {
		have[f] = true
	}
	orphan, deleteDependents := have[metav1.FinalizerOrphanDependents], have[metav1.FinalizerDeleteDependents]
	if options.OrphanDependents != nil {
		orphan, deleteDependents = *options.OrphanDependents, !*options.OrphanDependents
	} else if options.PropagationPolicy != nil {
		orphan, deleteDependents = *options.PropagationPolicy == metav1.DeletePropagationOrphan, *options.PropagationPolicy == metav1.DeletePropagationForeground
	}
	have[metav1.FinalizerOrphanDependents], have[metav1.FinalizerDeleteDependents] = orphan, deleteDependents
	finalizers := []string{}
	for f, keep := range have {
		if keep {
			finalizers = append(finalizers, f)
		}
	}
	return finalizers
}

type namespaceCreateStrategy struct{ rest.RESTCreateStrategy }

func (s namespaceCreateStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	s.RESTCreateStrategy.PrepareForCreate(ctx, obj)
	namespace := obj.(*corev1.Namespace)
	namespace.Status = corev1.NamespaceStatus{Phase: corev1.NamespaceActive}
	for _, f := range namespace.Spec.Finalizers {
		if f == corev1.FinalizerKubernetes {
			return
		}
	}
	namespace.Spec.Finalizers = append(namespace.Spec.Finalizers, corev1.FinalizerKubernetes)
}

type namespaceUpdateStrategy struct{ rest.RESTUpdateStrategy }

func (s namespaceUpdateStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	obj.(*corev1.Namespace).Spec.Finalizers = old.(*corev1.Namespace).Spec.Finalizers
	obj.(*corev1.Namespace).Status = old.(*corev1.Namespace).Status
}

type namespaceFinalizeStrategy struct{ rest.RESTUpdateStrategy }

func (namespaceFinalizeStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	obj.(*corev1.Namespace).Status = old.(*corev1.Namespace).Status
}
