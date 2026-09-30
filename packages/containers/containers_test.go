package containers

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func podWith(requests, limits map[corev1.ResourceName]string) *corev1.Pod {
	toList := func(m map[corev1.ResourceName]string) corev1.ResourceList {
		out := corev1.ResourceList{}
		for k, v := range m {
			out[k] = resource.MustParse(v)
		}
		return out
	}
	return &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "app", Image: "cloudflare/debian-trixie",
		Resources: corev1.ResourceRequirements{Requests: toList(requests), Limits: toList(limits)},
	}}}}
}

func TestInstanceFor(t *testing.T) {
	cases := []struct {
		name     string
		requests map[corev1.ResourceName]string
		limits   map[corev1.ResourceName]string
		want     string
		wantErr  string
	}{
		{name: "nothing requested", want: "lite"},
		{name: "tiny", requests: map[corev1.ResourceName]string{"cpu": "50m", "memory": "128Mi"}, want: "lite"},
		{name: "quarter cpu", requests: map[corev1.ResourceName]string{"cpu": "250m", "memory": "1Gi"}, want: "basic"},
		{name: "limit wins", requests: map[corev1.ResourceName]string{"cpu": "50m"}, limits: map[corev1.ResourceName]string{"memory": "3Gi"}, want: "standard-1"},
		{name: "half cpu with big disk", requests: map[corev1.ResourceName]string{"cpu": "500m", "memory": "1Gi", "ephemeral-storage": "5G"}, want: "standard-1"},
		{name: "below 1 vcpu but too much memory", requests: map[corev1.ResourceName]string{"cpu": "500m", "memory": "6Gi"}, wantErr: "largest named instance standard-1"},
		{name: "custom one vcpu", requests: map[corev1.ResourceName]string{"cpu": "1", "memory": "3Gi"}, want: `{"vcpu":1,"memoryMib":3072,"diskMb":2000}`},
		{name: "custom fractional", requests: map[corev1.ResourceName]string{"cpu": "1500m", "memory": "4608Mi", "ephemeral-storage": "8000M"}, want: `{"vcpu":1.5,"memoryMib":4608,"diskMb":8000}`},
		{name: "custom disk floor", requests: map[corev1.ResourceName]string{"cpu": "2", "memory": "8Gi", "ephemeral-storage": "500Mi"}, want: `{"vcpu":2,"memoryMib":8192,"diskMb":2000}`},
		{name: "ratio violated", requests: map[corev1.ResourceName]string{"cpu": "2", "memory": "2Gi"}, wantErr: "below the custom instance minimum of 3072 MiB per vCPU (6144 MiB for 2 vCPU)"},
		{name: "cpu without memory", requests: map[corev1.ResourceName]string{"cpu": "1"}, wantErr: "memory 0 MiB is below"},
		{name: "too much memory", requests: map[corev1.ResourceName]string{"cpu": "4", "memory": "16Gi"}, wantErr: "exceeds the custom instance maximum of 12288 MiB"},
		{name: "too many vcpu", requests: map[corev1.ResourceName]string{"cpu": "5", "memory": "12Gi"}, wantErr: "exceeds the custom instance maximum of 4 vCPU"},
		{name: "too much disk", requests: map[corev1.ResourceName]string{"cpu": "1", "memory": "3Gi", "ephemeral-storage": "30G"}, wantErr: "exceeds the custom instance maximum of 20000 MB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := InstanceFor(podWith(tc.requests, tc.limits))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestResolveImage(t *testing.T) {
	declared := ParseDeclaredImages(`["web","worker"]`)
	digest := "registry.cloudflare.com/acc/repo@sha256:" + strings.Repeat("ab", 32)
	cases := []struct {
		image string
		want  string
		ok    bool
	}{
		{"web", "images/web", true},
		{"cloudflare/debian-trixie", "cloudflare/debian-trixie", true},
		{digest, digest, true},
		{"registry.cloudflare.com/acc/repo:latest", "", false},
		{"docker.io/library/nginx@sha256:" + strings.Repeat("ab", 32), "", false},
		{"nginx", "", false},
	}
	for _, tc := range cases {
		got, err := ResolveImage(tc.image, declared)
		if tc.ok != (err == nil) || got != tc.want {
			t.Fatalf("%s: got %q, %v", tc.image, got, err)
		}
		if err != nil && !strings.Contains(err.Error(), "declared in wrangler.jsonc containers[].images or be digest-pinned") {
			t.Fatalf("message: %v", err)
		}
	}
	if ParseDeclaredImages("") != nil || ParseDeclaredImages("nope") != nil {
		t.Fatal("bad input must parse to nothing")
	}
}

func TestValidateRejectsUnsupportedFields(t *testing.T) {
	yes := true
	cases := []struct {
		name string
		edit func(*corev1.Pod)
		want string
	}{
		{"two containers", func(p *corev1.Pod) { p.Spec.Containers = append(p.Spec.Containers, p.Spec.Containers[0]) }, "exactly one container"},
		{"init", func(p *corev1.Pod) { p.Spec.InitContainers = []corev1.Container{{Name: "i"}} }, "initContainers"},
		{"ephemeral", func(p *corev1.Pod) { p.Spec.EphemeralContainers = []corev1.EphemeralContainer{{}} }, "ephemeralContainers"},
		{"hostNetwork", func(p *corev1.Pod) { p.Spec.HostNetwork = true }, "hostNetwork"},
		{"hostPID", func(p *corev1.Pod) { p.Spec.HostPID = true }, "hostPID"},
		{"hostIPC", func(p *corev1.Pod) { p.Spec.HostIPC = true }, "hostIPC"},
		{"privileged", func(p *corev1.Pod) { p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: &yes} }, "privileged"},
		{"hostPath", func(p *corev1.Pod) {
			p.Spec.Volumes = []corev1.Volume{{Name: "h", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}}}
		}, `volume "h" (hostPath)`},
		{"configMap", func(p *corev1.Pod) {
			p.Spec.Volumes = []corev1.Volume{{Name: "c", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}}}
		}, "env and envFrom"},
		{"projected", func(p *corev1.Pod) {
			p.Spec.Volumes = []corev1.Volume{{Name: "kube-api-access", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{}}}}
		}, `volume "kube-api-access" (projected)`},
		{"image", func(p *corev1.Pod) { p.Spec.Containers[0].Image = "nginx:1.27" }, "digest-pinned"},
		{"shape", func(p *corev1.Pod) {
			p.Spec.Containers[0].Resources.Requests = corev1.ResourceList{"cpu": resource.MustParse("2"), "memory": resource.MustParse("1Gi")}
		}, "3072 MiB per vCPU"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := podWith(nil, nil)
			tc.edit(pod)
			_, err := Validate(pod, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	got, err := Validate(podWith(map[corev1.ResourceName]string{"cpu": "1", "memory": "3Gi"}, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[ImageAnnotation] != "cloudflare/debian-trixie" || got[InstanceAnnotation] != `{"vcpu":1,"memoryMib":3072,"diskMb":2000}` {
		t.Fatalf("annotations = %v", got)
	}
}

func placedPod(name, uid string, requests map[corev1.ResourceName]string) *corev1.Pod {
	pod := podWith(requests, nil)
	pod.Name, pod.Namespace, pod.UID = name, "default", types.UID(uid)
	pod.Spec.SchedulerName = SchedulerName
	pod.Status.Phase = corev1.PodPending
	return pod
}

func TestPlaceBindsFitAndRejectsUnfit(t *testing.T) {
	fit := placedPod("fit", "a", map[corev1.ResourceName]string{"cpu": "1500m", "memory": "4608Mi"})
	unfit := placedPod("unfit", "b", map[corev1.ResourceName]string{"cpu": "2", "memory": "1Gi"})
	other := placedPod("other", "c", nil)
	other.Spec.SchedulerName = "default-scheduler"
	client := fake.NewSimpleClientset(fit, unfit, other)
	ctx := context.Background()

	result, err := Place(ctx, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bound != 1 || result.Rejected != 1 {
		t.Fatalf("result = %+v", result)
	}
	node, err := client.CoreV1().Nodes().Get(ctx, NodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if node.Labels[VirtualNodeLabel] != VirtualNodeType || len(node.Spec.Taints) != 1 || node.Spec.Taints[0].Key != TaintKey {
		t.Fatalf("node = %+v", node)
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			t.Fatalf("virtual node must not publish an InternalIP: %+v", node.Status.Addresses)
		}
	}
	var bound *corev1.Binding
	for _, a := range client.Actions() {
		if a.GetSubresource() == "binding" {
			bound = a.(k8stesting.CreateAction).GetObject().(*corev1.Binding)
		}
	}
	if bound == nil || bound.Name != "fit" || bound.Target.Name != NodeName {
		t.Fatalf("binding = %+v", bound)
	}
	if bound.Annotations[InstanceAnnotation] != `{"vcpu":1.5,"memoryMib":4608,"diskMb":2000}` || bound.Annotations[ImageAnnotation] != "cloudflare/debian-trixie" {
		t.Fatalf("binding annotations = %v", bound.Annotations)
	}

	pending, err := client.CoreV1().Pods("default").Get(ctx, "unfit", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Spec.NodeName != "" {
		t.Fatal("unfit pod was bound")
	}
	var scheduled *corev1.PodCondition
	for i := range pending.Status.Conditions {
		if pending.Status.Conditions[i].Type == corev1.PodScheduled {
			scheduled = &pending.Status.Conditions[i]
		}
	}
	if scheduled == nil || scheduled.Status != corev1.ConditionFalse || scheduled.Reason != corev1.PodReasonUnschedulable || !strings.Contains(scheduled.Message, "3072 MiB per vCPU") {
		t.Fatalf("condition = %+v", scheduled)
	}

	if _, err := Place(ctx, client, nil); err != nil {
		t.Fatal(err)
	}
	events, err := client.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("events = %d, want one aggregated event", len(events.Items))
	}
	ev := events.Items[0]
	if ev.Reason != "FailedScheduling" || ev.Type != corev1.EventTypeWarning || ev.Count != 2 || ev.InvolvedObject.Name != "unfit" || !strings.Contains(ev.Message, "3072 MiB per vCPU") {
		t.Fatalf("event = %+v", ev)
	}
	if nodes, _ := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); len(nodes.Items) != 1 {
		t.Fatalf("nodes = %d", len(nodes.Items))
	}
}
