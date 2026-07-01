package apiserver

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ApplyDefaults sets standard Kubernetes default values on an object.
// In a full K8s API server this is done by admission controllers, but our
// simplified control plane needs to set the critical defaults manually so
// that kubelet/containerd can process the objects correctly.
func ApplyDefaults(obj runtime.Object) {
	switch o := obj.(type) {
	case *corev1.Pod:
		defaultPodSpec(&o.Spec)
		if o.Status.Phase == "" {
			o.Status.Phase = corev1.PodPending
		}
	}
}

func defaultPodSpec(spec *corev1.PodSpec) {
	if spec.RestartPolicy == "" {
		spec.RestartPolicy = corev1.RestartPolicyAlways
	}
	if spec.DNSPolicy == "" {
		spec.DNSPolicy = corev1.DNSClusterFirst
	}
	if spec.SecurityContext == nil {
		spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if spec.TerminationGracePeriodSeconds == nil {
		grace := int64(30)
		spec.TerminationGracePeriodSeconds = &grace
	}
	if spec.SchedulerName == "" {
		spec.SchedulerName = corev1.DefaultSchedulerName
	}
	if spec.EnableServiceLinks == nil {
		enable := false // disable service links since we don't inject service env vars
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

	for i := range spec.Containers {
		defaultContainer(&spec.Containers[i])
	}
	for i := range spec.InitContainers {
		defaultContainer(&spec.InitContainers[i])
	}
}

func defaultContainer(c *corev1.Container) {
	if c.TerminationMessagePath == "" {
		c.TerminationMessagePath = corev1.TerminationMessagePathDefault
	}
	if c.TerminationMessagePolicy == "" {
		c.TerminationMessagePolicy = corev1.TerminationMessageReadFile
	}
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = corev1.PullIfNotPresent
	}
	defaultProbe(c.LivenessProbe)
	defaultProbe(c.ReadinessProbe)
	defaultProbe(c.StartupProbe)
}

// defaultProbe fills in a probe's timing fields when left unset. Real
// kubelet passes PeriodSeconds straight into time.NewTicker, which panics
// on a non-positive interval — a probe with an explicit action but no
// PeriodSeconds crashes the whole kubelet process, not just that one pod.
func defaultProbe(p *corev1.Probe) {
	if p == nil {
		return
	}
	if p.TimeoutSeconds == 0 {
		p.TimeoutSeconds = 1
	}
	if p.PeriodSeconds == 0 {
		p.PeriodSeconds = 10
	}
	if p.SuccessThreshold == 0 {
		p.SuccessThreshold = 1
	}
	if p.FailureThreshold == 0 {
		p.FailureThreshold = 3
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
