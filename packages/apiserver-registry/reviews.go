package registry

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

type reviewREST struct {
	gvk        schema.GroupVersionKind
	singular   string
	namespaced bool
	create     func(ctx context.Context, obj runtime.Object) runtime.Object
}

var _ rest.Creater = (*reviewREST)(nil)

func Review(create func(deps Deps) func(context.Context, runtime.Object) runtime.Object) func(schema.GroupVersion, metav1.APIResource, Deps) rest.Storage {
	return func(gv schema.GroupVersion, res metav1.APIResource, deps Deps) rest.Storage {
		return &reviewREST{gvk: gv.WithKind(res.Kind), singular: res.SingularName, namespaced: res.Namespaced, create: create(deps)}
	}
}

func (r *reviewREST) New() runtime.Object {
	obj, _ := scheme.Scheme.New(r.gvk)
	return obj
}
func (r *reviewREST) Destroy()                {}
func (r *reviewREST) NamespaceScoped() bool   { return r.namespaced }
func (r *reviewREST) GetSingularName() string { return r.singular }
func (r *reviewREST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	return r.create(ctx, obj), nil
}
