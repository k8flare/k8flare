package storage

import (
	"context"
	"github.com/k8flare/k8flare/packages/workloads"
	_ "github.com/k8flare/k8flare/packages/workloads/shards/all"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSyncBindsStaticPersistentVolume(t *testing.T) {
	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "static", UID: "pv1", ResourceVersion: "1"},
		Spec: v1.PersistentVolumeSpec{
			Capacity:                      v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
			AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			PersistentVolumeSource:        v1.PersistentVolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/tmp/static"}},
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
			VolumeMode:                    &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "default", UID: "pvc1", ResourceVersion: "1"},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources:   v1.VolumeResourceRequirements{Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")}},
			VolumeMode:  &mode,
		},
	}
	client := fake.NewSimpleClientset(pv, pvc)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"persistentvolumeclaims"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "claim", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.VolumeName != "static" || got.Status.Phase != v1.ClaimBound {
		t.Fatalf("claim = %s %s", got.Spec.VolumeName, got.Status.Phase)
	}
	bound, err := client.CoreV1().PersistentVolumes().Get(context.Background(), "static", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if bound.Status.Phase != v1.VolumeBound || bound.Spec.ClaimRef == nil || bound.Spec.ClaimRef.Name != "claim" {
		t.Fatalf("volume = %s %+v", bound.Status.Phase, bound.Spec.ClaimRef)
	}
}
