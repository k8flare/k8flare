package admissionregistration

import (
	"errors"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/admissionregistration"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/mutatingadmissionpolicy"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/mutatingadmissionpolicybinding"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/mutatingwebhookconfiguration"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/resolver"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/validatingadmissionpolicy"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/validatingadmissionpolicybinding"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/validatingwebhookconfiguration"
)

func init() {
	utilruntime.Must(admissionregistration.AddToScheme(registry.InternalScheme))
	utilruntime.Must(admissionregv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "admissionregistration.k8s.io", Resource: resource}
	}
	unresolved := resolver.ResourceResolverFunc(func(schema.GroupVersionKind) (schema.GroupVersionResource, error) {
		return schema.GroupVersionResource{}, errors.New("resource resolution is not available")
	})
	policy := validatingadmissionpolicy.NewStrategy(nil, unresolved)
	registry.Upstreams[group("mutatingwebhookconfigurations")] = registry.Upstream{Strategy: mutatingwebhookconfiguration.Strategy}
	registry.Upstreams[group("validatingwebhookconfigurations")] = registry.Upstream{Strategy: validatingwebhookconfiguration.Strategy}
	registry.Upstreams[group("validatingadmissionpolicies")] = registry.Upstream{Strategy: policy, Status: validatingadmissionpolicy.NewStatusStrategy(policy)}
	registry.Upstreams[group("validatingadmissionpolicybindings")] = registry.Upstream{Strategy: validatingadmissionpolicybinding.NewStrategy(nil, nil, unresolved)}
	registry.Upstreams[group("mutatingadmissionpolicies")] = registry.Upstream{Strategy: mutatingadmissionpolicy.NewStrategy(nil, unresolved)}
	registry.Upstreams[group("mutatingadmissionpolicybindings")] = registry.Upstream{Strategy: mutatingadmissionpolicybinding.NewStrategy(nil, nil, unresolved)}
}
