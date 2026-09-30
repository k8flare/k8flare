package admissionregistration

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/admissionregistration"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/mutatingwebhookconfiguration"
	"k8s.io/kubernetes/pkg/registry/admissionregistration/validatingwebhookconfiguration"
)

func init() {
	utilruntime.Must(admissionregistration.AddToScheme(registry.InternalScheme))
	utilruntime.Must(admissionregistrationv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "admissionregistration.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("mutatingwebhookconfigurations")] = registry.Upstream{Strategy: mutatingwebhookconfiguration.Strategy}
	registry.Upstreams[group("validatingwebhookconfigurations")] = registry.Upstream{Strategy: validatingwebhookconfiguration.Strategy}
}
