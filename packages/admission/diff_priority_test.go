package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func priorityClass(name string, value int32, globalDefault bool, policy *corev1.PreemptionPolicy) *schedulingv1.PriorityClass {
	return &schedulingv1.PriorityClass{
		ObjectMeta:       metav1.ObjectMeta{Name: name},
		Value:            value,
		GlobalDefault:    globalDefault,
		PreemptionPolicy: policy,
	}
}

func priorityPod(className string, priority *int32, policy *corev1.PreemptionPolicy) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) {
		p.Spec.PriorityClassName = className
		p.Spec.Priority = priority
		p.Spec.PreemptionPolicy = policy
	})
}

func TestDiffPriorityPods(t *testing.T) {
	const plugin = "Priority"
	never := corev1.PreemptNever
	lower := corev1.PreemptLowerPriority
	high := priorityClass("high", 1000, false, nil)
	low := priorityClass("low", 10, false, nil)
	defaultLow := priorityClass("default-low", 5, true, nil)
	defaultHigh := priorityClass("default-high", 50, true, nil)
	neverClass := priorityClass("never", 300, false, &never)
	defaultNever := priorityClass("default-never", 7, true, &never)

	withCluster := func(c diffCase, objs ...runtime.Object) diffCase {
		c.cluster = objs
		return c
	}
	update := func(c diffCase, old *corev1.Pod) diffCase {
		c.operation = "UPDATE"
		c.oldObject = old
		return c
	}

	cases := []diffCase{
		podDiffCase(plugin, "no class and no default", priorityPod("", nil, nil)),
		withCluster(podDiffCase(plugin, "no class falls back to the global default", priorityPod("", nil, nil)), high, defaultLow),
		withCluster(podDiffCase(plugin, "multiple defaults resolve to the lowest value", priorityPod("", nil, nil)), defaultHigh, defaultLow),
		withCluster(podDiffCase(plugin, "default class with Never policy", priorityPod("", nil, nil)), defaultNever),
		withCluster(podDiffCase(plugin, "named class", priorityPod("high", nil, nil)), high, defaultLow),
		withCluster(podDiffCase(plugin, "named class with Never policy", priorityPod("never", nil, nil)), neverClass),
		withCluster(podDiffCase(plugin, "named class missing", priorityPod("missing", nil, nil)), high),
		podDiffCase(plugin, "named class with empty cluster", priorityPod("high", nil, nil)),
		withCluster(podDiffCase(plugin, "explicit priority equal to the class value", priorityPod("high", ptr.To(int32(1000)), nil)), high),
		withCluster(podDiffCase(plugin, "explicit priority differing from the class value", priorityPod("high", ptr.To(int32(1)), nil)), high),
		withCluster(podDiffCase(plugin, "explicit priority with no class and a default", priorityPod("", ptr.To(int32(1)), nil)), defaultLow),
		podDiffCase(plugin, "explicit zero priority with no class and no default", priorityPod("", ptr.To(int32(0)), nil)),
		podDiffCase(plugin, "explicit nonzero priority with no class and no default", priorityPod("", ptr.To(int32(9)), nil)),
		withCluster(podDiffCase(plugin, "explicit preemption policy equal to the class", priorityPod("never", nil, &never)), neverClass),
		withCluster(podDiffCase(plugin, "explicit preemption policy differing from the class", priorityPod("never", nil, &lower)), neverClass),
		withCluster(podDiffCase(plugin, "explicit preemption policy with a class without one", priorityPod("high", nil, &never)), high),
		podDiffCase(plugin, "explicit Never preemption policy with no class and no default", priorityPod("", nil, &never)),
		withCluster(podDiffCase(plugin, "class name set to the default class name", priorityPod("default-low", nil, nil)), defaultLow, low),
		withCluster(update(podDiffCase(plugin, "update keeps the old priority", priorityPod("high", nil, nil)), priorityPod("high", ptr.To(int32(1000)), &never)), high),
		withCluster(update(podDiffCase(plugin, "update with its own priority", priorityPod("high", ptr.To(int32(5)), nil)), priorityPod("high", ptr.To(int32(1000)), &never)), high),
		withCluster(update(podDiffCase(plugin, "update of a pod that never had a priority", priorityPod("", nil, nil)), priorityPod("", nil, nil)), defaultLow),
		withCluster(podDiffCase(plugin, "system class resolved from the cluster", priorityPod("system-node-critical", nil, nil)), priorityClass("system-node-critical", 2000001000, false, nil)),
		podDiffCase(plugin, "system class absent from the cluster", priorityPod("system-node-critical", nil, nil)),
	}
	status := withCluster(podDiffCase(plugin, "status subresource is ignored", priorityPod("missing", nil, nil)), high)
	status.subresource = "status"
	status.operation = "UPDATE"
	status.oldObject = priorityPod("missing", nil, nil)
	deleted := podDiffCase(plugin, "delete is ignored", priorityPod("missing", nil, nil))
	deleted.operation = "DELETE"
	cases = append(cases, status, deleted)
	runDiffCases(t, cases)
}

func TestDiffPriorityClasses(t *testing.T) {
	const plugin = "Priority"
	resource := schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses"}
	kind := schema.GroupVersionKind{Group: "scheduling.k8s.io", Version: "v1", Kind: "PriorityClass"}
	pcCase := func(name string, op string, obj *schedulingv1.PriorityClass, cluster ...runtime.Object) diffCase {
		c := diffCase{plugin: plugin, name: name, phase: "validate", resource: resource, kind: kind, objName: obj.Name, object: obj, cluster: cluster}
		c.operation = admissionOperation(op)
		if op == "UPDATE" {
			c.oldObject = obj
		}
		return c
	}
	existing := priorityClass("existing", 5, true, nil)
	other := priorityClass("other", 6, false, nil)
	cases := []diffCase{
		pcCase("create a default with none existing", "CREATE", priorityClass("new", 1, true, nil)),
		pcCase("create a default with one existing", "CREATE", priorityClass("new", 1, true, nil), existing),
		pcCase("create a non-default with a default existing", "CREATE", priorityClass("new", 1, false, nil), existing),
		pcCase("update the default itself", "UPDATE", priorityClass("existing", 9, true, nil), existing),
		pcCase("update another class to default", "UPDATE", priorityClass("other", 6, true, nil), existing, other),
		pcCase("update a non-default", "UPDATE", priorityClass("other", 7, false, nil), existing, other),
		pcCase("create a default when two defaults exist", "CREATE", priorityClass("new", 1, true, nil), existing, priorityClass("existing2", 3, true, nil)),
		pcCase("update a default when two defaults exist", "UPDATE", priorityClass("existing", 5, true, nil), existing, priorityClass("existing2", 3, true, nil)),
		pcCase("value above the highest user priority", "CREATE", priorityClass("huge", 2000000000, false, nil)),
		pcCase("delete is ignored", "DELETE", priorityClass("new", 1, true, nil), existing),
	}
	status := pcCase("status subresource is ignored", "UPDATE", priorityClass("new", 1, true, nil), existing)
	status.subresource = "status"
	cases = append(cases, status)
	runDiffCases(t, cases)
}
