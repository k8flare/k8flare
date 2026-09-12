package apiserver

import (
	"context"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

// scaleREST serves a resource's /scale subresource as an autoscaling/v1
// Scale over the resource's own store, which is what upstream's per-resource
// ScaleRESTs do. Upstream's cannot be reused: they are written against the
// internal API versions, and this apiserver registers external versions only.
//
// The two conversions are the generic reflective ones subresource.go already
// used, so a resource acquires a working /scale by declaring it in
// apidef.Table and having a spec.replicas.
type scaleREST struct {
	store *registry.Store
}

func newScaleREST(parent *registry.Store) *scaleREST { return &scaleREST{store: parent} }

func (r *scaleREST) New() runtime.Object { return &autoscalingv1.Scale{} }

func (r *scaleREST) Destroy() {}

// GroupVersionKind tells the installer this subresource answers in a
// different group than the resource containing it.
func (r *scaleREST) GroupVersionKind(schema.GroupVersion) schema.GroupVersionKind {
	return autoscalingv1.SchemeGroupVersion.WithKind("Scale")
}

func (r *scaleREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	obj, err := r.store.Get(ctx, name, options)
	if err != nil {
		return nil, err
	}
	return scaleFromObject(obj)
}

func (r *scaleREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	current, err := r.store.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}
	currentScale, err := scaleFromObject(current)
	if err != nil {
		return nil, false, err
	}
	requested, err := objInfo.UpdatedObject(ctx, currentScale)
	if err != nil {
		return nil, false, err
	}
	scale, ok := requested.(*autoscalingv1.Scale)
	if !ok {
		return nil, false, apierrors.NewBadRequest("scale update body is not an autoscaling/v1 Scale")
	}

	updated, created, err := r.store.Update(ctx, name, rest.DefaultUpdatedObjectInfo(nil,
		func(_ context.Context, _, oldObj runtime.Object) (runtime.Object, error) {
			next := oldObj.DeepCopyObject()
			if err := applyReplicasToObject(next, scale.Spec.Replicas); err != nil {
				return nil, err
			}
			return next, nil
		}), createValidation, updateValidation, false, options)
	if err != nil {
		return nil, false, err
	}
	out, err := scaleFromObject(updated)
	if err != nil {
		return nil, false, err
	}
	return out, created, nil
}

func (r *scaleREST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.store.ConvertToTable(ctx, object, tableOptions)
}
