package core

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestValidatePodSysctlsRejectsInvalidNames(t *testing.T) {
	errs := validatePodSysctls(&corev1.Pod{Spec: corev1.PodSpec{SecurityContext: &corev1.PodSecurityContext{Sysctls: []corev1.Sysctl{
		{Name: "foo-", Value: "bar"},
		{Name: "kernel.shmmax", Value: "1"},
		{Name: "safe-and-unsafe", Value: "1"},
		{Name: "bar..", Value: "42"},
	}}}})
	if len(errs) == 0 {
		t.Fatal("expected invalid sysctl names")
	}
	got := errs.ToAggregate().Error()
	if !strings.Contains(got, `Invalid value: "foo-"`) || !strings.Contains(got, `Invalid value: "bar.."`) {
		t.Fatalf("got %s", got)
	}
	if strings.Contains(got, "safe-and-unsafe") || strings.Contains(got, "kernel.shmmax") {
		t.Fatalf("unexpected %s", got)
	}
}
