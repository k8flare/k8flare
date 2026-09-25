package storage

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsStorageClassAndCSIDriver(t *testing.T) {
	class := &storagev1.StorageClass{}
	scheme.Scheme.Default(class)
	if class.ReclaimPolicy == nil || *class.ReclaimPolicy != corev1.PersistentVolumeReclaimDelete {
		t.Fatalf("reclaim=%v", class.ReclaimPolicy)
	}
	if class.VolumeBindingMode == nil || *class.VolumeBindingMode != storagev1.VolumeBindingImmediate {
		t.Fatalf("binding=%v", class.VolumeBindingMode)
	}
	driver := &storagev1.CSIDriver{}
	scheme.Scheme.Default(driver)
	if driver.Spec.AttachRequired == nil || !*driver.Spec.AttachRequired {
		t.Fatalf("attachRequired=%v", driver.Spec.AttachRequired)
	}
	if driver.Spec.FSGroupPolicy == nil || *driver.Spec.FSGroupPolicy != storagev1.ReadWriteOnceWithFSTypeFSGroupPolicy {
		t.Fatalf("fsGroupPolicy=%v", driver.Spec.FSGroupPolicy)
	}
}
