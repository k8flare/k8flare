package registry

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

// statusREST serves /status the way upstream does: a second rest.Storage
// over a copy of the store whose update strategy lets a write touch only
// the status.
type statusREST struct {
	store *genericregistry.Store
}

// NewStatusREST serves /status over parent.
func NewStatusREST(parent *genericregistry.Store) *statusREST {
	statusStore := *parent
	statusStore.UpdateStrategy = statusOnlyStrategy{parent.UpdateStrategy}
	return &statusREST{store: &statusStore}
}

func (r *statusREST) New() runtime.Object { return r.store.New() }
func (r *statusREST) Destroy()            {}

func (r *statusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

func (r *statusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, _ bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}

func (r *statusREST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.store.ConvertToTable(ctx, object, tableOptions)
}

type statusOnlyStrategy struct {
	rest.RESTUpdateStrategy
}

func (statusOnlyStrategy) PrepareForUpdate(_ context.Context, obj, old runtime.Object) {
	newVal := reflect.ValueOf(obj).Elem()
	oldVal := reflect.ValueOf(old).Elem()
	if newVal.Type() != oldVal.Type() || newVal.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < newVal.NumField(); i++ {
		switch newVal.Type().Field(i).Name {
		case "Status", "ObjectMeta":
			continue
		}
		if newVal.Field(i).CanSet() {
			newVal.Field(i).Set(oldVal.Field(i))
		}
	}
	newMeta, newErr := meta.Accessor(obj)
	oldMeta, oldErr := meta.Accessor(old)
	if newErr != nil || oldErr != nil {
		return
	}
	newMeta.SetOwnerReferences(oldMeta.GetOwnerReferences())
	newMeta.SetDeletionTimestamp(oldMeta.GetDeletionTimestamp())
	newMeta.SetDeletionGracePeriodSeconds(oldMeta.GetDeletionGracePeriodSeconds())
	newMeta.SetFinalizers(oldMeta.GetFinalizers())
}
