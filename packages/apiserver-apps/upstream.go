package apps

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/apps"
	"k8s.io/kubernetes/pkg/registry/apps/controllerrevision"
	"k8s.io/kubernetes/pkg/registry/apps/daemonset"
	"k8s.io/kubernetes/pkg/registry/apps/deployment"
	"k8s.io/kubernetes/pkg/registry/apps/replicaset"
	"k8s.io/kubernetes/pkg/registry/apps/statefulset"
)

func init() {
	utilruntime.Must(apps.AddToScheme(registry.InternalScheme))
	utilruntime.Must(appsv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "apps", Resource: resource}
	}
	registry.Upstreams[group("controllerrevisions")] = registry.Upstream{Strategy: controllerrevision.Strategy}
	registry.Upstreams[group("daemonsets")] = registry.Upstream{Strategy: daemonset.Strategy, Status: daemonset.StatusStrategy}
	registry.Upstreams[group("deployments")] = registry.Upstream{Strategy: deployment.Strategy, Status: deployment.StatusStrategy}
	registry.Upstreams[group("replicasets")] = registry.Upstream{Strategy: replicaset.Strategy, Status: replicaset.StatusStrategy}
	registry.Upstreams[group("statefulsets")] = registry.Upstream{Strategy: statefulset.Strategy, Status: statefulset.StatusStrategy}
}
