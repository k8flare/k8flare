package admission

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func resourceQuota(name string, hard, used corev1.ResourceList, scopes ...corev1.ResourceQuotaScope) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec:       corev1.ResourceQuotaSpec{Hard: hard, Scopes: scopes},
		Status:     corev1.ResourceQuotaStatus{Hard: hard, Used: used},
	}
}

func counts(kv ...string) corev1.ResourceList {
	list := corev1.ResourceList{}
	for i := 0; i+1 < len(kv); i += 2 {
		list[corev1.ResourceName(kv[i])] = quantity(kv[i+1])
	}
	return list
}

func quotaPod(requests, limits corev1.ResourceList, mutate func(*corev1.Pod)) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) {
		p.Spec.Containers[0].Resources = corev1.ResourceRequirements{Requests: requests, Limits: limits}
		if mutate != nil {
			mutate(p)
		}
	})
}

func quotaDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "b"}}, Spec: diffPod(nil).Spec},
		},
	}
}

func TestDiffResourceQuotaPods(t *testing.T) {
	const plugin = "ResourceQuota"
	existing := func(name string, requests corev1.ResourceList) *corev1.Pod {
		pod := quotaPod(requests, nil, nil)
		pod.Name = name
		return pod
	}
	var cases []diffCase
	add := func(name string, pod *corev1.Pod, cluster ...runtime.Object) {
		c := podDiffCase(plugin, name, pod)
		c.phase = "validate"
		c.cluster = cluster
		cases = append(cases, c)
	}
	add("no quota in the namespace", diffPod(nil))
	add("pod count below the limit", diffPod(nil), resourceQuota("q", counts("pods", "5"), counts("pods", "1")))
	add("pod count at the limit", diffPod(nil), resourceQuota("q", counts("pods", "2"), counts("pods", "1")))
	add("pod count over the limit", diffPod(nil), resourceQuota("q", counts("pods", "2"), counts("pods", "2")))
	add("pod count with no recorded usage", diffPod(nil), resourceQuota("q", counts("pods", "1"), nil))
	add("quota tracking an unrelated resource", diffPod(nil), resourceQuota("q", counts("services", "1"), counts("services", "1")))
	add("cpu requests within the limit", quotaPod(counts("cpu", "100m"), nil, nil), resourceQuota("q", counts("requests.cpu", "1"), counts("requests.cpu", "500m")))
	add("cpu requests over the limit", quotaPod(counts("cpu", "600m"), nil, nil), resourceQuota("q", counts("requests.cpu", "1"), counts("requests.cpu", "500m")))
	add("memory requests over the limit", quotaPod(counts("memory", "2Gi"), nil, nil), resourceQuota("q", counts("requests.memory", "1Gi"), counts("requests.memory", "0")))
	add("cpu and memory both over", quotaPod(counts("cpu", "2", "memory", "2Gi"), nil, nil), resourceQuota("q", counts("requests.cpu", "1", "requests.memory", "1Gi"), counts("requests.cpu", "0", "requests.memory", "0")))
	add("limits over the limit", quotaPod(nil, counts("cpu", "3"), nil), resourceQuota("q", counts("limits.cpu", "2"), counts("limits.cpu", "0")))
	add("legacy cpu key over the limit", quotaPod(counts("cpu", "3"), nil, nil), resourceQuota("q", counts("cpu", "2"), counts("cpu", "0")))
	add("pod without the tracked request", quotaPod(nil, nil, nil), resourceQuota("q", counts("requests.cpu", "1"), counts("requests.cpu", "0")))
	add("pod with limit but no request for a tracked request", quotaPod(nil, counts("cpu", "100m"), nil), resourceQuota("q", counts("requests.cpu", "1"), counts("requests.cpu", "0")))
	add("pod without a tracked limit", quotaPod(counts("cpu", "100m"), nil, nil), resourceQuota("q", counts("limits.cpu", "1"), counts("limits.cpu", "0")))
	add("init container request dominating", quotaPod(counts("cpu", "100m"), nil, func(p *corev1.Pod) {
		p.Spec.InitContainers = []corev1.Container{{Name: "i", Image: "img", Resources: corev1.ResourceRequirements{Requests: counts("cpu", "900m")}}}
	}), resourceQuota("q", counts("requests.cpu", "1"), counts("requests.cpu", "500m")))
	add("two containers summed", diffPod(func(p *corev1.Pod) {
		p.Spec.Containers = []corev1.Container{
			{Name: "a", Image: "i", Resources: corev1.ResourceRequirements{Requests: counts("cpu", "300m")}},
			{Name: "b", Image: "i", Resources: corev1.ResourceRequirements{Requests: counts("cpu", "300m")}},
		}
	}), resourceQuota("q", counts("requests.cpu", "1"), counts("requests.cpu", "500m")))
	add("pod overhead counted", quotaPod(counts("cpu", "100m"), nil, func(p *corev1.Pod) { p.Spec.Overhead = counts("cpu", "500m") }), resourceQuota("q", counts("requests.cpu", "500m"), counts("requests.cpu", "0")))
	add("extended resource over the limit", quotaPod(counts("example.com/gpu", "2"), nil, nil), resourceQuota("q", counts("requests.example.com/gpu", "1"), counts("requests.example.com/gpu", "0")))
	add("ephemeral storage over the limit", quotaPod(counts("ephemeral-storage", "2Gi"), nil, nil), resourceQuota("q", counts("requests.ephemeral-storage", "1Gi"), counts("requests.ephemeral-storage", "0")))
	add("two quotas, the second exceeded", quotaPod(counts("cpu", "600m"), nil, nil), resourceQuota("a", counts("requests.cpu", "2"), counts("requests.cpu", "0")), resourceQuota("b", counts("requests.cpu", "1"), counts("requests.cpu", "500m")))
	add("two quotas, both satisfied", quotaPod(counts("cpu", "100m"), nil, nil), resourceQuota("a", counts("requests.cpu", "2"), counts("requests.cpu", "0")), resourceQuota("b", counts("pods", "10"), counts("pods", "1")))
	add("quota in another namespace", diffPod(nil), &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "other"}, Spec: corev1.ResourceQuotaSpec{Hard: counts("pods", "0")}, Status: corev1.ResourceQuotaStatus{Hard: counts("pods", "0"), Used: counts("pods", "0")}})
	add("best-effort scope matches a pod without resources", diffPod(nil), resourceQuota("q", counts("pods", "1"), counts("pods", "1"), corev1.ResourceQuotaScopeBestEffort))
	add("best-effort scope ignores a pod with resources", quotaPod(counts("cpu", "1"), nil, nil), resourceQuota("q", counts("pods", "1"), counts("pods", "1"), corev1.ResourceQuotaScopeBestEffort))
	add("not-best-effort scope matches a pod with resources", quotaPod(counts("cpu", "1"), nil, nil), resourceQuota("q", counts("pods", "1"), counts("pods", "1"), corev1.ResourceQuotaScopeNotBestEffort))
	add("terminating scope matches a pod with a deadline", quotaPod(nil, nil, func(p *corev1.Pod) { p.Spec.ActiveDeadlineSeconds = ptr.To(int64(30)) }), resourceQuota("q", counts("pods", "1"), counts("pods", "1"), corev1.ResourceQuotaScopeTerminating))
	add("terminating scope ignores a pod without a deadline", diffPod(nil), resourceQuota("q", counts("pods", "1"), counts("pods", "1"), corev1.ResourceQuotaScopeTerminating))
	add("not-terminating scope matches a pod without a deadline", diffPod(nil), resourceQuota("q", counts("pods", "1"), counts("pods", "1"), corev1.ResourceQuotaScopeNotTerminating))
	add("priority class scope selector", quotaPod(nil, nil, func(p *corev1.Pod) { p.Spec.PriorityClassName = "high" }), &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"},
		Spec:       corev1.ResourceQuotaSpec{Hard: counts("pods", "1"), ScopeSelector: &corev1.ScopeSelector{MatchExpressions: []corev1.ScopedResourceSelectorRequirement{{ScopeName: corev1.ResourceQuotaScopePriorityClass, Operator: corev1.ScopeSelectorOpIn, Values: []string{"high"}}}}},
		Status:     corev1.ResourceQuotaStatus{Hard: counts("pods", "1"), Used: counts("pods", "1")},
	})
	add("quota with no status hard", diffPod(nil), &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"}, Spec: corev1.ResourceQuotaSpec{Hard: counts("pods", "1")}})
	add("quota whose status hard is stale", diffPod(nil), &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"}, Spec: corev1.ResourceQuotaSpec{Hard: counts("pods", "1")}, Status: corev1.ResourceQuotaStatus{Hard: counts("pods", "5"), Used: counts("pods", "1")}})
	add("status used below the live usage", diffPod(nil), resourceQuota("q", counts("pods", "2"), counts("pods", "0")), existing("a", nil), existing("b", nil))

	dry := podDiffCase(plugin, "dry run does not reserve", diffPod(nil))
	dry.phase = "validate"
	dry.dryRun = true
	dry.cluster = []runtime.Object{resourceQuota("q", counts("pods", "5"), counts("pods", "1"))}

	update := podDiffCase(plugin, "update of a pod without a usage change", diffPod(nil))
	update.phase = "validate"
	update.operation = "UPDATE"
	update.oldObject = diffPod(nil)
	update.cluster = []runtime.Object{resourceQuota("q", counts("pods", "1"), counts("pods", "1")), diffPod(nil)}

	grow := podDiffCase(plugin, "update growing a pod's requests", quotaPod(counts("cpu", "800m"), nil, nil))
	grow.phase = "validate"
	grow.operation = "UPDATE"
	grow.oldObject = quotaPod(counts("cpu", "100m"), nil, nil)
	grow.cluster = []runtime.Object{resourceQuota("q", counts("requests.cpu", "500m"), counts("requests.cpu", "100m")), quotaPod(counts("cpu", "100m"), nil, nil)}

	resize := podDiffCase(plugin, "resize subresource growing requests over the limit", quotaPod(counts("cpu", "800m"), nil, nil))
	resize.phase = "validate"
	resize.operation = "UPDATE"
	resize.subresource = "resize"
	resize.oldObject = quotaPod(counts("cpu", "100m"), nil, nil)
	resize.cluster = []runtime.Object{resourceQuota("q", counts("requests.cpu", "500m"), counts("requests.cpu", "100m")), quotaPod(counts("cpu", "100m"), nil, nil)}

	status := podDiffCase(plugin, "status subresource is ignored", diffPod(nil))
	status.phase = "validate"
	status.operation = "UPDATE"
	status.subresource = "status"
	status.oldObject = diffPod(nil)
	status.cluster = []runtime.Object{resourceQuota("q", counts("pods", "1"), counts("pods", "1"))}

	deleted := podDiffCase(plugin, "delete is ignored", diffPod(nil))
	deleted.phase = "validate"
	deleted.operation = "DELETE"
	deleted.cluster = []runtime.Object{resourceQuota("q", counts("pods", "1"), counts("pods", "1"))}

	cases = append(cases, dry, update, grow, resize, status, deleted)
	updateReason := "upstream's pod evaluator handles a plain Update only when the terminating scope changes, ours charges every Update the pod's full usage again (pkg/quota/v1/evaluator/core/pods.go Handles)"
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"pod count with no recorded usage":                   {reason: "upstream rejects with 'status unknown for quota' while status.used lacks a tracked resource, ours treats it as zero usage (apiserver resourcequota/controller.go CheckRequest)", signature: "outcome: upstream denied 403 Forbidden; ours allowed"},
		"quota with no status hard":                          {reason: "upstream ignores a quota whose status.hard is empty, ours falls back to spec.hard and writes status.used (apiserver resourcequota/controller.go, ours quota.go applyResourceQuota)", signature: "outcome: quota status.used (- upstream, + ours):\n- \"q\": nil,\n+ \"q\": {s\"pods\": {i: resource.int64Amount{value: 1}, s: \"1\", Format: \"DecimalSI\"}},"},
		"status used below the live usage":                   {reason: "ours also counts the live objects in the store and takes the larger of that and status.used, upstream trusts status.used (apiserver resourcequota/controller.go CheckRequest)", signature: "outcome: upstream allowed; ours denied 403 Forbidden"},
		"update of a pod without a usage change":             {reason: updateReason, signature: "outcome: upstream allowed; ours denied 403 Forbidden"},
		"update growing a pod's requests":                    {reason: updateReason, signature: "outcome: upstream allowed; ours denied 403 Forbidden"},
		"resize subresource growing requests over the limit": {reason: "ours skips every subresource, upstream's pod evaluator also charges the resize subresource (pkg/quota/v1/evaluator/core/pods.go Handles)", signature: "outcome: upstream denied 403 Forbidden; ours allowed"},
	})
	runDiffCases(t, cases)
}

func TestDiffResourceQuotaObjects(t *testing.T) {
	const plugin = "ResourceQuota"
	quotaCase := func(name string, gvr schema.GroupVersionResource, kind schema.GroupVersionKind, obj runtime.Object, cluster ...runtime.Object) diffCase {
		return diffCase{plugin: plugin, name: name, phase: "validate", resource: gvr, kind: kind, namespace: "ns", objName: "x", object: obj, cluster: cluster}
	}
	svc := func(t corev1.ServiceType) *corev1.Service {
		return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"}, Spec: corev1.ServiceSpec{Type: t, Ports: []corev1.ServicePort{{Port: 80}}}}
	}
	svcGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	svcKind := schema.GroupVersionKind{Version: "v1", Kind: "Service"}
	cmGVR := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	cmKind := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	secretGVR := schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	secretKind := schema.GroupVersionKind{Version: "v1", Kind: "Secret"}
	claim := func(class *string, size string) *corev1.PersistentVolumeClaim {
		return diffClaim(func(p *corev1.PersistentVolumeClaim) {
			p.Name = "x"
			p.Spec.StorageClassName = class
			p.Spec.Resources.Requests = counts("storage", size)
		})
	}
	cases := []diffCase{
		quotaCase("service count over the limit", svcGVR, svcKind, svc(corev1.ServiceTypeClusterIP), resourceQuota("q", counts("services", "1"), counts("services", "1"))),
		quotaCase("service count within the limit", svcGVR, svcKind, svc(corev1.ServiceTypeClusterIP), resourceQuota("q", counts("services", "2"), counts("services", "1"))),
		quotaCase("load balancer count over the limit", svcGVR, svcKind, svc(corev1.ServiceTypeLoadBalancer), resourceQuota("q", counts("services.loadbalancers", "1"), counts("services.loadbalancers", "1"))),
		quotaCase("load balancer count ignores a cluster IP service", svcGVR, svcKind, svc(corev1.ServiceTypeClusterIP), resourceQuota("q", counts("services.loadbalancers", "1"), counts("services.loadbalancers", "1"))),
		quotaCase("node port count over the limit", svcGVR, svcKind, svc(corev1.ServiceTypeNodePort), resourceQuota("q", counts("services.nodeports", "1"), counts("services.nodeports", "1"))),
		quotaCase("load balancer counts toward node ports", svcGVR, svcKind, svc(corev1.ServiceTypeLoadBalancer), resourceQuota("q", counts("services.nodeports", "1"), counts("services.nodeports", "1"))),
		quotaCase("config map count over the limit", cmGVR, cmKind, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"}}, resourceQuota("q", counts("configmaps", "1"), counts("configmaps", "1"))),
		quotaCase("config map count within the limit", cmGVR, cmKind, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"}}, resourceQuota("q", counts("configmaps", "2"), counts("configmaps", "1"))),
		quotaCase("secret count over the limit", secretGVR, secretKind, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"}}, resourceQuota("q", counts("secrets", "1"), counts("secrets", "1"))),
		quotaCase("generic object count over the limit", schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, quotaDeployment(), resourceQuota("q", counts("count/deployments.apps", "1"), counts("count/deployments.apps", "1"))),
		quotaCase("claim storage over the limit", pvcResource, pvcKind, claim(nil, "5Gi"), resourceQuota("q", counts("requests.storage", "10Gi"), counts("requests.storage", "6Gi"))),
		quotaCase("claim storage within the limit", pvcResource, pvcKind, claim(nil, "1Gi"), resourceQuota("q", counts("requests.storage", "10Gi"), counts("requests.storage", "6Gi"))),
		quotaCase("claim count over the limit", pvcResource, pvcKind, claim(nil, "1Gi"), resourceQuota("q", counts("persistentvolumeclaims", "1"), counts("persistentvolumeclaims", "1"))),
		quotaCase("claim storage by storage class", pvcResource, pvcKind, claim(ptr.To("gold"), "5Gi"), resourceQuota("q", counts("gold.storageclass.storage.k8s.io/requests.storage", "4Gi"), counts("gold.storageclass.storage.k8s.io/requests.storage", "0"))),
		quotaCase("claim of another storage class ignores the class quota", pvcResource, pvcKind, claim(ptr.To("silver"), "5Gi"), resourceQuota("q", counts("gold.storageclass.storage.k8s.io/requests.storage", "4Gi"), counts("gold.storageclass.storage.k8s.io/requests.storage", "0"))),
		quotaCase("claim count by storage class", pvcResource, pvcKind, claim(ptr.To("gold"), "1Gi"), resourceQuota("q", counts("gold.storageclass.storage.k8s.io/persistentvolumeclaims", "1"), counts("gold.storageclass.storage.k8s.io/persistentvolumeclaims", "1"))),
		quotaCase("resource quota count over the limit", schema.GroupVersionResource{Version: "v1", Resource: "resourcequotas"}, schema.GroupVersionKind{Version: "v1", Kind: "ResourceQuota"}, resourceQuota("x", counts("pods", "1"), nil), resourceQuota("q", counts("resourcequotas", "1"), counts("resourcequotas", "1"))),
	}
	nodeObject := quotaCase("cluster-scoped object is ignored", schema.GroupVersionResource{Version: "v1", Resource: "nodes"}, schema.GroupVersionKind{Version: "v1", Kind: "Node"}, diffNode("x", nil))
	nodeObject.namespace = ""
	cases = append(cases, nodeObject)
	runDiffCases(t, cases)
}
