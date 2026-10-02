package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func sizedNode(name string, labels map[string]string, cpu, memory string, declared ...string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status: corev1.NodeStatus{
			Allocatable:      rl(cpu, memory),
			DeclaredFeatures: declared,
			NodeInfo:         corev1.NodeSystemInfo{KubeletVersion: "v1.36.0"},
		},
	}
}

func resizePod(node string, generation int64, cpu, memory string, mutate func(*corev1.Pod)) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) {
		p.Generation = generation
		p.Spec.NodeName = node
		p.Spec.Containers[0].Resources = corev1.ResourceRequirements{Requests: rl(cpu, memory), Limits: rl(cpu, memory)}
		if mutate != nil {
			mutate(p)
		}
	})
}

func resizeCase(plugin, name string, pod, old *corev1.Pod, cluster ...runtime.Object) diffCase {
	c := podDiffCase(plugin, name, pod)
	c.phase = "validate"
	c.operation = "UPDATE"
	c.subresource = "resize"
	c.oldObject = old
	c.cluster = cluster
	return c
}

func TestDiffPodResize(t *testing.T) {
	const plugin = "PodResize"
	linux := sizedNode("n1", map[string]string{corev1.LabelOSStable: "linux"}, "2", "4Gi")
	unlabeled := sizedNode("n1", nil, "2", "4Gi")
	windows := sizedNode("n1", map[string]string{corev1.LabelOSStable: "windows"}, "2", "4Gi")
	old := resizePod("n1", 1, "500m", "1Gi", nil)
	cases := []diffCase{
		resizeCase(plugin, "resize within allocatable", resizePod("n1", 2, "1", "2Gi", nil), old, linux),
		resizeCase(plugin, "resize to exactly allocatable", resizePod("n1", 2, "2", "4Gi", nil), old, linux),
		resizeCase(plugin, "cpu over allocatable", resizePod("n1", 2, "3", "2Gi", nil), old, linux),
		resizeCase(plugin, "memory over allocatable", resizePod("n1", 2, "1", "8Gi", nil), old, linux),
		resizeCase(plugin, "cpu and memory over allocatable", resizePod("n1", 2, "3", "8Gi", nil), old, linux),
		resizeCase(plugin, "node without an os label", resizePod("n1", 2, "1", "2Gi", nil), old, unlabeled),
		resizeCase(plugin, "windows node", resizePod("n1", 2, "1", "2Gi", nil), old, windows),
		resizeCase(plugin, "node missing", resizePod("n1", 2, "1", "2Gi", nil), old),
		resizeCase(plugin, "generation unchanged", resizePod("n1", 1, "3", "8Gi", nil), old, linux),
		resizeCase(plugin, "pod not bound to a node", resizePod("", 2, "3", "8Gi", nil), resizePod("", 1, "500m", "1Gi", nil), linux),
		resizeCase(plugin, "init container counted", resizePod("n1", 2, "1", "2Gi", func(p *corev1.Pod) {
			p.Spec.InitContainers = []corev1.Container{{Name: "i", Image: "img", Resources: corev1.ResourceRequirements{Requests: rl("3", "")}}}
		}), old, linux),
		resizeCase(plugin, "pod overhead is counted", resizePod("n1", 2, "1", "2Gi", func(p *corev1.Pod) { p.Spec.Overhead = rl("2", "") }), old, linux),
		resizeCase(plugin, "two containers summed", resizePod("n1", 2, "1500m", "2Gi", func(p *corev1.Pod) {
			p.Spec.Containers = append(p.Spec.Containers, corev1.Container{Name: "c2", Image: "img", Resources: corev1.ResourceRequirements{Requests: rl("1", "")}})
		}), old, linux),
	}
	plain := resizeCase(plugin, "update without the resize subresource", resizePod("n1", 2, "3", "8Gi", nil), old, linux)
	plain.subresource = ""
	create := resizeCase(plugin, "create is ignored", resizePod("n1", 2, "3", "8Gi", nil), nil, linux)
	create.operation = "CREATE"
	cases = append(cases, plain, create)
	runDiffCases(t, cases)
}

func TestDiffNodeDeclaredFeatures(t *testing.T) {
	const plugin = "NodeDeclaredFeatures"
	const feature = "InPlacePodLevelResourcesVerticalScaling"
	podLevel := func(generation int64, cpu string, node string) *corev1.Pod {
		return resizePod(node, generation, "500m", "1Gi", func(p *corev1.Pod) {
			p.Spec.Resources = &corev1.ResourceRequirements{Requests: rl(cpu, "2Gi"), Limits: rl(cpu, "2Gi")}
		})
	}
	with := sizedNode("n1", nil, "4", "8Gi", feature)
	without := sizedNode("n1", nil, "4", "8Gi")
	other := sizedNode("n1", nil, "4", "8Gi", "SomethingElse")
	cases := []diffCase{
		resizeCase(plugin, "pod-level resize on a node declaring the feature", podLevel(2, "2", "n1"), podLevel(1, "1", "n1"), with),
		resizeCase(plugin, "pod-level resize on a node without the feature", podLevel(2, "2", "n1"), podLevel(1, "1", "n1"), without),
		resizeCase(plugin, "pod-level resize on a node declaring another feature", podLevel(2, "2", "n1"), podLevel(1, "1", "n1"), other),
		resizeCase(plugin, "pod-level resize with the node missing", podLevel(2, "2", "n1"), podLevel(1, "1", "n1")),
		resizeCase(plugin, "pod-level resources unchanged", podLevel(2, "1", "n1"), podLevel(1, "1", "n1"), without),
		resizeCase(plugin, "generation unchanged", podLevel(1, "2", "n1"), podLevel(1, "1", "n1"), without),
		resizeCase(plugin, "pod not bound", podLevel(2, "2", ""), podLevel(1, "1", ""), without),
		resizeCase(plugin, "no pod-level resources at all", resizePod("n1", 2, "1", "2Gi", nil), resizePod("n1", 1, "500m", "1Gi", nil), without),
		resizeCase(plugin, "pod-level resources added", podLevel(2, "2", "n1"), resizePod("n1", 1, "500m", "1Gi", nil), without),
	}
	plain := resizeCase(plugin, "plain update is also checked", podLevel(2, "2", "n1"), podLevel(1, "1", "n1"), without)
	plain.subresource = ""
	status := resizeCase(plugin, "status subresource is ignored", podLevel(2, "2", "n1"), podLevel(1, "1", "n1"), without)
	status.subresource = "status"
	create := resizeCase(plugin, "create is ignored", podLevel(2, "2", "n1"), nil, without)
	create.operation = "CREATE"
	cases = append(cases, plain, status, create)
	runDiffCases(t, cases)
}
