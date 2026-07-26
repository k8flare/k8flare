package apiserver

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/storage/names"
)

// Compute-class routing for the Pod-on-Containers backend
// (workers/nodes): a Pod opts in with a single annotation, and this
// apiserver injects the scheduling constraints at admission (the role a
// mutating webhook plays on managed per-Pod-node platforms). Every
// Pod-on-Containers NodeVM self-registers with a
// `k8flare.com/pod-on-containers=true:NoSchedule` taint (its own real
// kubelet, given `-node-taints` by packages/k8flare-worker/images/node/
// entrypoint.sh), so un-annotated Pods can never land on the Containers
// backend, and annotated Pods schedule ONLY there.
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

	// NodeVMTierAnnotation records the size tier AssignContainersNode
	// picked (see sizeTiers below), for workers/nodes' NodeVM manager
	// (nodes/scheduler.ts) to read when it boots the matching Cloudflare
	// Containers instance -- Go owns the decision, TS just carries it
	// out (the Loader/DO split forces the actual boot call to be TS, see
	// nodes/scheduler.ts's own doc comment).
	NodeVMTierAnnotation = "k8flare.com/nodevm-tier"
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

// MutatePodForComputeClass injects the Containers-backend nodeSelector
// marker and taint toleration. Idempotent; the caller decides
// applicability via PodWantsContainers (handler.go's pod-create path,
// which can see the Namespace object).
//
// Unlike an earlier design, this does NOT set spec.schedulerName: these
// Pods are bound by the real, unmodified kube-scheduler (the sched
// dynamic worker, pkg/controllers/sched) exactly like any other Pod, via
// the standard "default-scheduler" path -- see AssignContainersNode
// below for how a still-nonexistent, dedicated Node is targeted so the
// real scheduler only ever considers it once demand-provisioned and
// Ready. (Verified live 2026-07-10 against wrangler dev: a Node with a
// non-Ready condition and no not-ready taint was NOT excluded by the
// real scheduler's predicates -- readiness gating comes from the
// kubelet's own self-applied `node.kubernetes.io/not-ready` taint at
// registration, not from the Ready condition alone. This repo never
// creates Node objects itself for this backend for exactly that reason:
// every Pod-on-Containers Node is self-registered by its own real
// kubelet (packages/k8flare-worker/images/node/entrypoint.sh), which carries
// that taint machinery for free.)
func MutatePodForComputeClass(pod *corev1.Pod) {
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

// sizeTier is one entry of the fixed (image, instance_type) pairing
// Cloudflare Containers forces on this backend: `instance_type`
// (vCPU/memory/disk) is a `containers[]` entry field fixed at deploy
// time in wrangler.jsonc (no API for a Worker/DO to choose an arbitrary
// size at runtime), so honoring a Pod's resources.requests means
// picking, per Pod, one of a small fixed set of pre-declared tiers
// rather than an arbitrary size -- this rounds a Pod's requests up to
// the nearest tier below. Each tier maps 1:1 to a NodeVM Durable Object
// class (nodes/nodevm.ts) + a durable_objects/containers[] binding in
// wrangler.jsonc; milliCPU/memoryBytes here must track those deploy-time
// instance_type allocations (lite/basic/standard-1 today).
type sizeTier struct {
	name        string
	milliCPU    int64
	memoryBytes int64
}

// Ordered smallest-to-largest so resolveSizeTier can pick the first tier
// that fits.
var sizeTiers = []sizeTier{
	{name: "small", milliCPU: 63, memoryBytes: 256 * 1024 * 1024},        // lite: 1/16 vCPU, 256 MiB
	{name: "medium", milliCPU: 250, memoryBytes: 1 * 1024 * 1024 * 1024}, // basic: 1/4 vCPU, 1 GiB
	{name: "large", milliCPU: 500, memoryBytes: 4 * 1024 * 1024 * 1024},  // standard-1: 1/2 vCPU, 4 GiB
}

// podResourceFootprint sums the per-container effective resource ask
// across the Pod, taking max(limits, requests) per container: limits
// are the ceiling the workload may actually consume, so a Pod sized only
// by its (smaller) requests could be placed on a tier its limits then
// blow through. Standard k8s semantics also guarantee requests <= limits
// when both are set, so this is simply "whichever of the two is
// specified, prefer the ceiling". Uses corev1.ResourceList's own
// Cpu()/Memory() accessors (real upstream helpers, zero-valued when
// absent) rather than hand-parsing quantity strings.
func podResourceFootprint(pod *corev1.Pod) (milliCPU int64, memoryBytes int64) {
	for _, c := range pod.Spec.Containers {
		reqCPU := c.Resources.Requests.Cpu().MilliValue()
		limCPU := c.Resources.Limits.Cpu().MilliValue()
		if limCPU > reqCPU {
			reqCPU = limCPU
		}
		reqMem := c.Resources.Requests.Memory().Value()
		limMem := c.Resources.Limits.Memory().Value()
		if limMem > reqMem {
			reqMem = limMem
		}
		milliCPU += reqCPU
		memoryBytes += reqMem
	}
	return milliCPU, memoryBytes
}

// resolveSizeTier rounds a Pod's total resource requests up to the
// nearest size tier that fits both its CPU and memory ask. Returns
// "small" for a Pod with no requests at all (matches a real cluster's
// behavior of scheduling best-effort Pods onto any node). Returns "" if
// the Pod's request exceeds even the largest tier -- the caller rejects
// the Pod outright rather than silently under-provisioning it.
func resolveSizeTier(milliCPU, memoryBytes int64) string {
	for _, t := range sizeTiers {
		if milliCPU <= t.milliCPU && memoryBytes <= t.memoryBytes {
			return t.name
		}
	}
	return ""
}

// AssignContainersNode picks this Pod's dedicated Cloudflare Containers
// NodeVM: a size tier (from its resource footprint) and a unique,
// not-yet-existing Node name, pinned via the standard
// `kubernetes.io/hostname` nodeSelector so the real kube-scheduler binds
// this Pod to that Node and no other, once it exists and is Ready (see
// MutatePodForComputeClass's doc comment for why nothing here creates
// the Node object itself). The tier is also recorded as an annotation
// (NodeVMTierAnnotation) for nodes/scheduler.ts to read when it boots
// the matching NodeVM class.
//
// Idempotent only in the sense that it no-ops if a hostname pin is
// already present (handler.go calls this once, at Pod create, after
// LimitRange defaulting has filled in any container resources the Pod
// itself omitted -- see the call site's ordering comment). Returns an
// error if the Pod's resources exceed the largest tier; the caller
// rejects the create synchronously (a kubectl-visible 403) rather than
// admitting a Pod this backend can never run.
func AssignContainersNode(pod *corev1.Pod) error {
	if pod.Spec.NodeSelector[corev1.LabelHostname] != "" {
		return nil
	}
	milliCPU, memoryBytes := podResourceFootprint(pod)
	tier := resolveSizeTier(milliCPU, memoryBytes)
	if tier == "" {
		largest := sizeTiers[len(sizeTiers)-1]
		return fmt.Errorf(
			"pod resources (%dm CPU, %d bytes memory) exceed the largest cf-containers size tier %q (%dm CPU, %d bytes memory)",
			milliCPU, memoryBytes, largest.name, largest.milliCPU, largest.memoryBytes,
		)
	}
	// names.SimpleNameGenerator is the exact upstream apiserver name
	// generator (also used by store.go's own GenerateName handling):
	// base + 5 random alphanumerics, self-truncated to fit the 63-char
	// Kubernetes name/label-value limit. pod.Name is already a valid
	// DNS-1123 subdomain (validated before admission runs), so a
	// mid-name truncation stays charset-valid.
	nodeName := names.SimpleNameGenerator.GenerateName("cf-" + pod.Name + "-")
	pod.Spec.NodeSelector[corev1.LabelHostname] = nodeName
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[NodeVMTierAnnotation] = tier
	return nil
}
