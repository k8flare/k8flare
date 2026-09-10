package apiserver

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestApplyDefaults_Pod_StatusPhasePending(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Status.Phase != corev1.PodPending {
		t.Errorf("Status.Phase = %v, want %v", pod.Status.Phase, corev1.PodPending)
	}
}

func TestApplyDefaults_Pod_StatusPhaseNotOverwritten(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	ApplyDefaults(pod)

	if pod.Status.Phase != corev1.PodRunning {
		t.Errorf("Status.Phase overwritten: got %v, want %v", pod.Status.Phase, corev1.PodRunning)
	}
}

func TestApplyDefaults_Pod_EnableServiceLinks(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Spec.EnableServiceLinks == nil {
		t.Fatal("EnableServiceLinks should be set")
	}
	if *pod.Spec.EnableServiceLinks != false {
		t.Errorf("EnableServiceLinks = %v, want false (intentional divergence from k8s default)", *pod.Spec.EnableServiceLinks)
	}
}

func TestApplyDefaults_Pod_HostUsers(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Spec.HostUsers == nil {
		t.Fatal("HostUsers should be set")
	}
	if *pod.Spec.HostUsers != true {
		t.Errorf("HostUsers = %v, want true", *pod.Spec.HostUsers)
	}
}

func tolerationFor(pod *corev1.Pod, key string) *corev1.Toleration {
	for i := range pod.Spec.Tolerations {
		if pod.Spec.Tolerations[i].Key == key {
			return &pod.Spec.Tolerations[i]
		}
	}
	return nil
}

func TestApplyDefaults_Pod_DefaultTolerationSeconds(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	for _, key := range []string{corev1.TaintNodeNotReady, corev1.TaintNodeUnreachable} {
		tol := tolerationFor(pod, key)
		if tol == nil {
			t.Fatalf("no toleration for %s", key)
		}
		if tol.Operator != corev1.TolerationOpExists {
			t.Errorf("%s Operator = %v, want %v", key, tol.Operator, corev1.TolerationOpExists)
		}
		if tol.Effect != corev1.TaintEffectNoExecute {
			t.Errorf("%s Effect = %v, want %v", key, tol.Effect, corev1.TaintEffectNoExecute)
		}
		if tol.TolerationSeconds == nil || *tol.TolerationSeconds != 300 {
			t.Errorf("%s TolerationSeconds = %v, want 300", key, tol.TolerationSeconds)
		}
	}
}

func TestApplyDefaults_Pod_DefaultTolerationSecondsRespectsExisting(t *testing.T) {
	ten := int64(10)
	pod := &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{
		Key:               corev1.TaintNodeUnreachable,
		Operator:          corev1.TolerationOpExists,
		Effect:            corev1.TaintEffectNoExecute,
		TolerationSeconds: &ten,
	}}}}
	ApplyDefaults(pod)

	tol := tolerationFor(pod, corev1.TaintNodeUnreachable)
	if tol == nil || tol.TolerationSeconds == nil || *tol.TolerationSeconds != 10 {
		t.Errorf("declared unreachable toleration overwritten: %+v", tol)
	}
	if tolerationFor(pod, corev1.TaintNodeNotReady) == nil {
		t.Error("not-ready toleration should still be defaulted")
	}
	if len(pod.Spec.Tolerations) != 2 {
		t.Errorf("Tolerations = %d, want 2", len(pod.Spec.Tolerations))
	}
}

func TestApplyDefaults_Pod_DefaultTolerationSecondsWildcard(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{
		Operator: corev1.TolerationOpExists,
	}}}}
	ApplyDefaults(pod)

	if len(pod.Spec.Tolerations) != 1 {
		t.Errorf("Tolerations = %d, want 1 (wildcard tolerates both taints)", len(pod.Spec.Tolerations))
	}
}

func TestApplyDefaults_Pod_DNSPolicy(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Spec.DNSPolicy != corev1.DNSClusterFirst {
		t.Errorf("DNSPolicy = %v, want %v", pod.Spec.DNSPolicy, corev1.DNSClusterFirst)
	}
}

func TestApplyDefaults_Pod_RestartPolicy(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Errorf("RestartPolicy = %v, want %v", pod.Spec.RestartPolicy, corev1.RestartPolicyAlways)
	}
}

func TestApplyDefaults_Pod_TerminationGracePeriod(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Spec.TerminationGracePeriodSeconds == nil {
		t.Fatal("TerminationGracePeriodSeconds should be set")
	}
	if *pod.Spec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("TerminationGracePeriodSeconds = %d, want 30", *pod.Spec.TerminationGracePeriodSeconds)
	}
}

func TestApplyDefaults_Pod_SchedulerName(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Spec.SchedulerName != corev1.DefaultSchedulerName {
		t.Errorf("SchedulerName = %v, want %v", pod.Spec.SchedulerName, corev1.DefaultSchedulerName)
	}
}

func TestApplyDefaults_Pod_PreemptionPolicy(t *testing.T) {
	pod := &corev1.Pod{}
	ApplyDefaults(pod)

	if pod.Spec.PreemptionPolicy == nil {
		t.Fatal("PreemptionPolicy should be set")
	}
	if *pod.Spec.PreemptionPolicy != corev1.PreemptLowerPriority {
		t.Errorf("PreemptionPolicy = %v, want %v", *pod.Spec.PreemptionPolicy, corev1.PreemptLowerPriority)
	}
}

func TestApplyDefaults_Pod_ContainerDefaults(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test", Image: "nginx"},
			},
		},
	}
	ApplyDefaults(pod)

	c := pod.Spec.Containers[0]
	if c.TerminationMessagePath != corev1.TerminationMessagePathDefault {
		t.Errorf("TerminationMessagePath = %v, want %v", c.TerminationMessagePath, corev1.TerminationMessagePathDefault)
	}
	if c.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("TerminationMessagePolicy = %v, want %v", c.TerminationMessagePolicy, corev1.TerminationMessageReadFile)
	}
	// Real upstream core/v1 defaulting (ApplyDefaults delegates to it via
	// Scheme.Default, see defaults.go) defaults ImagePullPolicy to Always
	// for an untagged image like "nginx" here -- the tag is ambiguous/
	// mutable, so always checking for a newer image is the correct
	// default, matching what a real cluster does. Only an explicitly
	// tagged image (e.g. "nginx:1.25") defaults to IfNotPresent.
	if c.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("ImagePullPolicy = %v, want %v", c.ImagePullPolicy, corev1.PullAlways)
	}
}

// TestApplyDefaults_Pod_ProbeDefaults guards against a real kubelet crash
// found via live e2e testing: PeriodSeconds is passed straight into
// time.NewTicker, which panics on 0 and takes down the whole kubelet
// process, not just the offending pod.
func TestApplyDefaults_Pod_ProbeDefaults(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:           "test",
					Image:          "nginx",
					LivenessProbe:  &corev1.Probe{},
					ReadinessProbe: &corev1.Probe{},
					StartupProbe:   &corev1.Probe{},
				},
			},
		},
	}
	ApplyDefaults(pod)

	for _, p := range []*corev1.Probe{
		pod.Spec.Containers[0].LivenessProbe,
		pod.Spec.Containers[0].ReadinessProbe,
		pod.Spec.Containers[0].StartupProbe,
	} {
		if p.PeriodSeconds != 10 {
			t.Errorf("PeriodSeconds = %d, want 10", p.PeriodSeconds)
		}
		if p.TimeoutSeconds != 1 {
			t.Errorf("TimeoutSeconds = %d, want 1", p.TimeoutSeconds)
		}
		if p.SuccessThreshold != 1 {
			t.Errorf("SuccessThreshold = %d, want 1", p.SuccessThreshold)
		}
		if p.FailureThreshold != 3 {
			t.Errorf("FailureThreshold = %d, want 3", p.FailureThreshold)
		}
	}
}

func TestApplyDefaults_Pod_ProbeDefaults_DoesNotOverwriteExplicitValues(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "nginx",
					LivenessProbe: &corev1.Probe{
						PeriodSeconds:    5,
						TimeoutSeconds:   2,
						SuccessThreshold: 1,
						FailureThreshold: 1,
					},
				},
			},
		},
	}
	ApplyDefaults(pod)

	p := pod.Spec.Containers[0].LivenessProbe
	if p.PeriodSeconds != 5 {
		t.Errorf("PeriodSeconds overwritten: got %d, want 5", p.PeriodSeconds)
	}
	if p.FailureThreshold != 1 {
		t.Errorf("FailureThreshold overwritten: got %d, want 1", p.FailureThreshold)
	}
}

func TestApplyDefaults_Pod_InitContainerDefaults(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init", Image: "busybox"},
			},
		},
	}
	ApplyDefaults(pod)

	c := pod.Spec.InitContainers[0]
	if c.TerminationMessagePath != corev1.TerminationMessagePathDefault {
		t.Errorf("TerminationMessagePath = %v, want %v", c.TerminationMessagePath, corev1.TerminationMessagePathDefault)
	}
	if c.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("TerminationMessagePolicy = %v, want %v", c.TerminationMessagePolicy, corev1.TerminationMessageReadFile)
	}
	// See TestApplyDefaults_Pod_ContainerDefaults: "busybox" is untagged,
	// so real upstream defaulting gives it Always, not IfNotPresent.
	if c.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("ImagePullPolicy = %v, want %v", c.ImagePullPolicy, corev1.PullAlways)
	}
}

func TestApplyDefaults_Node_NotModified(t *testing.T) {
	node := &corev1.Node{}
	ApplyDefaults(node)

	// ApplyDefaults should not modify a Node (only Pod is handled).
	// Verify that the Node is still empty.
	if node.Name != "" {
		t.Errorf("Node.Name should be empty, got %v", node.Name)
	}
	if len(node.Spec.Taints) != 0 {
		t.Errorf("Node.Spec.Taints should be empty, got %v", node.Spec.Taints)
	}
}

func TestApplyDefaults_Pod_DoesNotOverwriteExistingValues(t *testing.T) {
	grace := int64(60)
	enableSvc := true
	hostUsers := false
	policy := corev1.PreemptNever

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyNever,
			DNSPolicy:                     corev1.DNSDefault,
			TerminationGracePeriodSeconds: &grace,
			SchedulerName:                 "custom-scheduler",
			EnableServiceLinks:            &enableSvc,
			HostUsers:                     &hostUsers,
			PreemptionPolicy:              &policy,
			Containers: []corev1.Container{
				{
					Name:                     "test",
					Image:                    "nginx",
					TerminationMessagePath:   "/custom/path",
					TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
					ImagePullPolicy:          corev1.PullAlways,
				},
			},
		},
	}
	ApplyDefaults(pod)

	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("RestartPolicy overwritten: got %v, want %v", pod.Spec.RestartPolicy, corev1.RestartPolicyNever)
	}
	if pod.Spec.DNSPolicy != corev1.DNSDefault {
		t.Errorf("DNSPolicy overwritten: got %v, want %v", pod.Spec.DNSPolicy, corev1.DNSDefault)
	}
	if *pod.Spec.TerminationGracePeriodSeconds != 60 {
		t.Errorf("TerminationGracePeriodSeconds overwritten: got %d, want 60", *pod.Spec.TerminationGracePeriodSeconds)
	}
	if pod.Spec.SchedulerName != "custom-scheduler" {
		t.Errorf("SchedulerName overwritten: got %v, want custom-scheduler", pod.Spec.SchedulerName)
	}
	if *pod.Spec.EnableServiceLinks != true {
		t.Errorf("EnableServiceLinks overwritten: got %v, want true", *pod.Spec.EnableServiceLinks)
	}
	if *pod.Spec.HostUsers != false {
		t.Errorf("HostUsers overwritten: got %v, want false", *pod.Spec.HostUsers)
	}
	if *pod.Spec.PreemptionPolicy != corev1.PreemptNever {
		t.Errorf("PreemptionPolicy overwritten: got %v, want %v", *pod.Spec.PreemptionPolicy, corev1.PreemptNever)
	}

	c := pod.Spec.Containers[0]
	if c.TerminationMessagePath != "/custom/path" {
		t.Errorf("TerminationMessagePath overwritten: got %v, want /custom/path", c.TerminationMessagePath)
	}
	if c.TerminationMessagePolicy != corev1.TerminationMessageFallbackToLogsOnError {
		t.Errorf("TerminationMessagePolicy overwritten: got %v, want %v", c.TerminationMessagePolicy, corev1.TerminationMessageFallbackToLogsOnError)
	}
	if c.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("ImagePullPolicy overwritten: got %v, want %v", c.ImagePullPolicy, corev1.PullAlways)
	}
}

func TestApplyLimitRangeDefaults_FillsMissingRequestsAndLimits(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "test", Image: "nginx"}},
		},
	}
	limitRanges := []corev1.LimitRange{
		{
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						Default: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("500Mi"),
						},
						DefaultRequest: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
			},
		},
	}

	ApplyLimitRangeDefaults(pod, limitRanges)

	c := pod.Spec.Containers[0]
	if got := c.Resources.Limits[corev1.ResourceCPU]; got.Cmp(resource.MustParse("500m")) != 0 {
		t.Errorf("Limits[cpu] = %v, want 500m", got.String())
	}
	if got := c.Resources.Limits[corev1.ResourceMemory]; got.Cmp(resource.MustParse("500Mi")) != 0 {
		t.Errorf("Limits[memory] = %v, want 500Mi", got.String())
	}
	if got := c.Resources.Requests[corev1.ResourceCPU]; got.Cmp(resource.MustParse("100m")) != 0 {
		t.Errorf("Requests[cpu] = %v, want 100m", got.String())
	}
	if got := c.Resources.Requests[corev1.ResourceMemory]; got.Cmp(resource.MustParse("200Mi")) != 0 {
		t.Errorf("Requests[memory] = %v, want 200Mi", got.String())
	}
}

// TestApplyLimitRangeDefaults_RequestDefaultsToExplicitLimit covers the real
// Kubernetes LimitRanger nuance the official e2e conformance suite exercises:
// a container with an explicit limit but no request gets its request
// defaulted to that limit, NOT to LimitRange.DefaultRequest — DefaultRequest
// only applies when the container has no limit for the resource at all.
func TestApplyLimitRangeDefaults_RequestDefaultsToExplicitLimit(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						// cpu: explicit limit, no request.
						Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("300m")},
						// memory: explicit request, no limit.
						Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("150Mi")},
					},
				},
			},
		},
	}
	limitRanges := []corev1.LimitRange{
		{
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						Default: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("500Mi"),
						},
						DefaultRequest: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
			},
		},
	}

	ApplyLimitRangeDefaults(pod, limitRanges)

	c := pod.Spec.Containers[0]
	if got := c.Resources.Limits[corev1.ResourceCPU]; got.Cmp(resource.MustParse("300m")) != 0 {
		t.Errorf("Limits[cpu] = %v, want 300m (explicit, unchanged)", got.String())
	}
	if got := c.Resources.Requests[corev1.ResourceCPU]; got.Cmp(resource.MustParse("300m")) != 0 {
		t.Errorf("Requests[cpu] = %v, want 300m (defaulted from explicit limit, not DefaultRequest's 100m)", got.String())
	}
	if got := c.Resources.Limits[corev1.ResourceMemory]; got.Cmp(resource.MustParse("500Mi")) != 0 {
		t.Errorf("Limits[memory] = %v, want 500Mi (from LimitRange.Default, no explicit limit)", got.String())
	}
	if got := c.Resources.Requests[corev1.ResourceMemory]; got.Cmp(resource.MustParse("150Mi")) != 0 {
		t.Errorf("Requests[memory] = %v, want 150Mi (explicit, unchanged)", got.String())
	}
}

func TestApplyLimitRangeDefaults_DoesNotOverwriteExplicitValues(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("750m")},
					},
				},
			},
		},
	}
	limitRanges := []corev1.LimitRange{
		{
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type:           corev1.LimitTypeContainer,
						Default:        corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
						DefaultRequest: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
					},
				},
			},
		},
	}

	ApplyLimitRangeDefaults(pod, limitRanges)

	c := pod.Spec.Containers[0]
	if got := c.Resources.Requests[corev1.ResourceCPU]; got.Cmp(resource.MustParse("250m")) != 0 {
		t.Errorf("Requests[cpu] overwritten: got %v, want 250m", got.String())
	}
	if got := c.Resources.Limits[corev1.ResourceCPU]; got.Cmp(resource.MustParse("750m")) != 0 {
		t.Errorf("Limits[cpu] overwritten: got %v, want 750m", got.String())
	}
}

func TestApplyLimitRangeDefaults_IgnoresPodScopedItems(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "test", Image: "nginx"}},
		},
	}
	limitRanges := []corev1.LimitRange{
		{
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type:    corev1.LimitTypePod,
						Default: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
					},
				},
			},
		},
	}

	ApplyLimitRangeDefaults(pod, limitRanges)

	c := pod.Spec.Containers[0]
	if _, exists := c.Resources.Limits[corev1.ResourceCPU]; exists {
		t.Errorf("Pod-scoped LimitRangeItem should not apply to container resources, got %v", c.Resources.Limits)
	}
}

func limitRangeWithBounds() []corev1.LimitRange {
	return []corev1.LimitRange{
		{
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						Min:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
						Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
					},
				},
			},
		},
	}
}

func TestValidateLimitRange_RejectsBelowMin(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m")},
					},
				},
			},
		},
	}
	if err := ValidateLimitRange(pod, limitRangeWithBounds()); err == nil {
		t.Error("Expected an error for a request below LimitRange.Min, got nil")
	}
}

func TestValidateLimitRange_RejectsAboveMax(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
				},
			},
		},
	}
	if err := ValidateLimitRange(pod, limitRangeWithBounds()); err == nil {
		t.Error("Expected an error for a limit above LimitRange.Max, got nil")
	}
}

func TestValidateLimitRange_AllowsWithinBounds(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("300m")},
					},
				},
			},
		},
	}
	if err := ValidateLimitRange(pod, limitRangeWithBounds()); err != nil {
		t.Errorf("Expected no error for values within bounds, got: %v", err)
	}
}
