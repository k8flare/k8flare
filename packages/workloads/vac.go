package workloads

import (
	"context"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/component-helpers/storage/ephemeral"
)

const vacProtectionFinalizer = "kubernetes.io/vac-protection"

func releaseVolumeAttributesClasses(ctx context.Context, client kubernetes.Interface, changed []string) error {
	if len(changed) > 0 && !changedHas(changed, "volumeattributesclasses") && !changedHas(changed, "storage.k8s.io") && !changedHas(changed, "persistentvolumeclaims") && !changedHas(changed, "persistentvolumes") {
		return nil
	}
	vacs, err := client.StorageV1().VolumeAttributesClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	pvcs, err := client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	used := map[string]bool{}
	if pvcs != nil {
		for i := range pvcs.Items {
			markVolumeAttributesClass(used, pvcs.Items[i].Spec.VolumeAttributesClassName)
			markVolumeAttributesClass(used, pvcs.Items[i].Status.CurrentVolumeAttributesClassName)
			if status := pvcs.Items[i].Status.ModifyVolumeStatus; status != nil && status.TargetVolumeAttributesClassName != "" {
				used[status.TargetVolumeAttributesClassName] = true
			}
		}
	}
	pvs, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if pvs != nil {
		for i := range pvs.Items {
			markVolumeAttributesClass(used, pvs.Items[i].Spec.VolumeAttributesClassName)
		}
	}
	for i := range vacs.Items {
		vac := &vacs.Items[i]
		if vac.DeletionTimestamp == nil || used[vac.Name] || !hasFinalizer(vac.Finalizers, vacProtectionFinalizer) {
			continue
		}
		vac.Finalizers = dropFinalizer(vac.Finalizers, vacProtectionFinalizer)
		if _, err := client.StorageV1().VolumeAttributesClasses().Update(ctx, vac, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

const (
	pvcProtectionFinalizer = "kubernetes.io/pvc-protection"
	pvProtectionFinalizer  = "kubernetes.io/pv-protection"
)

func markVolumeAttributesClass(used map[string]bool, name *string) {
	if name != nil && *name != "" {
		used[*name] = true
	}
}

func releaseStorageProtection(ctx context.Context, client kubernetes.Interface, changed []string) error {
	if err := releaseDeletingClaims(ctx, client, changed); err != nil {
		return err
	}
	return releaseDeletingVolumes(ctx, client, changed)
}

func releaseDeletingClaims(ctx context.Context, client kubernetes.Interface, changed []string) error {
	if len(changed) > 0 && !changedHas(changed, "persistentvolumeclaims") && !changedHas(changed, "pods") {
		return nil
	}
	claims, err := client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	used := map[string]bool{}
	if pods != nil {
		for i := range pods.Items {
			pod := &pods.Items[i]
			for _, vol := range pod.Spec.Volumes {
				if pod.Spec.NodeName != "" && vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName != "" {
					used[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
				}
				if pod.Spec.NodeName == "" || podIsShutDown(pod) || vol.Ephemeral == nil {
					continue
				}
				claimName := ephemeral.VolumeClaimName(pod, &vol)
				if claimName == "" {
					continue
				}
				used[pod.Namespace+"/"+claimName] = true
			}
		}
	}
	for i := range claims.Items {
		claim := &claims.Items[i]
		if claim.DeletionTimestamp == nil || claimUsed(used, pods, claim) || !hasFinalizer(claim.Finalizers, pvcProtectionFinalizer) {
			continue
		}
		claim.Finalizers = dropFinalizer(claim.Finalizers, pvcProtectionFinalizer)
		if _, err := client.CoreV1().PersistentVolumeClaims(claim.Namespace).Update(ctx, claim, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func claimUsed(used map[string]bool, pods *v1.PodList, claim *v1.PersistentVolumeClaim) bool {
	if used[claim.Namespace+"/"+claim.Name] {
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Namespace != claim.Namespace || pod.Spec.NodeName == "" || podIsShutDown(pod) {
				continue
			}
			for _, vol := range pod.Spec.Volumes {
				if vol.Ephemeral == nil || ephemeral.VolumeClaimName(pod, &vol) != claim.Name {
					continue
				}
				if ephemeral.VolumeIsForPod(pod, claim) == nil {
					return true
				}
			}
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Namespace != claim.Namespace || pod.Spec.NodeName == "" {
				continue
			}
			for _, vol := range pod.Spec.Volumes {
				if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == claim.Name {
					return true
				}
			}
		}
	}
	return false
}

func podIsShutDown(pod *v1.Pod) bool {
	return pod.DeletionTimestamp != nil && pod.DeletionGracePeriodSeconds != nil && *pod.DeletionGracePeriodSeconds == 0
}

func releaseDeletingVolumes(ctx context.Context, client kubernetes.Interface, changed []string) error {
	if len(changed) > 0 && !changedHas(changed, "persistentvolumes") {
		return nil
	}
	volumes, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	for i := range volumes.Items {
		volume := &volumes.Items[i]
		if volume.DeletionTimestamp == nil || volume.Status.Phase == v1.VolumeBound || !hasFinalizer(volume.Finalizers, pvProtectionFinalizer) {
			continue
		}
		volume.Finalizers = dropFinalizer(volume.Finalizers, pvProtectionFinalizer)
		if _, err := client.CoreV1().PersistentVolumes().Update(ctx, volume, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func hasFinalizer(items []string, name string) bool {
	for _, item := range items {
		if item == name {
			return true
		}
	}
	return false
}

func dropFinalizer(items []string, name string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != name {
			out = append(out, item)
		}
	}
	return out
}
