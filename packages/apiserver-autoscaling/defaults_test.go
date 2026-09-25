package autoscaling

import (
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsHorizontalPodAutoscaler(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	scheme.Scheme.Default(hpa)
	if hpa.Spec.MinReplicas == nil || *hpa.Spec.MinReplicas != 1 {
		t.Fatalf("minReplicas=%v", hpa.Spec.MinReplicas)
	}
	if len(hpa.Spec.Metrics) != 1 || hpa.Spec.Metrics[0].Resource == nil || hpa.Spec.Metrics[0].Resource.Target.AverageUtilization == nil || *hpa.Spec.Metrics[0].Resource.Target.AverageUtilization != 80 {
		t.Fatalf("metrics=%v", hpa.Spec.Metrics)
	}
}
