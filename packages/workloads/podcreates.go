package workloads

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/flowcontrol"
)

const (
	podCreatesPerSecond = 20.0
	podCreateBurst      = 30
)

var podCreates = newPodCreateLimiter()

func newPodCreateLimiter() flowcontrol.RateLimiter {
	return flowcontrol.NewTokenBucketRateLimiter(podCreatesPerSecond, podCreateBurst)
}

func (c *observeClient) controllerSnapshot(kind schema.GroupKind) (*snapshotInformer, schema.GroupResource) {
	switch kind {
	case schema.GroupKind{Group: "apps", Kind: "ReplicaSet"}:
		return c.replicaSets, schema.GroupResource{Group: "apps", Resource: "replicasets"}
	case schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
		return c.sets, schema.GroupResource{Group: "apps", Resource: "statefulsets"}
	case schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
		return c.daemonSets, schema.GroupResource{Group: "apps", Resource: "daemonsets"}
	case schema.GroupKind{Group: "batch", Kind: "Job"}:
		return c.jobs, schema.GroupResource{Group: "batch", Resource: "jobs"}
	case schema.GroupKind{Kind: "ReplicationController"}:
		return c.rcs, schema.GroupResource{Resource: "replicationcontrollers"}
	case schema.GroupKind{Group: "apps", Kind: "Deployment"}:
		return c.deployments, schema.GroupResource{Group: "apps", Resource: "deployments"}
	case schema.GroupKind{Group: "batch", Kind: "CronJob"}:
		return c.cronJobs, schema.GroupResource{Group: "batch", Resource: "cronjobs"}
	}
	return nil, schema.GroupResource{}
}

func (c *observeClient) controllerLeftSnapshot(namespace string, owned metav1.Object) error {
	ref := metav1.GetControllerOf(owned)
	if c == nil || ref == nil {
		return nil
	}
	snapshot, resource := c.controllerSnapshot(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind).GroupKind())
	if snapshot == nil {
		return nil
	}
	held, exists, _ := snapshot.SharedIndexInformer.GetIndexer().GetByKey(namespace + "/" + ref.Name)
	if exists {
		if controller, err := meta.Accessor(held); err == nil && controller.GetUID() == ref.UID {
			return c.controllerLeftSnapshot(namespace, controller)
		}
	}
	return apierrors.NewNotFound(resource, ref.Name)
}
