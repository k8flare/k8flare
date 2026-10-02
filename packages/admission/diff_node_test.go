package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func runtimeClass(name string, overhead corev1.ResourceList, scheduling *nodev1.Scheduling) *nodev1.RuntimeClass {
	rc := &nodev1.RuntimeClass{ObjectMeta: metav1.ObjectMeta{Name: name}, Handler: "h", Scheduling: scheduling}
	if overhead != nil {
		rc.Overhead = &nodev1.Overhead{PodFixed: overhead}
	}
	return rc
}

func runtimeClassPod(className *string, mutate func(*corev1.Pod)) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) {
		p.Spec.RuntimeClassName = className
		if mutate != nil {
			mutate(p)
		}
	})
}

func TestDiffRuntimeClass(t *testing.T) {
	const plugin = "RuntimeClass"
	overhead := rl("100m", "10Mi")
	scheduling := &nodev1.Scheduling{
		NodeSelector: map[string]string{"disk": "ssd"},
		Tolerations:  []corev1.Toleration{{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}},
	}
	plain := runtimeClass("plain", nil, nil)
	withOverhead := runtimeClass("oh", overhead, nil)
	withScheduling := runtimeClass("sched", nil, scheduling)
	both := runtimeClass("both", overhead, scheduling)
	deleting := runtimeClass("deleting", overhead, nil)
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"example.com/f"}

	var cases []diffCase
	add := func(name string, pod *corev1.Pod, cluster ...runtime.Object) {
		cases = append(cases, bothPhases(podDiffCase(plugin, name, pod), cluster...)...)
	}
	add("no runtime class", runtimeClassPod(nil, nil))
	add("no runtime class but overhead set", runtimeClassPod(nil, func(p *corev1.Pod) { p.Spec.Overhead = overhead }))
	add("empty runtime class name", runtimeClassPod(strPtr(""), nil))
	add("runtime class missing", runtimeClassPod(strPtr("gone"), nil), plain)
	add("runtime class without overhead or scheduling", runtimeClassPod(strPtr("plain"), nil), plain)
	add("runtime class with overhead", runtimeClassPod(strPtr("oh"), nil), withOverhead)
	add("matching overhead already set", runtimeClassPod(strPtr("oh"), func(p *corev1.Pod) { p.Spec.Overhead = overhead }), withOverhead)
	add("differing overhead already set", runtimeClassPod(strPtr("oh"), func(p *corev1.Pod) { p.Spec.Overhead = rl("1", "1Gi") }), withOverhead)
	add("overhead set but runtime class defines none", runtimeClassPod(strPtr("plain"), func(p *corev1.Pod) { p.Spec.Overhead = overhead }), plain)
	add("runtime class with node selector and tolerations", runtimeClassPod(strPtr("sched"), nil), withScheduling)
	add("node selector merged with the pod's", runtimeClassPod(strPtr("sched"), func(p *corev1.Pod) { p.Spec.NodeSelector = map[string]string{"zone": "a"} }), withScheduling)
	add("conflicting node selector", runtimeClassPod(strPtr("sched"), func(p *corev1.Pod) { p.Spec.NodeSelector = map[string]string{"disk": "hdd"} }), withScheduling)
	add("same node selector already set", runtimeClassPod(strPtr("sched"), func(p *corev1.Pod) { p.Spec.NodeSelector = map[string]string{"disk": "ssd"} }), withScheduling)
	add("toleration already present", runtimeClassPod(strPtr("sched"), func(p *corev1.Pod) { p.Spec.Tolerations = scheduling.Tolerations }), withScheduling)
	add("pod has other tolerations", runtimeClassPod(strPtr("sched"), func(p *corev1.Pod) {
		p.Spec.Tolerations = []corev1.Toleration{{Key: "other", Operator: corev1.TolerationOpExists}}
	}), withScheduling)
	add("overhead and scheduling together", runtimeClassPod(strPtr("both"), nil), both)
	add("runtime class being deleted", runtimeClassPod(strPtr("deleting"), nil), deleting)

	update := podDiffCase(plugin, "update is ignored", runtimeClassPod(strPtr("gone"), nil))
	update.operation = "UPDATE"
	update.oldObject = runtimeClassPod(strPtr("gone"), nil)
	status := podDiffCase(plugin, "status subresource is ignored", runtimeClassPod(strPtr("gone"), nil))
	status.operation = "UPDATE"
	status.subresource = "status"
	status.oldObject = runtimeClassPod(strPtr("gone"), nil)
	cases = append(cases, bothPhases(update)...)
	cases = append(cases, bothPhases(status)...)
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"empty runtime class name":               {reason: "upstream looks up a non-nil empty runtimeClassName and rejects it as not found, ours treats it as unset (runtimeclass/admission.go prepareObjects)", signature: "outcome: upstream denied 403 Forbidden; ours allowed"},
		"empty runtime class name (validate)":    {reason: "same as the admit case", signature: "outcome: upstream denied 403 Forbidden; ours allowed"},
		"runtime class being deleted":            {reason: "ours rejects a RuntimeClass with a deletionTimestamp as not found, upstream still uses it (runtimeclass/admission.go prepareObjects)", signature: "outcome: upstream allowed; ours denied 403 Forbidden"},
		"runtime class being deleted (validate)": {reason: "both reject but for different reasons: upstream finds the class and reports the overhead mismatch, ours reports it not found (runtimeclass/admission.go validateOverhead)", messageOnly: true},
	})
	runDiffCases(t, cases)
}

func diffNode(name string, labels map[string]string, taints ...corev1.Taint) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       corev1.NodeSpec{Taints: taints},
	}
}

func TestDiffTaintNodesByCondition(t *testing.T) {
	const plugin = "TaintNodesByCondition"
	nodeResource := schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	nodeKind := schema.GroupVersionKind{Version: "v1", Kind: "Node"}
	nodeCase := func(name, op string, node *corev1.Node) diffCase {
		return diffCase{plugin: plugin, name: name, resource: nodeResource, kind: nodeKind, objName: "n", object: node, operation: admissionOperation(op)}
	}
	notReady := corev1.Taint{Key: corev1.TaintNodeNotReady, Effect: corev1.TaintEffectNoSchedule}
	cases := []diffCase{
		nodeCase("node without taints", "CREATE", diffNode("n", nil)),
		nodeCase("node with the not-ready taint", "CREATE", diffNode("n", nil, notReady)),
		nodeCase("node with another taint", "CREATE", diffNode("n", nil, corev1.Taint{Key: "x", Effect: corev1.TaintEffectNoSchedule})),
		nodeCase("node with not-ready taint and a value", "CREATE", diffNode("n", nil, corev1.Taint{Key: corev1.TaintNodeNotReady, Value: "v", Effect: corev1.TaintEffectNoSchedule})),
		nodeCase("node with not-ready taint and NoExecute effect", "CREATE", diffNode("n", nil, corev1.Taint{Key: corev1.TaintNodeNotReady, Effect: corev1.TaintEffectNoExecute})),
		nodeCase("node with not-ready taint among others", "CREATE", diffNode("n", nil, corev1.Taint{Key: "x", Effect: corev1.TaintEffectNoSchedule}, notReady)),
		nodeCase("node update", "UPDATE", diffNode("n", nil)),
		nodeCase("node delete", "DELETE", diffNode("n", nil)),
	}
	status := nodeCase("status subresource", "UPDATE", diffNode("n", nil))
	status.subresource = "status"
	cases = append(cases, status, podDiffCase(plugin, "other resource is ignored", diffPod(nil)))
	runDiffCases(t, cases)
}

func TestDiffPodTopologyLabels(t *testing.T) {
	const plugin = "PodTopologyLabels"
	zoned := diffNode("n1", map[string]string{corev1.LabelTopologyZone: "z1", corev1.LabelTopologyRegion: "r1", "other": "x"})
	zoneOnly := diffNode("n2", map[string]string{corev1.LabelTopologyZone: "z2"})
	empty := diffNode("n3", map[string]string{corev1.LabelTopologyZone: "", corev1.LabelTopologyRegion: "r3"})
	bare := diffNode("n4", nil)
	scheduled := func(node string, labels map[string]string) *corev1.Pod {
		return diffPod(func(p *corev1.Pod) { p.Spec.NodeName = node; p.Labels = labels })
	}
	binding := func(kind, node string, labels map[string]string) *corev1.Binding {
		return &corev1.Binding{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns", Labels: labels}, Target: corev1.ObjectReference{Kind: kind, Name: node}}
	}
	bindingCase := func(name string, b *corev1.Binding, cluster ...runtime.Object) diffCase {
		c := podDiffCase(plugin, name, nil)
		c.subresource = "binding"
		c.kind = schema.GroupVersionKind{Version: "v1", Kind: "Binding"}
		c.object = b
		c.cluster = cluster
		return c
	}
	podCase := func(name, op string, pod *corev1.Pod, cluster ...runtime.Object) diffCase {
		c := podDiffCase(plugin, name, pod)
		c.operation = admissionOperation(op)
		c.cluster = cluster
		return c
	}
	cases := []diffCase{
		podCase("unscheduled pod", "CREATE", scheduled("", nil), zoned),
		podCase("scheduled pod gets zone and region", "CREATE", scheduled("n1", nil), zoned),
		podCase("existing labels are overwritten", "CREATE", scheduled("n1", map[string]string{corev1.LabelTopologyZone: "other", "keep": "me"}), zoned),
		podCase("node with only a zone", "CREATE", scheduled("n2", nil), zoneOnly),
		podCase("node label with an empty value", "CREATE", scheduled("n3", nil), empty),
		podCase("node without topology labels", "CREATE", scheduled("n4", nil), bare),
		podCase("node missing from the cluster", "CREATE", scheduled("gone", nil), zoned),
		podCase("update of a scheduled pod", "UPDATE", scheduled("n1", nil), zoned),
		podCase("delete", "DELETE", scheduled("n1", nil), zoned),
		bindingCase("binding to a node", binding("Node", "n1", nil), zoned),
		bindingCase("binding with existing labels", binding("Node", "n1", map[string]string{corev1.LabelTopologyZone: "other", "keep": "me"}), zoned),
		bindingCase("binding to a node with an empty zone", binding("Node", "n3", nil), empty),
		bindingCase("binding to a missing node", binding("Node", "gone", nil), zoned),
		bindingCase("binding to a non-node target", binding("Other", "n1", nil), zoned),
		bindingCase("binding without a target kind", binding("", "n1", nil), zoned),
	}
	status := podCase("status subresource", "UPDATE", scheduled("n1", nil), zoned)
	status.subresource = "status"
	cases = append(cases, status)
	emptyLabel := &knownDifference{reason: "upstream copies a configured topology label even when its value is empty, ours skips empty values (podtopologylabels/admission.go topologyLabelsForNodeName)", rewrites: []objectRewrite{dropLabels(corev1.LabelTopologyZone)}}
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"node label with an empty value":       emptyLabel,
		"binding to a node with an empty zone": emptyLabel,
		"update of a scheduled pod":            {reason: "upstream handles only Create so an update leaves the labels alone, ours also labels on UPDATE (podtopologylabels/admission.go NewPodTopologyPlugin)", rewrites: []objectRewrite{addLabels(map[string]string{corev1.LabelTopologyRegion: "r1", corev1.LabelTopologyZone: "z1"})}},
	})
	runDiffCases(t, cases)
}

func TestDiffPersistentVolumeClaimResize(t *testing.T) {
	const plugin = "PersistentVolumeClaimResize"
	growable := func(name string, expansion *bool) *storagev1.StorageClass {
		return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: name}, Provisioner: "example.com/p", AllowVolumeExpansion: expansion}
	}
	yes, no := true, false
	sized := func(size string, class *string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
		return diffClaim(func(p *corev1.PersistentVolumeClaim) {
			p.Spec.StorageClassName = class
			p.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: quantity(size)}
			p.Status.Phase = phase
		})
	}
	resizeCase := func(name string, pvc, old *corev1.PersistentVolumeClaim, cluster ...runtime.Object) diffCase {
		c := claimCase(plugin, name, pvc, cluster...)
		c.phase = "validate"
		c.operation = "UPDATE"
		c.oldObject = old
		return c
	}
	grow := growable("grow", &yes)
	cases := []diffCase{
		resizeCase("grow bound claim on an expandable class", sized("2Gi", strPtr("grow"), corev1.ClaimBound), sized("1Gi", strPtr("grow"), corev1.ClaimBound), grow),
		resizeCase("same size", sized("1Gi", strPtr("grow"), corev1.ClaimBound), sized("1Gi", strPtr("grow"), corev1.ClaimBound)),
		resizeCase("shrink", sized("1Gi", strPtr("grow"), corev1.ClaimBound), sized("2Gi", strPtr("grow"), corev1.ClaimBound), grow),
		resizeCase("grow pending claim", sized("2Gi", strPtr("grow"), corev1.ClaimPending), sized("1Gi", strPtr("grow"), corev1.ClaimPending), grow),
		resizeCase("grow claim whose old phase is empty", sized("2Gi", strPtr("grow"), ""), sized("1Gi", strPtr("grow"), ""), grow),
		resizeCase("grow on a class forbidding expansion", sized("2Gi", strPtr("grow"), corev1.ClaimBound), sized("1Gi", strPtr("grow"), corev1.ClaimBound), growable("grow", &no)),
		resizeCase("grow on a class without the expansion field", sized("2Gi", strPtr("grow"), corev1.ClaimBound), sized("1Gi", strPtr("grow"), corev1.ClaimBound), growable("grow", nil)),
		resizeCase("grow with the class missing", sized("2Gi", strPtr("grow"), corev1.ClaimBound), sized("1Gi", strPtr("grow"), corev1.ClaimBound)),
		resizeCase("grow with no class name", sized("2Gi", nil, corev1.ClaimBound), sized("1Gi", nil, corev1.ClaimBound), grow),
		resizeCase("grow with an empty class name", sized("2Gi", strPtr(""), corev1.ClaimBound), sized("1Gi", strPtr(""), corev1.ClaimBound), grow),
		resizeCase("grow while changing the class", sized("2Gi", strPtr("grow"), corev1.ClaimBound), sized("1Gi", strPtr("other"), corev1.ClaimBound), grow, growable("other", &yes)),
		resizeCase("grow with the class from the beta annotation", diffClaim(func(p *corev1.PersistentVolumeClaim) {
			p.Annotations = map[string]string{"volume.beta.kubernetes.io/storage-class": "grow"}
			p.Status.Phase = corev1.ClaimBound
			p.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: quantity("2Gi")}
		}), diffClaim(func(p *corev1.PersistentVolumeClaim) {
			p.Annotations = map[string]string{"volume.beta.kubernetes.io/storage-class": "grow"}
			p.Status.Phase = corev1.ClaimBound
		}), grow),
		resizeCase("grow from no request", diffClaim(func(p *corev1.PersistentVolumeClaim) {
			p.Spec.StorageClassName = strPtr("grow")
			p.Status.Phase = corev1.ClaimBound
		}), diffClaim(func(p *corev1.PersistentVolumeClaim) {
			p.Spec.StorageClassName = strPtr("grow")
			p.Spec.Resources.Requests = nil
			p.Status.Phase = corev1.ClaimBound
		}), grow),
		resizeCase("grow with a fractional milli request", sized("1500m", strPtr("grow"), corev1.ClaimBound), sized("1", strPtr("grow"), corev1.ClaimBound), grow),
	}
	create := resizeCase("create is ignored", sized("2Gi", strPtr("grow"), corev1.ClaimBound), nil)
	create.operation = "CREATE"
	noOld := resizeCase("update without an old object", sized("2Gi", strPtr("grow"), corev1.ClaimBound), nil, grow)
	status := resizeCase("status subresource", sized("2Gi", strPtr("grow"), corev1.ClaimPending), sized("1Gi", strPtr("grow"), corev1.ClaimPending))
	status.subresource = "status"
	cases = append(cases, create, noOld, status)
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"grow with the class from the beta annotation": {reason: "upstream resolves the class through GetPersistentVolumeClaimClass which prefers the volume.beta.kubernetes.io/storage-class annotation over spec.storageClassName, ours reads spec.storageClassName only (pkg/apis/core/helper GetPersistentVolumeClaimClass, resize/admission.go allowResize)", signature: "outcome: upstream allowed; ours denied 403 Forbidden"},
	})
	runDiffCases(t, cases)
}
