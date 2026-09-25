package workloads

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
)

func observeWrites(client kubernetes.Interface, all []loadedSource) kubernetes.Interface {
	var volumes, claims cache.Indexer
	for _, l := range all {
		switch l.informer.example.(type) {
		case *corev1.PersistentVolume:
			volumes = l.informer.GetIndexer()
		case *corev1.PersistentVolumeClaim:
			claims = l.informer.GetIndexer()
		}
	}
	if volumes == nil && claims == nil {
		return client
	}
	return &observeClient{Interface: client, volumes: volumes, claims: claims}
}

func remember(indexer cache.Indexer, obj runtime.Object) {
	if indexer == nil || obj == nil {
		return
	}
	if err := indexer.Update(obj); err != nil {
		_ = indexer.Add(obj)
	}
}

type observeClient struct {
	kubernetes.Interface
	volumes cache.Indexer
	claims  cache.Indexer
}

func (c *observeClient) CoreV1() corev1client.CoreV1Interface {
	return observeCore{CoreV1Interface: c.Interface.CoreV1(), volumes: c.volumes, claims: c.claims}
}

type observeCore struct {
	corev1client.CoreV1Interface
	volumes cache.Indexer
	claims  cache.Indexer
}

func (c observeCore) PersistentVolumes() corev1client.PersistentVolumeInterface {
	return observeVolumes{PersistentVolumeInterface: c.CoreV1Interface.PersistentVolumes(), index: c.volumes}
}

func (c observeCore) PersistentVolumeClaims(namespace string) corev1client.PersistentVolumeClaimInterface {
	return observeClaims{PersistentVolumeClaimInterface: c.CoreV1Interface.PersistentVolumeClaims(namespace), index: c.claims}
}

type observeVolumes struct {
	corev1client.PersistentVolumeInterface
	index cache.Indexer
}

func (v observeVolumes) Update(ctx context.Context, volume *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	got, err := v.PersistentVolumeInterface.Update(ctx, volume, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}

func (v observeVolumes) UpdateStatus(ctx context.Context, volume *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	got, err := v.PersistentVolumeInterface.UpdateStatus(ctx, volume, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}

type observeClaims struct {
	corev1client.PersistentVolumeClaimInterface
	index cache.Indexer
}

func (v observeClaims) Update(ctx context.Context, claim *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	got, err := v.PersistentVolumeClaimInterface.Update(ctx, claim, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}

func (v observeClaims) UpdateStatus(ctx context.Context, claim *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	got, err := v.PersistentVolumeClaimInterface.UpdateStatus(ctx, claim, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}
