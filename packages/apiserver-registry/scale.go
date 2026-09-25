package registry

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

func init() {
	utilruntime.Must(autoscalingv1.AddToScheme(scheme.Scheme))
	scheme.Scheme.AddKnownTypes(appsv1.SchemeGroupVersion, &autoscalingv1.Scale{})
	scheme.Scheme.AddKnownTypes(corev1.SchemeGroupVersion, &autoscalingv1.Scale{})
	scheme.Scheme.AddKnownTypes(batchv1.SchemeGroupVersion, &autoscalingv1.Scale{})
}

type scaleREST struct {
	parent *Store
}

var (
	_ rest.Patcher                  = (*scaleREST)(nil)
	_ rest.Getter                   = (*scaleREST)(nil)
	_ rest.Updater                  = (*scaleREST)(nil)
	_ rest.GroupVersionKindProvider = (*scaleREST)(nil)
	_ rest.Scoper                   = (*scaleREST)(nil)
	_ rest.SingularNameProvider     = (*scaleREST)(nil)
)

func NewScaleREST(parent *Store) rest.Storage {
	return &scaleREST{parent: parent}
}

func (scaleREST) New() runtime.Object   { return &autoscalingv1.Scale{} }
func (scaleREST) Destroy()              {}
func (scaleREST) NamespaceScoped() bool { return true }
func (scaleREST) GetSingularName() string {
	return "scale"
}

func (scaleREST) GroupVersionKind(schema.GroupVersion) schema.GroupVersionKind {
	return autoscalingv1.SchemeGroupVersion.WithKind("Scale")
}

func (r *scaleREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	obj, err := r.parent.Get(ctx, name, options)
	if err != nil {
		return nil, err
	}
	return scaleFrom(obj)
}

func (r *scaleREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, _ bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	obj, _, err := r.parent.Update(ctx, name, rest.DefaultUpdatedObjectInfo(nil, func(ctx context.Context, _, old runtime.Object) (runtime.Object, error) {
		oldScale, err := scaleFrom(old)
		if err != nil {
			return nil, err
		}
		updated, err := objInfo.UpdatedObject(ctx, oldScale)
		if err != nil {
			return nil, err
		}
		scale, ok := updated.(*autoscalingv1.Scale)
		if !ok {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("not a Scale object: %T", updated))
		}
		if updateValidation != nil {
			if err := updateValidation(ctx, scale.DeepCopyObject(), oldScale); err != nil {
				return nil, err
			}
		}
		out := old.DeepCopyObject()
		if err := applyScale(out, scale); err != nil {
			return nil, err
		}
		return out, nil
	}), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, options)
	if err != nil {
		return nil, false, err
	}
	scale, err := scaleFrom(obj)
	return scale, false, err
}

func scaleFrom(obj runtime.Object) (*autoscalingv1.Scale, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	replicas, statusReplicas, selector, err := scaleFields(obj)
	if err != nil {
		return nil, err
	}
	return &autoscalingv1.Scale{
		TypeMeta: metav1.TypeMeta{APIVersion: autoscalingv1.SchemeGroupVersion.String(), Kind: "Scale"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              acc.GetName(),
			Namespace:         acc.GetNamespace(),
			UID:               acc.GetUID(),
			ResourceVersion:   acc.GetResourceVersion(),
			CreationTimestamp: acc.GetCreationTimestamp(),
		},
		Spec:   autoscalingv1.ScaleSpec{Replicas: replicas},
		Status: autoscalingv1.ScaleStatus{Replicas: statusReplicas, Selector: selector},
	}, nil
}

func applyScale(obj runtime.Object, scale *autoscalingv1.Scale) error {
	replicas := scale.Spec.Replicas
	switch o := obj.(type) {
	case *appsv1.Deployment:
		o.Spec.Replicas = &replicas
	case *appsv1.ReplicaSet:
		o.Spec.Replicas = &replicas
	case *appsv1.StatefulSet:
		o.Spec.Replicas = &replicas
	case *corev1.ReplicationController:
		o.Spec.Replicas = &replicas
	case *batchv1.Job:
		o.Spec.Parallelism = &replicas
	default:
		return apierrors.NewBadRequest(fmt.Sprintf("cannot scale %T", obj))
	}
	return nil
}

func scaleFields(obj runtime.Object) (replicas, statusReplicas int32, selector string, err error) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return derefReplicas(o.Spec.Replicas), o.Status.Replicas, formatLabelSelector(o.Spec.Selector), nil
	case *appsv1.ReplicaSet:
		return derefReplicas(o.Spec.Replicas), o.Status.Replicas, formatLabelSelector(o.Spec.Selector), nil
	case *appsv1.StatefulSet:
		return derefReplicas(o.Spec.Replicas), o.Status.Replicas, formatLabelSelector(o.Spec.Selector), nil
	case *corev1.ReplicationController:
		return derefReplicas(o.Spec.Replicas), o.Status.Replicas, labels.Set(o.Spec.Selector).String(), nil
	case *batchv1.Job:
		return derefReplicas(o.Spec.Parallelism), o.Status.Active, formatLabelSelector(o.Spec.Selector), nil
	default:
		return 0, 0, "", apierrors.NewBadRequest(fmt.Sprintf("cannot scale %T", obj))
	}
}

func derefReplicas(n *int32) int32 {
	if n == nil {
		return 1
	}
	return *n
}

func formatLabelSelector(sel *metav1.LabelSelector) string {
	if sel == nil {
		return ""
	}
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return ""
	}
	return s.String()
}
