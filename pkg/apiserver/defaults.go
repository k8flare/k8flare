package apiserver

import (
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
}

// ApplyLimitRangeDefaults fills in any container resource requests/limits a
// pod didn't specify itself, using the Default/DefaultRequest values from any
// Container-scoped LimitRange in the pod's namespace — mirroring the subset
// of real Kubernetes LimitRange admission behavior needed for defaulting.
// Min/Max range enforcement (rejecting a pod for violating range bounds) is
// deliberately not implemented; nothing in this codebase exercises it.
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
	for name, val := range item.Default {
		if _, exists := c.Resources.Limits[name]; !exists {
			c.Resources.Limits[name] = val
		}
	}
	for name, val := range item.DefaultRequest {
		if _, exists := c.Resources.Requests[name]; !exists {
			c.Resources.Requests[name] = val
		}
	}
}
