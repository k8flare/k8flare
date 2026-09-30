package scheduler

import (
	"context"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func classNamed(name string, mode storagev1.VolumeBindingMode) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: name},
		Provisioner:       "rancher.io/local-path",
		VolumeBindingMode: &mode,
	}
}

func claimOf(name, class string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID("uid-" + name), ResourceVersion: "1"},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &class,
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources:        v1.VolumeResourceRequirements{Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")}},
		},
		Status: v1.PersistentVolumeClaimStatus{Phase: v1.ClaimPending},
	}
}

func podWithClaim(name, uid, claim string) *v1.Pod {
	p := pod(name, uid, "100m")
	p.Spec.Volumes = []v1.Volume{{Name: "data", VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: claim}}}}
	return p
}

func TestScheduleAnnotatesSelectedNodeForWaitForFirstConsumerClaim(t *testing.T) {
	client := fake.NewSimpleClientset(node("n1", "1"), classNamed("local-path", storagev1.VolumeBindingWaitForFirstConsumer), claimOf("data", "local-path"), podWithClaim("app", "a", "data"))
	if _, err := Schedule(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Annotations["volume.kubernetes.io/selected-node"] != "n1" {
		t.Fatalf("annotations = %v", got.Annotations)
	}
}

func TestScheduleKeepsPodWithUnboundImmediateClaimUnschedulable(t *testing.T) {
	client := fake.NewSimpleClientset(node("n1", "1"), classNamed("fast", storagev1.VolumeBindingImmediate), claimOf("data", "fast"), podWithClaim("app", "a", "data"))
	result, err := Schedule(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bound != 0 || len(result.Unschedulable) != 1 {
		t.Fatalf("result = %+v", result)
	}
	got, err := client.CoreV1().Pods("default").Get(context.Background(), "app", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var message string
	for _, c := range got.Status.Conditions {
		if c.Type == v1.PodScheduled {
			message = c.Message
		}
	}
	if !strings.Contains(message, "unbound immediate PersistentVolumeClaims") {
		t.Fatalf("condition message = %q", message)
	}
}
