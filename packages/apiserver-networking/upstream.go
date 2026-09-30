package networking

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/networking"
	"k8s.io/kubernetes/pkg/registry/networking/ingress"
	"k8s.io/kubernetes/pkg/registry/networking/ingressclass"
	"k8s.io/kubernetes/pkg/registry/networking/ipaddress"
	"k8s.io/kubernetes/pkg/registry/networking/networkpolicy"
	"k8s.io/kubernetes/pkg/registry/networking/servicecidr"
)

func init() {
	utilruntime.Must(networking.AddToScheme(registry.InternalScheme))
	utilruntime.Must(networkingv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "networking.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("ingressclasses")] = registry.Upstream{Strategy: ingressclass.Strategy}
	registry.Upstreams[group("ingresses")] = registry.Upstream{Strategy: ingress.Strategy, Status: ingress.StatusStrategy}
	registry.Upstreams[group("ipaddresses")] = registry.Upstream{Strategy: ipaddress.Strategy}
	registry.Upstreams[group("networkpolicies")] = registry.Upstream{Strategy: networkpolicy.Strategy}
	registry.Upstreams[group("servicecidrs")] = registry.Upstream{Strategy: servicecidr.Strategy, Status: servicecidr.StatusStrategy}
}
