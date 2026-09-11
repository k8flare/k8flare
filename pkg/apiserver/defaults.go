package apiserver

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ApplyDefaults sets standard Kubernetes default values on an object.
// In a full K8s API server this is done by admission controllers, but our
// simplified control plane needs to set the critical defaults manually so
// that kubelet/containerd (and real controllers like kube-controller-manager)
// can process the objects correctly.
func ApplyDefaults(obj runtime.Object) {
	if pod, ok := obj.(*corev1.Pod); ok {
		applyPodOverrides(&pod.Spec)
		if pod.Status.Phase == "" {
			pod.Status.Phase = corev1.PodPending
		}
	}

	// Upstream's namespace registry strategy (PrepareForCreate) stamps
	// Status.Phase = Active on every created Namespace; kubectl's STATUS
	// column reads it. This apiserver has no registry strategies, so set
	// it here, same as the Pod phase above. (Terminating is not modeled:
	// namespace delete is a synchronous sweep, see namespacedelete.go.)
	if ns, ok := obj.(*corev1.Namespace); ok {
		if ns.Status.Phase == "" {
			ns.Status.Phase = corev1.NamespaceActive
		}
	}

	// Real upstream versioned defaulters registered on Scheme (scheme.go,
	// zz_generated_defaulters.go): core/v1 Pod spec defaults (RestartPolicy,
	// DNSPolicy, SecurityContext, TerminationGracePeriodSeconds,
	// SchedulerName, per-container ImagePullPolicy/TerminationMessagePath/
	// Policy, per-probe TimeoutSeconds/PeriodSeconds/SuccessThreshold/
	// FailureThreshold, ...) plus every other registered group's (apps/v1's
	// Deployment/DaemonSet/ReplicaSet strategy defaults, batch/v1's Job/
	// CronJob completionMode, etc). Must run after applyPodOverrides above,
	// so its EnableServiceLinks nil-default (see below) wins over
	// upstream's -- upstream's own Pod defaulter only fills a field when
	// it's still nil, same rule this project's override relies on.
	Scheme.Default(obj)
}

// applyPodOverrides sets the small number of Pod defaults real upstream
// core/v1 defaulting (Scheme.Default, above) either doesn't set at all
// (HostUsers, PreemptionPolicy -- confirmed absent from
// k8s.io/kubernetes/pkg/apis/core/v1's defaulting functions) or would set
// differently than this project intentionally wants for a nil input
// (EnableServiceLinks: upstream defaults nil to true; this project
// defaults nil to false, since it doesn't inject the corresponding service
// env vars a client would need for EnableServiceLinks to actually do
// anything).
func applyPodOverrides(spec *corev1.PodSpec) {
	if spec.EnableServiceLinks == nil {
		enable := false
		spec.EnableServiceLinks = &enable
	}
	if spec.HostUsers == nil {
		hostUsers := true
		spec.HostUsers = &hostUsers
	}
	if spec.PreemptionPolicy == nil {
		policy := corev1.PreemptLowerPriority
		spec.PreemptionPolicy = &policy
	}
	applyDefaultTolerationSeconds(spec)
}

// defaultTolerationSeconds mirrors the value of
// defaultNotReadyTolerationSeconds / defaultUnreachableTolerationSeconds in
// k8s.io/kubernetes/plugin/pkg/admission/defaulttolerationseconds, which are
// package-private there (settable only through that plugin's
// --default-not-ready-toleration-seconds / --default-unreachable-toleration-seconds
// flags, which this apiserver has no flag surface for). The plugin itself
// cannot be linked here: it admits internal api.Pod objects and drags
// k8s.io/kubernetes/pkg/apis/core, apiserver/pkg/admission,
// component-base/featuregate and spf13/pflag into a binary that has to stay
// under the Worker Loader's 64MiB cap.
const defaultTolerationSeconds = int64(300)

// applyDefaultTolerationSeconds is the versioned-type equivalent of that
// plugin's Admit: without it every Pod tolerates neither
// node.kubernetes.io/not-ready:NoExecute nor
// node.kubernetes.io/unreachable:NoExecute, so the real
// taint-eviction-controller deletes each Pod on an unreachable Node the
// instant nodelifecycle taints it. The replicaset controller then recreates
// the Pod, the scheduler binds it, taint eviction deletes it again: measured
// at ~2 Pods/second of create/bind/delete churn for as long as the Node was
// down, and with it ~230 deployment-status and ~110 replicaset-status writes
// per minute (docs/platform-verification.md S32).
func applyDefaultTolerationSeconds(spec *corev1.PodSpec) {
	toleratesNotReady, toleratesUnreachable := false, false
	for _, t := range spec.Tolerations {
		if t.Effect != corev1.TaintEffectNoExecute && t.Effect != "" {
			continue
		}
		if t.Key == corev1.TaintNodeNotReady || t.Key == "" {
			toleratesNotReady = true
		}
		if t.Key == corev1.TaintNodeUnreachable || t.Key == "" {
			toleratesUnreachable = true
		}
	}
	if !toleratesNotReady {
		spec.Tolerations = append(spec.Tolerations, defaultToleration(corev1.TaintNodeNotReady))
	}
	if !toleratesUnreachable {
		spec.Tolerations = append(spec.Tolerations, defaultToleration(corev1.TaintNodeUnreachable))
	}
}

func defaultToleration(key string) corev1.Toleration {
	seconds := defaultTolerationSeconds
	return corev1.Toleration{
		Key:               key,
		Operator:          corev1.TolerationOpExists,
		Effect:            corev1.TaintEffectNoExecute,
		TolerationSeconds: &seconds,
	}
}

// ValidateLimitRange checks a pod's container resource requests/limits
// against any Container-scoped Min/Max bounds in the given LimitRanges,
// returning an error naming the first violation found. Must run after
// ApplyLimitRangeDefaults, so Min/Max are checked against the final,
// resolved values rather than the pod's pre-defaulting spec — matching real
// Kubernetes, where LimitRanger both defaults and validates in one pass.
func ValidateLimitRange(pod *corev1.Pod, limitRanges []corev1.LimitRange) error {
	for _, lr := range limitRanges {
		for _, item := range lr.Spec.Limits {
			if item.Type != corev1.LimitTypeContainer {
				continue
			}
			for _, c := range pod.Spec.Containers {
				if err := validateContainerAgainstLimitRange(c, item); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateContainerAgainstLimitRange(c corev1.Container, item corev1.LimitRangeItem) error {
	for name, min := range item.Min {
		if req, ok := c.Resources.Requests[name]; ok && req.Cmp(min) < 0 {
			return fmt.Errorf("minimum %s usage per Container is %s, but request for container %q is %s", name, min.String(), c.Name, req.String())
		}
		if lim, ok := c.Resources.Limits[name]; ok && lim.Cmp(min) < 0 {
			return fmt.Errorf("minimum %s usage per Container is %s, but limit for container %q is %s", name, min.String(), c.Name, lim.String())
		}
	}
	for name, max := range item.Max {
		if req, ok := c.Resources.Requests[name]; ok && req.Cmp(max) > 0 {
			return fmt.Errorf("maximum %s usage per Container is %s, but request for container %q is %s", name, max.String(), c.Name, req.String())
		}
		if lim, ok := c.Resources.Limits[name]; ok && lim.Cmp(max) > 0 {
			return fmt.Errorf("maximum %s usage per Container is %s, but limit for container %q is %s", name, max.String(), c.Name, lim.String())
		}
	}
	return nil
}

// ApplyLimitRangeDefaults fills in any container resource requests/limits a
// pod didn't specify itself, using the Default/DefaultRequest values from any
// Container-scoped LimitRange in the pod's namespace — mirroring the subset
// of real Kubernetes LimitRange admission behavior needed for defaulting.
// Callers should also call ValidateLimitRange afterward to enforce Min/Max.
func ApplyLimitRangeDefaults(pod *corev1.Pod, limitRanges []corev1.LimitRange) {
	for _, lr := range limitRanges {
		for _, item := range lr.Spec.Limits {
			if item.Type != corev1.LimitTypeContainer {
				continue // Pod-scoped/PVC-scoped items are out of scope here
			}
			for i := range pod.Spec.Containers {
				applyContainerLimitRangeDefaults(&pod.Spec.Containers[i], item)
			}
		}
	}
}

func applyContainerLimitRangeDefaults(c *corev1.Container, item corev1.LimitRangeItem) {
	if c.Resources.Limits == nil {
		c.Resources.Limits = corev1.ResourceList{}
	}
	if c.Resources.Requests == nil {
		c.Resources.Requests = corev1.ResourceList{}
	}

	// Snapshot which resources the container explicitly set, before any
	// LimitRange-driven defaulting below mutates these maps.
	hadExplicitLimit := make(map[corev1.ResourceName]bool, len(c.Resources.Limits))
	for name := range c.Resources.Limits {
		hadExplicitLimit[name] = true
	}
	hadExplicitRequest := make(map[corev1.ResourceName]bool, len(c.Resources.Requests))
	for name := range c.Resources.Requests {
		hadExplicitRequest[name] = true
	}

	for name, val := range item.Default {
		if !hadExplicitLimit[name] {
			c.Resources.Limits[name] = val
		}
	}

	// A resource with an explicit limit but no explicit request has its
	// request default to that (original, explicit) limit — this takes
	// priority over LimitRange.DefaultRequest, which only applies when the
	// container had no limit for the resource at all. Matches real
	// Kubernetes LimitRanger admission behavior.
	for name := range hadExplicitLimit {
		if !hadExplicitRequest[name] {
			c.Resources.Requests[name] = c.Resources.Limits[name]
		}
	}
	for name, val := range item.DefaultRequest {
		if !hadExplicitRequest[name] && !hadExplicitLimit[name] {
			c.Resources.Requests[name] = val
		}
	}
}
