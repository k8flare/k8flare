package registrytest

import (
	"context"
	"strings"
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

func Store(t *testing.T, gv schema.GroupVersion, res metav1.APIResource) *registry.Store {
	t.Helper()
	store, err := registry.NewStore(nil, gv, res)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func Context(store *registry.Store) context.Context {
	namespace := ""
	if store.CreateStrategy.NamespaceScoped() {
		namespace = "default"
	}
	return genericapirequest.WithNamespace(context.Background(), namespace)
}

func Create(store *registry.Store, obj runtime.Object) error {
	rest.FillObjectMetaSystemFields(mustAccessor(obj))
	return rest.BeforeCreate(store.CreateStrategy, Context(store), obj)
}

func Update(store *registry.Store, obj, old runtime.Object) error {
	store.UpdateStrategy.PrepareForUpdate(Context(store), obj, old)
	return rest.BeforeUpdate(store.UpdateStrategy, Context(store), obj, old)
}

func RequireFieldError(t *testing.T, err error, field, contains string) {
	t.Helper()
	status, ok := err.(apierrors.APIStatus)
	if !ok || !apierrors.IsInvalid(err) {
		t.Fatalf("want Invalid, got %v", err)
	}
	for _, cause := range status.Status().Details.Causes {
		if cause.Field == field && strings.Contains(cause.Message, contains) {
			return
		}
	}
	t.Fatalf("want %s error containing %q, got %v", field, contains, err)
}

func mustAccessor(obj runtime.Object) metav1.Object {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		panic(err)
	}
	return accessor
}
