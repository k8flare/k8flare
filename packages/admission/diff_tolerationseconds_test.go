package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func tolerationPod(tolerations ...corev1.Toleration) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) { p.Spec.Tolerations = tolerations })
}

func TestDiffDefaultTolerationSeconds(t *testing.T) {
	const plugin = "DefaultTolerationSeconds"
	notReadyNoExecute := corev1.Toleration{Key: corev1.TaintNodeNotReady, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute}
	unreachableNoExecute := corev1.Toleration{Key: corev1.TaintNodeUnreachable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute}
	cases := []diffCase{
		podDiffCase(plugin, "no tolerations", tolerationPod()),
		podDiffCase(plugin, "unrelated toleration", tolerationPod(corev1.Toleration{Key: "foo", Operator: corev1.TolerationOpEqual, Value: "bar", Effect: corev1.TaintEffectNoSchedule})),
		podDiffCase(plugin, "unrelated NoExecute toleration", tolerationPod(corev1.Toleration{Key: "foo", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute})),
		podDiffCase(plugin, "not-ready only", tolerationPod(notReadyNoExecute)),
		podDiffCase(plugin, "unreachable only", tolerationPod(unreachableNoExecute)),
		podDiffCase(plugin, "both present", tolerationPod(notReadyNoExecute, unreachableNoExecute)),
		podDiffCase(plugin, "both present with explicit seconds", tolerationPod(
			corev1.Toleration{Key: corev1.TaintNodeNotReady, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: ptr.To(int64(10))},
			corev1.Toleration{Key: corev1.TaintNodeUnreachable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: ptr.To(int64(20))},
		)),
		podDiffCase(plugin, "unreachable key with NoSchedule effect", tolerationPod(corev1.Toleration{Key: corev1.TaintNodeUnreachable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule})),
		podDiffCase(plugin, "unreachable key with empty effect", tolerationPod(corev1.Toleration{Key: corev1.TaintNodeUnreachable, Operator: corev1.TolerationOpExists})),
		podDiffCase(plugin, "wildcard", tolerationPod(corev1.Toleration{Operator: corev1.TolerationOpExists})),
		podDiffCase(plugin, "wildcard key with NoSchedule effect", tolerationPod(corev1.Toleration{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule})),
		podDiffCase(plugin, "wildcard key with NoExecute effect", tolerationPod(corev1.Toleration{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute})),
	}
	update := podDiffCase(plugin, "update restores defaults", tolerationPod())
	update.operation = "UPDATE"
	update.oldObject = tolerationPod(notReadyNoExecute, unreachableNoExecute)
	delete := podDiffCase(plugin, "delete is ignored", tolerationPod())
	delete.operation = "DELETE"
	status := podDiffCase(plugin, "status subresource is ignored", tolerationPod())
	status.subresource = "status"
	status.operation = "UPDATE"
	notPod := podDiffCase(plugin, "other resource is ignored", tolerationPod())
	notPod.resource.Resource = "configmaps"
	cases = append(cases, update, delete, status, notPod)
	runDiffCases(t, cases)
}
