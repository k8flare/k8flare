package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func rl(cpu, memory string) corev1.ResourceList {
	list := corev1.ResourceList{}
	if cpu != "" {
		list[corev1.ResourceCPU] = quantity(cpu)
	}
	if memory != "" {
		list[corev1.ResourceMemory] = quantity(memory)
	}
	return list
}

func limitRange(name string, items ...corev1.LimitRangeItem) *corev1.LimitRange {
	return &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec:       corev1.LimitRangeSpec{Limits: items},
	}
}

func containerPod(requests, limits corev1.ResourceList) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) {
		p.Spec.Containers[0].Resources = corev1.ResourceRequirements{Requests: requests, Limits: limits}
	})
}

func initAndAppPod(app, init corev1.ResourceRequirements) *corev1.Pod {
	return diffPod(func(p *corev1.Pod) {
		p.Spec.Containers[0].Resources = app
		p.Spec.InitContainers = []corev1.Container{{Name: "init", Image: "img", Resources: init}}
	})
}

func bothPhases(c diffCase, cluster ...runtime.Object) []diffCase {
	c.cluster = cluster
	validate := c
	validate.phase = "validate"
	validate.name = c.name + " (validate)"
	return []diffCase{c, validate}
}

func TestDiffLimitRangerPods(t *testing.T) {
	const plugin = "LimitRanger"
	containerDefaults := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, Default: rl("75m", "10Mi"), DefaultRequest: rl("50m", "5Mi")}
	containerBounds := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, Min: rl("25m", "1Mi"), Max: rl("100m", "2Gi")}
	containerRatio := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, MaxLimitRequestRatio: rl("2", "")}
	podBounds := corev1.LimitRangeItem{Type: corev1.LimitTypePod, Min: rl("50m", "2Mi"), Max: rl("200m", "4Gi")}
	podRatio := corev1.LimitRangeItem{Type: corev1.LimitTypePod, MaxLimitRequestRatio: rl("3", "")}
	defaultLimitOnly := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, Default: rl("80m", "")}
	defaultRequestOnly := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, DefaultRequest: rl("30m", "")}
	storageDefaults := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, Default: corev1.ResourceList{corev1.ResourceEphemeralStorage: quantity("1Gi")}}

	var cases []diffCase
	add := func(name string, pod *corev1.Pod, cluster ...runtime.Object) {
		cases = append(cases, bothPhases(podDiffCase(plugin, name, pod), cluster...)...)
	}
	add("no limit ranges", containerPod(nil, nil))
	add("defaults fill an empty container", containerPod(nil, nil), limitRange("a", containerDefaults))
	add("defaults keep an explicit request", containerPod(rl("10m", ""), nil), limitRange("a", containerDefaults))
	add("defaults keep an explicit limit", containerPod(nil, rl("20m", "")), limitRange("a", containerDefaults))
	add("defaults keep explicit request and limit", containerPod(rl("10m", "1Mi"), rl("20m", "2Mi")), limitRange("a", containerDefaults))
	add("default limit only gives the request the limit value", containerPod(nil, nil), limitRange("a", defaultLimitOnly))
	add("default request only", containerPod(nil, nil), limitRange("a", defaultRequestOnly))
	add("explicit request with a default limit smaller", containerPod(rl("500m", ""), nil), limitRange("a", containerDefaults))
	add("ephemeral storage default", containerPod(nil, nil), limitRange("a", storageDefaults))
	add("init containers get defaults too", initAndAppPod(corev1.ResourceRequirements{}, corev1.ResourceRequirements{}), limitRange("a", containerDefaults))
	add("two limit ranges with disjoint defaults", containerPod(nil, nil), limitRange("a", defaultLimitOnly), limitRange("b", corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, Default: rl("", "10Mi")}))
	add("two container items in one limit range with conflicting defaults", containerPod(nil, nil), limitRange("a", containerDefaults, defaultLimitOnly))
	add("limit range in another namespace is ignored", containerPod(nil, nil), &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "other"}, Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{containerDefaults}}})
	add("pod within container and pod bounds", containerPod(rl("50m", "5Mi"), rl("75m", "10Mi")), limitRange("a", containerBounds, podBounds))
	add("container request below min", containerPod(rl("10m", "5Mi"), rl("75m", "10Mi")), limitRange("a", containerBounds))
	add("container limit below min", containerPod(nil, rl("10m", "10Mi")), limitRange("a", containerBounds))
	add("container limit above max", containerPod(rl("50m", "5Mi"), rl("500m", "10Mi")), limitRange("a", containerBounds))
	add("container request above max with no limit", containerPod(rl("500m", "5Mi"), nil), limitRange("a", containerBounds))
	add("container with no resources against min", containerPod(nil, nil), limitRange("a", containerBounds))
	add("container with no limit against max", containerPod(rl("50m", "5Mi"), nil), limitRange("a", containerBounds))
	add("defaults then bounds satisfied", containerPod(nil, nil), limitRange("a", containerDefaults, containerBounds))
	add("init container above max", initAndAppPod(corev1.ResourceRequirements{Requests: rl("50m", "5Mi"), Limits: rl("75m", "10Mi")}, corev1.ResourceRequirements{Limits: rl("500m", "10Mi")}), limitRange("a", containerBounds))
	add("container ratio satisfied", containerPod(rl("100m", ""), rl("200m", "")), limitRange("a", containerRatio))
	add("container ratio exceeded", containerPod(rl("100m", ""), rl("300m", "")), limitRange("a", containerRatio))
	add("container ratio without request", containerPod(nil, rl("300m", "")), limitRange("a", containerRatio))
	add("container ratio without limit", containerPod(rl("100m", ""), nil), limitRange("a", containerRatio))
	add("pod totals within bounds", diffPod(func(p *corev1.Pod) {
		p.Spec.Containers = []corev1.Container{
			{Name: "a", Image: "i", Resources: corev1.ResourceRequirements{Requests: rl("60m", "3Mi"), Limits: rl("70m", "3Mi")}},
			{Name: "b", Image: "i", Resources: corev1.ResourceRequirements{Requests: rl("60m", "3Mi"), Limits: rl("70m", "3Mi")}},
		}
	}), limitRange("a", podBounds))
	add("pod totals above max", diffPod(func(p *corev1.Pod) {
		p.Spec.Containers = []corev1.Container{
			{Name: "a", Image: "i", Resources: corev1.ResourceRequirements{Requests: rl("60m", "3Mi"), Limits: rl("150m", "3Mi")}},
			{Name: "b", Image: "i", Resources: corev1.ResourceRequirements{Requests: rl("60m", "3Mi"), Limits: rl("150m", "3Mi")}},
		}
	}), limitRange("a", podBounds))
	add("pod totals below min", containerPod(rl("10m", "1Mi"), rl("20m", "1Mi")), limitRange("a", podBounds))
	add("pod totals with an init container taking the maximum", initAndAppPod(
		corev1.ResourceRequirements{Requests: rl("60m", "3Mi"), Limits: rl("100m", "3Mi")},
		corev1.ResourceRequirements{Requests: rl("150m", "3Mi"), Limits: rl("250m", "3Mi")},
	), limitRange("a", podBounds))
	add("pod ratio exceeded", containerPod(rl("100m", ""), rl("400m", "")), limitRange("a", podRatio))
	add("pod ratio satisfied", containerPod(rl("100m", ""), rl("300m", "")), limitRange("a", podRatio))
	add("pod with no resources against pod min", containerPod(nil, nil), limitRange("a", podBounds))
	add("pod with overhead", diffPod(func(p *corev1.Pod) {
		p.Spec.Containers[0].Resources = corev1.ResourceRequirements{Requests: rl("60m", "3Mi"), Limits: rl("70m", "3Mi")}
		p.Spec.Overhead = rl("200m", "")
	}), limitRange("a", podBounds))
	add("pod-level resources", diffPod(func(p *corev1.Pod) {
		p.Spec.Resources = &corev1.ResourceRequirements{Requests: rl("60m", "3Mi"), Limits: rl("70m", "3Mi")}
	}), limitRange("a", podBounds))
	add("unrelated limit type", containerPod(nil, nil), limitRange("a", corev1.LimitRangeItem{Type: corev1.LimitTypePersistentVolumeClaim, Max: corev1.ResourceList{corev1.ResourceStorage: quantity("1Gi")}}))

	updatePod := podDiffCase(plugin, "update of a pod is ignored", containerPod(nil, nil))
	updatePod.operation = "UPDATE"
	updatePod.oldObject = containerPod(nil, nil)
	cases = append(cases, bothPhases(updatePod, limitRange("a", containerDefaults, containerBounds))...)

	resize := podDiffCase(plugin, "resize subresource is checked", containerPod(rl("10m", "5Mi"), rl("75m", "10Mi")))
	resize.operation = "UPDATE"
	resize.subresource = "resize"
	resize.oldObject = containerPod(rl("50m", "5Mi"), rl("75m", "10Mi"))
	cases = append(cases, bothPhases(resize, limitRange("a", containerBounds))...)

	status := podDiffCase(plugin, "status subresource is ignored", containerPod(rl("10m", "5Mi"), rl("75m", "10Mi")))
	status.operation = "UPDATE"
	status.subresource = "status"
	status.oldObject = containerPod(nil, nil)
	cases = append(cases, bothPhases(status, limitRange("a", containerBounds))...)

	terminating := podDiffCase(plugin, "old object being deleted is ignored", containerPod(rl("10m", "5Mi"), rl("75m", "10Mi")))
	terminating.operation = "UPDATE"
	terminating.subresource = "resize"
	terminating.oldObject = diffPod(func(p *corev1.Pod) {
		now := metav1.Now()
		p.DeletionTimestamp = &now
	})
	cases = append(cases, bothPhases(terminating, limitRange("a", containerBounds))...)

	deleted := podDiffCase(plugin, "delete is ignored", containerPod(nil, nil))
	deleted.operation = "DELETE"
	cases = append(cases, bothPhases(deleted, limitRange("a", containerBounds))...)

	annotation := "kubernetes.io/limit-ranger"
	annotationReason := "upstream records what it defaulted in the kubernetes.io/limit-ranger annotation, this project does not (limitranger/admission.go mergePodResourceRequirements)"
	requestsReason := "ours copies a defaulted limit into a missing request, upstream leaves the request empty (limitranger/admission.go mergeContainerResources)"
	noAnnotation := &knownDifference{reason: annotationReason, rewrites: []objectRewrite{dropAnnotation(annotation)}}
	withRequests := &knownDifference{reason: annotationReason + "; " + requestsReason, rewrites: []objectRewrite{dropAnnotation(annotation), requestsFollowLimits}}
	lastItemWins := &knownDifference{reason: annotationReason + "; upstream lets the last container item in a limit range win a conflicting default, ours the first (limitranger/admission.go defaultContainerResourceRequirements)", rewrites: []objectRewrite{dropAnnotation(annotation), cpuLimit("75m")}}
	messageReason := "upstream aggregates every violated constraint into one bracketed message with a double space before No limit/No request, ours returns the first violation with one space (limitranger/admission.go PodValidateLimitFunc)"
	messageOnly := &knownDifference{reason: messageReason, messageOnly: true}
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"defaults fill an empty container":                                 noAnnotation,
		"defaults keep an explicit request":                                noAnnotation,
		"defaults keep an explicit limit":                                  noAnnotation,
		"default limit only gives the request the limit value":             withRequests,
		"default request only":                                             noAnnotation,
		"explicit request with a default limit smaller":                    noAnnotation,
		"ephemeral storage default":                                        withRequests,
		"init containers get defaults too":                                 noAnnotation,
		"two limit ranges with disjoint defaults":                          withRequests,
		"two container items in one limit range with conflicting defaults": lastItemWins,
		"defaults then bounds satisfied":                                   noAnnotation,
		"container request above max with no limit (validate)":             messageOnly,
		"container with no resources against min (validate)":               messageOnly,
		"container with no limit against max (validate)":                   messageOnly,
		"defaults then bounds satisfied (validate)":                        messageOnly,
		"pod totals below min (validate)":                                  messageOnly,
		"pod with no resources against pod min (validate)":                 messageOnly,
		"pod-level resources (validate)":                                   {reason: "ours sums container resources only, upstream uses spec.resources when PodLevelResources is on (limitranger/admission.go PodValidateLimitFunc podRequests)", signature: "outcome: upstream allowed; ours denied 403 Forbidden"},
	})
	runDiffCases(t, cases)
}

func TestDiffLimitRangerClaims(t *testing.T) {
	const plugin = "LimitRanger"
	resourceName := schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	kind := schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"}
	storage := func(q string) corev1.ResourceList {
		if q == "" {
			return nil
		}
		return corev1.ResourceList{corev1.ResourceStorage: quantity(q)}
	}
	claim := func(requests string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "ns"},
			Spec:       corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: storage(requests)}},
		}
	}
	bounds := corev1.LimitRangeItem{Type: corev1.LimitTypePersistentVolumeClaim, Min: storage("1Gi"), Max: storage("10Gi")}
	var cases []diffCase
	add := func(name, op string, pvc *corev1.PersistentVolumeClaim, cluster ...runtime.Object) {
		c := diffCase{plugin: plugin, name: name, resource: resourceName, kind: kind, namespace: "ns", objName: "pvc", object: pvc, operation: admissionOperation(op)}
		cases = append(cases, bothPhases(c, cluster...)...)
	}
	add("claim within bounds", "CREATE", claim("5Gi"), limitRange("a", bounds))
	add("claim below min", "CREATE", claim("500Mi"), limitRange("a", bounds))
	add("claim above max", "CREATE", claim("20Gi"), limitRange("a", bounds))
	add("claim at min", "CREATE", claim("1Gi"), limitRange("a", bounds))
	add("claim at max", "CREATE", claim("10Gi"), limitRange("a", bounds))
	add("claim without a request", "CREATE", claim(""), limitRange("a", bounds))
	add("claim update above max", "UPDATE", claim("20Gi"), limitRange("a", bounds))
	add("claim with only a max", "CREATE", claim(""), limitRange("a", corev1.LimitRangeItem{Type: corev1.LimitTypePersistentVolumeClaim, Max: storage("10Gi")}))
	add("claim with a container-type range", "CREATE", claim("20Gi"), limitRange("a", corev1.LimitRangeItem{Type: corev1.LimitTypeContainer, Max: rl("100m", "")}))
	add("claim with no limit ranges", "CREATE", claim("20Gi"))
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"claim without a request (validate)": {reason: "upstream aggregates min and max violations into one message, ours returns the first (limitranger/admission.go PersistentVolumeClaimValidateLimitFunc)", messageOnly: true},
	})
	runDiffCases(t, cases)
}

func requestsFollowLimits(obj runtime.Object) {
	pod := obj.(*corev1.Pod)
	for _, containers := range [][]corev1.Container{pod.Spec.Containers, pod.Spec.InitContainers} {
		for i := range containers {
			res := &containers[i].Resources
			for name, limit := range res.Limits {
				if _, ok := res.Requests[name]; ok {
					continue
				}
				if res.Requests == nil {
					res.Requests = corev1.ResourceList{}
				}
				res.Requests[name] = limit
			}
		}
	}
}

func cpuLimit(value string) objectRewrite {
	return func(obj runtime.Object) {
		pod := obj.(*corev1.Pod)
		for i := range pod.Spec.Containers {
			res := &pod.Spec.Containers[i].Resources
			res.Limits[corev1.ResourceCPU] = quantity(value)
		}
	}
}
