package workloads

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReleaseStorageProtectionDropsUnusedFinalizers(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "free", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{pvcProtectionFinalizer}}},
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "used", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{pvcProtectionFinalizer}}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default"}, Spec: v1.PodSpec{NodeName: "n", Volumes: []v1.Volume{{Name: "data", VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "used"}}}}}},
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{pvcProtectionFinalizer}}},
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "waiting", Namespace: "default"}, Spec: v1.PodSpec{Volumes: []v1.Volume{{Name: "data", VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "pending"}}}}}},
		&v1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv", DeletionTimestamp: &now, Finalizers: []string{pvProtectionFinalizer}}},
		&v1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "bound", DeletionTimestamp: &now, Finalizers: []string{pvProtectionFinalizer}}, Status: v1.PersistentVolumeStatus{Phase: v1.VolumeBound}},
	)
	if err := releaseStorageProtection(context.Background(), client, []string{"persistentvolumeclaims", "persistentvolumes"}); err != nil {
		t.Fatal(err)
	}
	free, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "free", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinalizer(free.Finalizers, pvcProtectionFinalizer) {
		t.Fatalf("free finalizers=%v", free.Finalizers)
	}
	used, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "used", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinalizer(used.Finalizers, pvcProtectionFinalizer) {
		t.Fatal("used claim finalizer was removed")
	}
	pending, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "pending", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinalizer(pending.Finalizers, pvcProtectionFinalizer) {
		t.Fatal("unscheduled pod kept the claim finalizer")
	}
	pv, err := client.CoreV1().PersistentVolumes().Get(context.Background(), "pv", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinalizer(pv.Finalizers, pvProtectionFinalizer) {
		t.Fatalf("pv finalizers=%v", pv.Finalizers)
	}
	bound, err := client.CoreV1().PersistentVolumes().Get(context.Background(), "bound", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinalizer(bound.Finalizers, pvProtectionFinalizer) {
		t.Fatal("bound volume finalizer was removed")
	}
}

func TestReleaseStorageProtectionKeepsEphemeralClaim(t *testing.T) {
	now := metav1.Now()
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default", UID: "pod-uid"}, Spec: v1.PodSpec{
		NodeName: "n",
		Volumes:  []v1.Volume{{Name: "data", VolumeSource: v1.VolumeSource{Ephemeral: &v1.EphemeralVolumeSource{}}}},
	}}
	client := fake.NewSimpleClientset(
		pod,
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "pod-data", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{pvcProtectionFinalizer},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: "pod", UID: types.UID("pod-uid"), Controller: ptrBool(true)}},
		}},
	)
	if err := releaseStorageProtection(context.Background(), client, []string{"persistentvolumeclaims"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "pod-data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinalizer(got.Finalizers, pvcProtectionFinalizer) {
		t.Fatal("ephemeral claim finalizer was removed")
	}
}

func ptrBool(v bool) *bool { return &v }

func TestReleaseVolumeAttributesClassDropsUnusedFinalizer(t *testing.T) {
	now := metav1.Now()
	name := "busy"
	client := fake.NewSimpleClientset(
		&storagev1.VolumeAttributesClass{ObjectMeta: metav1.ObjectMeta{Name: "gold", DeletionTimestamp: &now, Finalizers: []string{vacProtectionFinalizer}}},
		&storagev1.VolumeAttributesClass{ObjectMeta: metav1.ObjectMeta{Name: "busy", DeletionTimestamp: &now, Finalizers: []string{vacProtectionFinalizer}}},
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "default"}, Spec: v1.PersistentVolumeClaimSpec{VolumeAttributesClassName: &name}},
	)
	if err := releaseVolumeAttributesClasses(context.Background(), client, []string{"volumeattributesclasses"}); err != nil {
		t.Fatal(err)
	}
	gold, err := client.StorageV1().VolumeAttributesClasses().Get(context.Background(), "gold", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinalizer(gold.Finalizers, vacProtectionFinalizer) {
		t.Fatalf("gold finalizers=%v", gold.Finalizers)
	}
	busy, err := client.StorageV1().VolumeAttributesClasses().Get(context.Background(), "busy", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinalizer(busy.Finalizers, vacProtectionFinalizer) {
		t.Fatal("busy finalizer was removed")
	}
}

func TestSyncWithinReleasesProtectionWhileAPassHoldsTheLock(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&storagev1.VolumeAttributesClass{ObjectMeta: metav1.ObjectMeta{Name: "vac", DeletionTimestamp: &now, Finalizers: []string{vacProtectionFinalizer}}},
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{pvcProtectionFinalizer}}},
	)
	syncMu.Lock()
	defer syncMu.Unlock()

	changed := []string{"volumeattributesclasses", "persistentvolumeclaims"}
	if _, err := SyncWithin(context.Background(), client, []byte("ca"), nil, nil, changed, 4*time.Second, nil); err != nil {
		t.Fatal(err)
	}
	vac, err := client.StorageV1().VolumeAttributesClasses().Get(context.Background(), "vac", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinalizer(vac.Finalizers, vacProtectionFinalizer) {
		t.Fatalf("vac finalizers=%v", vac.Finalizers)
	}
	claim, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "claim", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinalizer(claim.Finalizers, pvcProtectionFinalizer) {
		t.Fatalf("claim finalizers=%v", claim.Finalizers)
	}
}

func TestReleaseVolumeAttributesClassKeepsPVAndStatusReferences(t *testing.T) {
	now := metav1.Now()
	current := "current"
	target := "target"
	pvName := "on-pv"
	client := fake.NewSimpleClientset(
		&storagev1.VolumeAttributesClass{ObjectMeta: metav1.ObjectMeta{Name: "current", DeletionTimestamp: &now, Finalizers: []string{vacProtectionFinalizer}}},
		&storagev1.VolumeAttributesClass{ObjectMeta: metav1.ObjectMeta{Name: "target", DeletionTimestamp: &now, Finalizers: []string{vacProtectionFinalizer}}},
		&storagev1.VolumeAttributesClass{ObjectMeta: metav1.ObjectMeta{Name: "on-pv", DeletionTimestamp: &now, Finalizers: []string{vacProtectionFinalizer}}},
		&v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "default"}, Status: v1.PersistentVolumeClaimStatus{
			CurrentVolumeAttributesClassName: &current,
			ModifyVolumeStatus:               &v1.ModifyVolumeStatus{TargetVolumeAttributesClassName: target},
		}},
		&v1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "vol"}, Spec: v1.PersistentVolumeSpec{VolumeAttributesClassName: &pvName}},
	)
	if err := releaseVolumeAttributesClasses(context.Background(), client, []string{"persistentvolumes"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"current", "target", "on-pv"} {
		got, err := client.StorageV1().VolumeAttributesClasses().Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !hasFinalizer(got.Finalizers, vacProtectionFinalizer) {
			t.Fatalf("%s finalizer was removed", name)
		}
	}
}
