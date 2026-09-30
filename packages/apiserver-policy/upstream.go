package policy

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/policy"
	"k8s.io/kubernetes/pkg/registry/policy/poddisruptionbudget"
)

func init() {
	utilruntime.Must(policy.AddToScheme(registry.InternalScheme))
	utilruntime.Must(policyv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "policy", Resource: resource}
	}
	registry.Upstreams[group("poddisruptionbudgets")] = registry.Upstream{Strategy: poddisruptionbudget.Strategy, Status: poddisruptionbudget.StatusStrategy}
}
