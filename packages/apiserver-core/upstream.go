package core

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	apicore "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/registry/core/configmap"
	"k8s.io/kubernetes/pkg/registry/core/endpoint"
	"k8s.io/kubernetes/pkg/registry/core/event"
	"k8s.io/kubernetes/pkg/registry/core/limitrange"
	"k8s.io/kubernetes/pkg/registry/core/namespace"
	"k8s.io/kubernetes/pkg/registry/core/node"
	"k8s.io/kubernetes/pkg/registry/core/persistentvolume"
	"k8s.io/kubernetes/pkg/registry/core/persistentvolumeclaim"
	"k8s.io/kubernetes/pkg/registry/core/pod"
	"k8s.io/kubernetes/pkg/registry/core/podtemplate"
	"k8s.io/kubernetes/pkg/registry/core/replicationcontroller"
	"k8s.io/kubernetes/pkg/registry/core/resourcequota"
	"k8s.io/kubernetes/pkg/registry/core/secret"
	"k8s.io/kubernetes/pkg/registry/core/service"
	"k8s.io/kubernetes/pkg/registry/core/serviceaccount"
)

func init() {
	utilruntime.Must(apicore.AddToScheme(registry.InternalScheme))
	utilruntime.Must(corev1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource { return schema.GroupResource{Resource: resource} }
	registry.Upstreams[group("configmaps")] = registry.Upstream{Strategy: configmap.Strategy}
	registry.Upstreams[group("endpoints")] = registry.Upstream{Strategy: endpoint.Strategy}
	registry.Upstreams[group("events")] = registry.Upstream{Strategy: event.Strategy}
	registry.Upstreams[group("limitranges")] = registry.Upstream{Strategy: limitrange.Strategy}
	registry.Upstreams[group("namespaces")] = registry.Upstream{Strategy: namespace.Strategy, Status: namespace.StatusStrategy}
	registry.Upstreams[group("nodes")] = registry.Upstream{Strategy: node.Strategy, Status: node.StatusStrategy}
	registry.Upstreams[group("persistentvolumeclaims")] = registry.Upstream{Strategy: persistentvolumeclaim.Strategy, Status: persistentvolumeclaim.StatusStrategy}
	registry.Upstreams[group("persistentvolumes")] = registry.Upstream{Strategy: persistentvolume.Strategy, Status: persistentvolume.StatusStrategy}
	registry.Upstreams[group("pods")] = registry.Upstream{Strategy: pod.Strategy, Status: pod.StatusStrategy}
	registry.Upstreams[group("podtemplates")] = registry.Upstream{Strategy: podtemplate.Strategy}
	registry.Upstreams[group("replicationcontrollers")] = registry.Upstream{Strategy: replicationcontroller.Strategy, Status: replicationcontroller.StatusStrategy}
	registry.Upstreams[group("resourcequotas")] = registry.Upstream{Strategy: resourcequota.Strategy, Status: resourcequota.StatusStrategy}
	registry.Upstreams[group("secrets")] = registry.Upstream{Strategy: secret.Strategy}
	registry.Upstreams[group("services")] = registry.Upstream{Strategy: service.Strategy, Status: service.StatusStrategy}
	registry.Upstreams[group("serviceaccounts")] = registry.Upstream{Strategy: serviceaccount.Strategy}
}
