package flowcontrol

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/flowcontrol"
	"k8s.io/kubernetes/pkg/registry/flowcontrol/flowschema"
	"k8s.io/kubernetes/pkg/registry/flowcontrol/prioritylevelconfiguration"
)

func init() {
	utilruntime.Must(flowcontrol.AddToScheme(registry.InternalScheme))
	utilruntime.Must(flowcontrolv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "flowcontrol.apiserver.k8s.io", Resource: resource}
	}
	registry.Upstreams[group("flowschemas")] = registry.Upstream{Strategy: flowschema.Strategy, Status: flowschema.StatusStrategy}
	registry.Upstreams[group("prioritylevelconfigurations")] = registry.Upstream{Strategy: prioritylevelconfiguration.Strategy, Status: prioritylevelconfiguration.StatusStrategy}
}
