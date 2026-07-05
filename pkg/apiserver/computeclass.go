package apiserver

import (
	corev1 "k8s.io/api/core/v1"
)

// EKS-on-Fargate-style compute-class routing for the Pod-on-Containers
// backend (workers/nodes): a Pod opts in with a single annotation, and
// this apiserver -- playing the role EKS's Fargate mutating webhook
// plays -- injects the scheduling constraints at admission. The virtual
// node registers with a `k8flare.dev/pod-on-containers=true:NoSchedule`
// taint (workers/nodes/src/virtualnode.ts), so un-annotated Pods can
// never land on the Containers backend (image allowlist, no UDP, no
// exec -- see README.md), and annotated Pods schedule ONLY there.
//
// Users set the annotation on the Pod template
// (spec.template.metadata.annotations for a Deployment); the real
// ReplicaSet controller copies template metadata onto the Pods it
// creates, so the mutation fires when those Pods reach Create here.
const (
	// ComputeClassAnnotation is the opt-in annotation key.
	ComputeClassAnnotation = "k8flare.dev/compute"
	// ComputeClassContainers routes the Pod to the Cloudflare Containers
	// virtual node pool.
	ComputeClassContainers = "containers"

	containersBackendLabel = "k8flare.dev/backend"
	containersTaintKey     = "k8flare.dev/pod-on-containers"

	// FargateSchedulerName is the schedulerName of workers/nodes'
	// fargate-style per-Pod binder (binds via the official Binding
	// subresource; see the design in the repo task/docs). The real
	// kube-scheduler never picks these pods up, and the binder never
	// touches default-scheduler pods.
	FargateSchedulerName = "k8flare-fargate"
)

// MutatePodForComputeClass injects the Containers-backend nodeSelector
// and taint toleration into pod iff it carries
// `k8flare.dev/compute: containers`. Idempotent; called from
// ResourceStore.Create, mirroring prepareJobForCreate's placement.
func MutatePodForComputeClass(pod *corev1.Pod) {
	if pod.Annotations[ComputeClassAnnotation] != ComputeClassContainers {
		return
	}
	// Route to the fargate-style binder via Kubernetes' standard
	// multi-scheduler mechanism (spec.schedulerName), exactly like EKS's
	// Fargate webhook does. Only when the pod didn't explicitly pick a
	// scheduler itself: "default-scheduler" is what admission defaulting
	// fills in for an unset field, so that value counts as unset here.
	if pod.Spec.SchedulerName == "" || pod.Spec.SchedulerName == corev1.DefaultSchedulerName {
		pod.Spec.SchedulerName = FargateSchedulerName
	}
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = map[string]string{}
	}
	pod.Spec.NodeSelector[containersBackendLabel] = ComputeClassContainers
	for _, t := range pod.Spec.Tolerations {
		if t.Key == containersTaintKey {
			return
		}
	}
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
		Key:      containersTaintKey,
		Operator: corev1.TolerationOpEqual,
		Value:    "true",
		Effect:   corev1.TaintEffectNoSchedule,
	})
}
