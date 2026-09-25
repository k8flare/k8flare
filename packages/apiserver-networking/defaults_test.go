package networking

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsNetworkPolicy(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{
		Ingress: []networkingv1.NetworkPolicyIngressRule{{Ports: []networkingv1.NetworkPolicyPort{{}}}},
		Egress:  []networkingv1.NetworkPolicyEgressRule{{}},
	}}
	scheme.Scheme.Default(policy)
	if len(policy.Spec.PolicyTypes) != 2 || policy.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress || policy.Spec.PolicyTypes[1] != networkingv1.PolicyTypeEgress {
		t.Fatalf("policyTypes=%v", policy.Spec.PolicyTypes)
	}
	if policy.Spec.Ingress[0].Ports[0].Protocol == nil || *policy.Spec.Ingress[0].Ports[0].Protocol != corev1.ProtocolTCP {
		t.Fatalf("protocol=%v", policy.Spec.Ingress[0].Ports[0].Protocol)
	}
}
