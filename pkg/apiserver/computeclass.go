package apiserver

import (
	corev1 "k8s.io/api/core/v1"
)

// Compute-class routing for the Pod-on-Containers backend
// (workers/nodes): a Pod opts in with a single annotation, and this
// apiserver injects the scheduling constraints at admission (the role a
// mutating webhook plays on managed per-Pod-node platforms). The virtual
// node registers with a `k8flare.com/pod-on-containers=true:NoSchedule`
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
	ComputeClassAnnotation = "k8flare.com/compute"
	// ComputeClassContainers routes the Pod to the Cloudflare Containers
	// virtual node pool.
	ComputeClassContainers = "containers"

	containersBackendLabel = "k8flare.com/backend"
	containersTaintKey     = "k8flare.com/pod-on-containers"

	// ContainersSchedulerName is the schedulerName of workers/nodes'
	// per-Pod binder (binds via the official Binding
	// subresource; see the design in the repo task/docs). The real
	// kube-scheduler never picks these pods up, and the binder never
	// touches default-scheduler pods.
	ContainersSchedulerName = "cf-containers-scheduler"
)

// PodWantsContainers resolves the effective compute class with
// namespace-first precedence (user decision 2026-07-06, mirroring how
// managed per-Pod-node platforms and Istio's namespace-scoped injection
// behave): an explicit pod annotation always wins (both to opt IN from a
// plain namespace and to opt OUT -- any value other than "containers" --
// from a containers namespace); otherwise the namespace LABEL
// k8flare.com/compute=containers routes every pod in it. A label, not an
// annotation, on the namespace because this is selection semantics
// (`kubectl get ns -l k8flare.com/compute=containers` should work).
// Label changes affect newly created pods only, same as any admission
// mechanism.
func PodWantsContainers(pod *corev1.Pod, nsLabels map[string]string) bool {
	if v, ok := pod.Annotations[ComputeClassAnnotation]; ok {
		return v == ComputeClassContainers
	}
	return nsLabels[ComputeClassAnnotation] == ComputeClassContainers
}

// MutatePodForComputeClass injects the Containers-backend schedulerName,
// nodeSelector and taint toleration. Idempotent; the caller decides
// applicability via PodWantsContainers (handler.go's pod-create path,
// which can see the Namespace object).
func MutatePodForComputeClass(pod *corev1.Pod) {
	// Route to the cf-containers-scheduler binder via Kubernetes' standard
	// multi-scheduler mechanism (spec.schedulerName). Only when the pod
	// didn't explicitly pick a scheduler itself: "default-scheduler" is what admission defaulting
	// fills in for an unset field, so that value counts as unset here.
	if pod.Spec.SchedulerName == "" || pod.Spec.SchedulerName == corev1.DefaultSchedulerName {
		pod.Spec.SchedulerName = ContainersSchedulerName
	}
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = map[string]string{}
	}
	pod.Spec.NodeSelector[containersBackendLabel] = ComputeClassContainers
	hasToleration := false
	for _, t := range pod.Spec.Tolerations {
		if t.Key == containersTaintKey {
			hasToleration = true
			break
		}
	}
	if !hasToleration {
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
			Key:      containersTaintKey,
			Operator: corev1.TolerationOpEqual,
			Value:    "true",
			Effect:   corev1.TaintEffectNoSchedule,
		})
	}
	// hostNetwork: each pod gets a dedicated microVM node, so the host
	// network namespace IS the pod's -- there is no cross-pod port or
	// isolation concern, no CNI sandbox to set up (the microVM kernel has
	// no netfilter, which stalled pods in ContainerCreating), and the
	// pod's ports are directly reachable via containerFetch. dnsPolicy is
	// left alone: kubelet treats ClusterFirst as Default for hostNetwork
	// pods, so pods resolve through the VM's resolv.conf until the
	// Worker-side cluster-DNS synthesizer lands.
	pod.Spec.HostNetwork = true
}
