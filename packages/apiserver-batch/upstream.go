package batch

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/registry/batch/cronjob"
	"k8s.io/kubernetes/pkg/registry/batch/job"
)

func init() {
	utilruntime.Must(batch.AddToScheme(registry.InternalScheme))
	utilruntime.Must(batchv1.AddToScheme(registry.InternalScheme))
	group := func(resource string) schema.GroupResource {
		return schema.GroupResource{Group: "batch", Resource: resource}
	}
	registry.Upstreams[group("cronjobs")] = registry.Upstream{Strategy: cronjob.Strategy, Status: cronjob.StatusStrategy}
	registry.Upstreams[group("jobs")] = registry.Upstream{Strategy: job.Strategy, Status: job.StatusStrategy}
}
