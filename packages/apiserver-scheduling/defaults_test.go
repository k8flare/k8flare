package scheduling

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsPriorityClassPreemption(t *testing.T) {
	pc := &schedulingv1.PriorityClass{}
	scheme.Scheme.Default(pc)
	if pc.PreemptionPolicy == nil || *pc.PreemptionPolicy != corev1.PreemptLowerPriority {
		t.Fatalf("preemption=%v", pc.PreemptionPolicy)
	}
}
