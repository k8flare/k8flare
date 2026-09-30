package containers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func Validate(pod *corev1.Pod, declared []string) (map[string]string, error) {
	if len(pod.Spec.Containers) != 1 {
		return nil, fmt.Errorf("exactly one container is supported, got %d", len(pod.Spec.Containers))
	}
	if len(pod.Spec.InitContainers) > 0 {
		return nil, fmt.Errorf("initContainers are not supported")
	}
	if len(pod.Spec.EphemeralContainers) > 0 {
		return nil, fmt.Errorf("ephemeralContainers are not supported")
	}
	if pod.Spec.HostNetwork {
		return nil, fmt.Errorf("hostNetwork is not supported")
	}
	if pod.Spec.HostPID {
		return nil, fmt.Errorf("hostPID is not supported")
	}
	if pod.Spec.HostIPC {
		return nil, fmt.Errorf("hostIPC is not supported")
	}
	container := pod.Spec.Containers[0]
	if sc := container.SecurityContext; sc != nil && sc.Privileged != nil && *sc.Privileged {
		return nil, fmt.Errorf("privileged containers are not supported")
	}
	for _, v := range pod.Spec.Volumes {
		return nil, fmt.Errorf("volume %q (%s) is not supported: no files are placed in the container; ConfigMap and Secret values are available through env and envFrom", v.Name, volumeKind(v))
	}
	image, err := ResolveImage(container.Image, declared)
	if err != nil {
		return nil, err
	}
	instance, err := InstanceFor(pod)
	if err != nil {
		return nil, err
	}
	return map[string]string{ImageAnnotation: image, InstanceAnnotation: instance}, nil
}

func volumeKind(v corev1.Volume) string {
	switch {
	case v.HostPath != nil:
		return "hostPath"
	case v.ConfigMap != nil:
		return "configMap"
	case v.Secret != nil:
		return "secret"
	case v.EmptyDir != nil:
		return "emptyDir"
	case v.Projected != nil:
		return "projected"
	case v.DownwardAPI != nil:
		return "downwardAPI"
	case v.PersistentVolumeClaim != nil:
		return "persistentVolumeClaim"
	}
	return "volume"
}
