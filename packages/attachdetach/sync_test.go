package attachdetach

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	volumeutil "k8s.io/kubernetes/pkg/volume/util"
)

func TestEncodeStorageListOptions(t *testing.T) {
	_, err := scheme.ParameterCodec.EncodeParameters(&metav1.ListOptions{Limit: 500}, storagev1.SchemeGroupVersion)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncCreatesVolumeAttachment(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:        "n1",
		Annotations: map[string]string{volumeutil.ControllerManagedAttachAnnotation: "true"},
	}}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv1"},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{Driver: "csi.example.com", VolumeHandle: "vol-1"},
			},
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Capacity:    v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
			ClaimRef:    &v1.ObjectReference{Namespace: "default", Name: "pvc1"},
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "default"},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources:   v1.VolumeResourceRequirements{Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")}},
			VolumeName:  "pv1",
		},
		Status: v1.PersistentVolumeClaimStatus{Phase: v1.ClaimBound},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default", UID: "pod-1"},
		Spec: v1.PodSpec{
			NodeName: "n1",
			Volumes: []v1.Volume{{
				Name: "data",
				VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"}},
			}},
			Containers: []v1.Container{{Name: "c", Image: "pause"}},
		},
	}
	client := fake.NewSimpleClientset(node, pv, pvc, pod)
	result, err := syncFor(context.Background(), client, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Objects["pods"] != 1 || result.Objects["persistentvolumes"] != 1 {
		t.Fatalf("objects = %v", result.Objects)
	}
	want := fmt.Sprintf("csi-%x", sha256.Sum256([]byte("vol-1csi.example.comn1")))
	var va *storagev1.VolumeAttachment
	deadline := time.Now().Add(3 * time.Second)
	for {
		va, err = client.StorageV1().VolumeAttachments().Get(context.Background(), want, metav1.GetOptions{})
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("VolumeAttachment %s: %v", want, err)
	}
	if va.Spec.NodeName != "n1" || va.Spec.Attacher != "csi.example.com" || va.Spec.Source.PersistentVolumeName == nil || *va.Spec.Source.PersistentVolumeName != "pv1" {
		t.Fatalf("spec = %+v", va.Spec)
	}
}
