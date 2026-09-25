package core

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsReplicationControllerReplicas(t *testing.T) {
	rc := &corev1.ReplicationController{}
	scheme.Scheme.Default(rc)
	if rc.Spec.Replicas == nil || *rc.Spec.Replicas != 1 {
		t.Fatalf("replicas=%v", rc.Spec.Replicas)
	}
}
