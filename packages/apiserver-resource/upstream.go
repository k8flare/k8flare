package resource

import (
	"context"
	"net/http"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/kubernetes/pkg/apis/resource"
	"k8s.io/kubernetes/pkg/registry/resource/deviceclass"
	"k8s.io/kubernetes/pkg/registry/resource/resourceclaim"
	"k8s.io/kubernetes/pkg/registry/resource/resourceclaimtemplate"
	"k8s.io/kubernetes/pkg/registry/resource/resourceslice"
)

var namespaces storage.Interface

type namespaceReader struct {
	corev1client.NamespaceInterface
}

func (namespaceReader) Get(ctx context.Context, name string, _ metav1.GetOptions) (*corev1.Namespace, error) {
	namespace := &corev1.Namespace{}
	if err := namespaces.Get(ctx, "/namespaces/"+name, storage.GetOptions{}, namespace); err != nil {
		return nil, err
	}
	return namespace, nil
}

func init() {
	utilruntime.Must(resource.AddToScheme(registry.InternalScheme))
	utilruntime.Must(resourcev1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "resource.k8s.io", Resource: resource}
	}
	claim := resourceclaim.NewStrategy(namespaceReader{}, authorizerfactory.NewAlwaysAllowAuthorizer())
	registry.Upstreams[group("deviceclasses")] = registry.Upstream{Strategy: deviceclass.Strategy}
	registry.Upstreams[group("resourceclaims")] = registry.Upstream{Strategy: claim, Status: resourceclaim.NewStatusStrategy(claim)}
	registry.Upstreams[group("resourceclaimtemplates")] = registry.Upstream{Strategy: resourceclaimtemplate.NewStrategy(namespaceReader{})}
	registry.Upstreams[group("resourceslices")] = registry.Upstream{Strategy: resourceslice.Strategy}
	registry.Middleware = append(registry.Middleware, func(map[string]*registry.Store) func(http.Handler) http.Handler {
		return auth.WithRequestInfo
	})
	for _, name := range []string{"resourceclaims", "resourceclaimtemplates"} {
		registry.Customizers[name] = func(_ *registry.Store, deps registry.Deps) {
			namespaces = kine.NewStorage(deps.Kine, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return &corev1.Namespace{} })
		}
	}
}
