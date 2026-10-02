package admission

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	podResource = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	podKind     = schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
)

func diffPod(mutate func(*corev1.Pod)) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c", Image: "img"}},
		},
	}
	if mutate != nil {
		mutate(pod)
	}
	return pod
}

func podDiffCase(plugin, name string, pod *corev1.Pod) diffCase {
	return diffCase{
		plugin:    plugin,
		name:      name,
		resource:  podResource,
		kind:      podKind,
		namespace: "ns",
		objName:   "p",
		object:    pod,
	}
}

func quantity(s string) resource.Quantity { return resource.MustParse(s) }
