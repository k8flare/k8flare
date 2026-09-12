package apiserver

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

// statusREST serves a resource's /status subresource from the same store as
// the resource itself, which is what upstream does for every resource that
// has one -- a second rest.Storage over a copy of the store whose update
// strategy refuses to let a status write touch anything but the status.
//
// Generic rather than one per resource, the same way subresource.go's
// copyStatus already is: the field is found by name, so a resource acquires
// a working /status by declaring it in apidef.Table and nothing else.
type statusREST struct {
	store *registry.Store
}

func newStatusREST(parent *registry.Store) *statusREST {
	statusStore := *parent
	statusStore.UpdateStrategy = statusOnlyStrategy{parent.UpdateStrategy}
	return &statusREST{store: &statusStore}
}

func (r *statusREST) New() runtime.Object { return r.store.New() }

func (r *statusREST) Destroy() {}

func (r *statusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

func (r *statusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}

func (r *statusREST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.store.ConvertToTable(ctx, object, tableOptions)
}

// statusOnlyStrategy restores every field except Status and ObjectMeta from
// the stored object, then puts back the two pieces of metadata a status
// write must not move. This is upstream's shape (`new.Spec = old.Spec` plus
// OwnerReferences and DeletionTimestamp), generalised by field name.
//
// ObjectMeta is deliberately NOT restored wholesale. The request's
// resourceVersion is the optimistic-concurrency precondition Store.Update
// checks; overwriting it with the stored object's would make a stale status
// write succeed instead of conflicting, and a kubelet or controller that
// depends on that 409 would overwrite a newer status with an older one. The
// hand-written handler never had to think about this because it read the
// object fresh and wrote only Status -- the revision came from the store,
// not from the request.
type statusOnlyStrategy struct {
	rest.RESTUpdateStrategy
}

func (statusOnlyStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
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
}
