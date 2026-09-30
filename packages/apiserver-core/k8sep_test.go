package core

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadyNodeIPsSkipsNotReadyAndSorts(t *testing.T) {
	ips := readyNodeIPs([]corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "b"},
			Status: corev1.NodeStatus{
				Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.2"}},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a"},
			Status: corev1.NodeStatus{
				Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.1"}},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "down"},
			Status: corev1.NodeStatus{
				Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.9"}},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}},
			},
		},
	})
	if len(ips) != 2 || ips[0] != "10.0.0.1" || ips[1] != "10.0.0.2" {
		t.Fatalf("got %v", ips)
	}
}

func TestKubernetesEndpointsEmptyWhenNoReadyNodes(t *testing.T) {
	ep := kubernetesEndpoints(nil)
	if ep.Name != "kubernetes" || len(ep.Subsets) != 0 {
		t.Fatalf("got %#v", ep)
	}
}

func TestKubernetesSlicePortsAndAddresses(t *testing.T) {
	slice := kubernetesSlice([]string{"192.168.1.1"})
	if slice.AddressType != "IPv4" || len(slice.Endpoints) != 1 || slice.Endpoints[0].Addresses[0] != "192.168.1.1" {
		t.Fatalf("got %#v", slice)
	}
	if slice.Ports[0].Port == nil || *slice.Ports[0].Port != 6443 {
		t.Fatalf("port %#v", slice.Ports)
	}
	if slice.UID == "" || slice.CreationTimestamp.IsZero() {
		t.Fatal("slice is missing identity")
	}
}

func TestKubernetesSliceEndpointsAreServingTerminatingSoAgentsKeepTheirTunnelTargets(t *testing.T) {
	slice := kubernetesSlice([]string{"192.168.1.1", "192.168.1.2"})
	for _, ep := range slice.Endpoints {
		c := ep.Conditions
		if c.Ready == nil || *c.Ready {
			t.Fatalf("endpoint %v must not be ready: %#v", ep.Addresses, c)
		}
		if c.Serving == nil || !*c.Serving || c.Terminating == nil || !*c.Terminating {
			t.Fatalf("endpoint %v must be serving and terminating: %#v", ep.Addresses, c)
		}
	}
}
