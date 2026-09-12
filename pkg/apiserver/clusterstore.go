package apiserver

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

// protectedClusterStore refuses to delete the management Cluster, and leaves
// it behind when the collection is deleted rather than refusing the whole
// collection -- deleting every other cluster is a legitimate request.
//
// A decorator rather than admission: a collection delete reaches admission
// once, with no name, so admission can only accept or refuse the lot. The
// store is where the per-object decision belongs, and it is also where
// upstream puts resource-specific delete policy.
type protectedClusterStore struct {
	*registry.Store
}

func (s protectedClusterStore) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	if name == protectedClusterName {
		return nil, false, apierrors.NewForbidden(
			s.DefaultQualifiedResource, name,
			errManagementClusterUndeletable)
	}
	return s.Store.Delete(ctx, name, deleteValidation, options)
}

func (s protectedClusterStore) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *metainternalversion.ListOptions) (runtime.Object, error) {
	narrowed := listOptions
	if narrowed == nil {
		narrowed = &metainternalversion.ListOptions{}
	}
	copied := *narrowed
	exclude := fields.OneTermNotEqualSelector("metadata.name", protectedClusterName)
	if copied.FieldSelector == nil {
		copied.FieldSelector = exclude
	} else {
		copied.FieldSelector = fields.AndSelectors(copied.FieldSelector, exclude)
	}
	return s.Store.DeleteCollection(ctx, deleteValidation, options, &copied)
}
