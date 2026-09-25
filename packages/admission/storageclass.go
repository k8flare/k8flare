package admission

import (
	"context"
	"sort"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultStorageClassAnnotation     = "storageclass.kubernetes.io/is-default-class"
	betaDefaultStorageClassAnnotation = "storageclass.beta.kubernetes.io/is-default-class"
	betaPVCStorageClassAnnotation     = "volume.beta.kubernetes.io/storage-class"
)

func applyDefaultStorageClass(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "persistentvolumeclaims" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	if _, ok := spec["storageClassName"]; ok {
		return nil
	}
	if meta, _ := req.Object["metadata"].(map[string]any); meta != nil {
		if anns, _ := meta["annotations"].(map[string]any); anns != nil {
			if _, ok := anns[betaPVCStorageClassAnnotation]; ok {
				return nil
			}
		}
	}
	classes, err := s.storageClasses(ctx)
	if err != nil {
		return err
	}
	var defaults []storagev1.StorageClass
	for _, class := range classes {
		if isDefaultStorageClass(class.ObjectMeta) {
			defaults = append(defaults, class)
		}
	}
	if len(defaults) == 0 {
		return nil
	}
	sort.Slice(defaults, func(i, j int) bool {
		if defaults[i].CreationTimestamp.Equal(&defaults[j].CreationTimestamp) {
			return defaults[i].Name < defaults[j].Name
		}
		return defaults[i].CreationTimestamp.After(defaults[j].CreationTimestamp.Time)
	})
	spec["storageClassName"] = defaults[0].Name
	return nil
}

func isDefaultStorageClass(meta metav1.ObjectMeta) bool {
	if meta.Annotations[defaultStorageClassAnnotation] == "true" {
		return true
	}
	return meta.Annotations[betaDefaultStorageClassAnnotation] == "true"
}
