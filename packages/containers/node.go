package containers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	TaintKey          = "k8flare.com/pod-on-containers"
	VirtualNodeLabel  = "type"
	VirtualNodeType   = "virtual-kubelet"
	computeLabel      = "k8flare.com/compute"
	computeContainers = "containers"
)

func VirtualNode() *corev1.Node {
	capacity := corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse("4"),
		corev1.ResourceMemory:           resource.MustParse("12Gi"),
		corev1.ResourceEphemeralStorage: resource.MustParse("20G"),
		corev1.ResourcePods:             resource.MustParse("110"),
	}
	now := metav1.Now()
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName,
			Labels: map[string]string{
				VirtualNodeLabel:       VirtualNodeType,
				"kubernetes.io/role":   "agent",
				corev1.LabelHostname:   NodeName,
				computeLabel:           computeContainers,
				corev1.LabelOSStable:   "linux",
				corev1.LabelArchStable: "amd64",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: TaintKey, Value: "true", Effect: corev1.TaintEffectNoSchedule}},
		},
		Status: corev1.NodeStatus{
			Capacity:    capacity,
			Allocatable: capacity,
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady", Message: "Pod on Containers is ready", LastHeartbeatTime: now, LastTransitionTime: now},
			},
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeHostName, Address: NodeName}},
			NodeInfo:  corev1.NodeSystemInfo{OperatingSystem: "linux", Architecture: "amd64", KubeletVersion: "k8flare-containers"},
		},
	}
}

func EnsureNode(ctx context.Context, client kubernetes.Interface) error {
	nodes := client.CoreV1().Nodes()
	_, err := nodes.Get(ctx, NodeName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	want := VirtualNode()
	created, err := nodes.Create(ctx, want, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return err
	}
	created.Status = want.Status
	_, err = nodes.UpdateStatus(ctx, created, metav1.UpdateOptions{})
	return err
}
