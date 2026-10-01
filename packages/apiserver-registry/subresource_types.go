package registry

import (
	"context"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

func init() {
	utilruntime.Must(policyv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(authenticationv1.AddToScheme(scheme.Scheme))
	scheme.Scheme.AddKnownTypes(corev1.SchemeGroupVersion, &policyv1.Eviction{}, &authenticationv1.TokenRequest{})
	for _, name := range []string{"deployments", "replicasets", "statefulsets", "replicationcontrollers"} {
		parent := name
		Subresources[parent+"/scale"] = func(stores map[string]*Store, _ Deps) rest.Storage {
			return NewScaleREST(stores[parent])
		}
	}
	Subresources["pods/eviction"] = func(map[string]*Store, Deps) rest.Storage {
		return typedNamedCreateREST{
			obj: &policyv1.Eviction{},
			gvk: policyv1.SchemeGroupVersion.WithKind("Eviction"),
		}
	}
	Subresources["serviceaccounts/token"] = func(map[string]*Store, Deps) rest.Storage {
		return typedNamedCreateREST{
			obj: &authenticationv1.TokenRequest{},
			gvk: authenticationv1.SchemeGroupVersion.WithKind("TokenRequest"),
		}
	}
}

type typedNamedCreateREST struct {
	obj runtime.Object
	gvk schema.GroupVersionKind
}

var (
	_ rest.NamedCreater             = typedNamedCreateREST{}
	_ rest.GroupVersionKindProvider = typedNamedCreateREST{}
	_ rest.Scoper                   = typedNamedCreateREST{}
	_ rest.SingularNameProvider     = typedNamedCreateREST{}
)

func (t typedNamedCreateREST) New() runtime.Object { return t.obj.DeepCopyObject() }
func (typedNamedCreateREST) Destroy()              {}
func (typedNamedCreateREST) NamespaceScoped() bool { return true }
func (t typedNamedCreateREST) GetSingularName() string {
	return t.gvk.Kind
}
func (t typedNamedCreateREST) GroupVersionKind(schema.GroupVersion) schema.GroupVersionKind {
	return t.gvk
}
func (typedNamedCreateREST) Create(context.Context, string, runtime.Object, rest.ValidateObjectFunc, *metav1.CreateOptions) (runtime.Object, error) {
	return nil, apierrors.NewServiceUnavailable("")
}
