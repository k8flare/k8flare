package localpath

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/k8flare/k8flare/packages/addons"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

var start = time.Unix(1_700_000_000, 0)

const storageRoot = "/var/lib/rancher/k3s/storage/"

func packagedLocalStorage(t *testing.T) []runtime.Object {
	t.Helper()
	for _, f := range addons.Render(addons.Packaged(), addons.Vars()) {
		if f.Name != "local-storage.yaml" {
			continue
		}
		objs, err := addons.Decode(f.Content)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]runtime.Object, len(objs))
		for i, o := range objs {
			out[i] = o
		}
		return out
	}
	t.Fatal("local-storage.yaml is not packaged")
	return nil
}

func toUnstructured(t *testing.T, obj runtime.Object, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	t.Helper()
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatal(err)
	}
	u := &unstructured.Unstructured{Object: m}
	u.SetGroupVersionKind(gvk)
	return u
}

func nodeObject(t *testing.T, name string) *unstructured.Unstructured {
	return toUnstructured(t, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"kubernetes.io/hostname": name}}}, corev1.SchemeGroupVersion.WithKind("Node"))
}

func claimObject(t *testing.T, annotations map[string]string) *unstructured.Unstructured {
	class := "local-path"
	mode := corev1.PersistentVolumeFilesystem
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "app", UID: "u1", ResourceVersion: "7", Annotations: annotations},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &class,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       &mode,
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("16Mi")}},
		},
	}
	return toUnstructured(t, claim, corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"))
}

func selectedClaim(t *testing.T) *unstructured.Unstructured {
	return claimObject(t, map[string]string{annStorageProvisioner: Provisioner, annSelectedNode: "n1"})
}

func newClient(t *testing.T, objs ...runtime.Object) *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		configMapsGVR:     "ConfigMapList",
		nodesGVR:          "NodeList",
		podsGVR:           "PodList",
		claimsGVR:         "PersistentVolumeClaimList",
		volumesGVR:        "PersistentVolumeList",
		storageClassesGVR: "StorageClassList",
		eventsGVR:         "EventList",
	}, append(packagedLocalStorage(t), objs...)...)
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newController(client *fake.FakeDynamicClient) (*Controller, *clock) {
	clk := &clock{now: start}
	return &Controller{Client: client, Now: clk.Now}, clk
}

func reconcile(t *testing.T, c *Controller) time.Time {
	t.Helper()
	wake, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return wake
}

func pods(t *testing.T, client *fake.FakeDynamicClient) []corev1.Pod {
	t.Helper()
	list, err := client.Resource(podsGVR).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]corev1.Pod, len(list.Items))
	for i := range list.Items {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func volumes(t *testing.T, client *fake.FakeDynamicClient) []corev1.PersistentVolume {
	t.Helper()
	list, err := client.Resource(volumesGVR).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]corev1.PersistentVolume, len(list.Items))
	for i := range list.Items {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func finish(t *testing.T, client *fake.FakeDynamicClient, name string, phase corev1.PodPhase, created, finished time.Time) {
	t.Helper()
	u, err := client.Resource(podsGVR).Namespace(metav1.NamespaceSystem).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &pod); err != nil {
		t.Fatal(err)
	}
	pod.CreationTimestamp = metav1.NewTime(created)
	pod.Status.Phase = phase
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "helper-pod", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{FinishedAt: metav1.NewTime(finished)}}}}
	if err := client.Tracker().Update(podsGVR, toUnstructured(t, &pod, corev1.SchemeGroupVersion.WithKind("Pod")), metav1.NamespaceSystem); err != nil {
		t.Fatal(err)
	}
}

func env(c corev1.Container, name string) string {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func hostPathOf(pod corev1.Pod, volume string) string {
	for _, v := range pod.Spec.Volumes {
		if v.Name == volume && v.HostPath != nil {
			return v.HostPath.Path
		}
	}
	return ""
}

func configMapOf(pod corev1.Pod, volume string) string {
	for _, v := range pod.Spec.Volumes {
		if v.Name == volume && v.ConfigMap != nil {
			return v.ConfigMap.Name
		}
	}
	return ""
}

func checkHelper(t *testing.T, pod corev1.Pod, name, script, volDir string) {
	t.Helper()
	c := pod.Spec.Containers[0]
	if pod.Name != name || pod.Namespace != metav1.NamespaceSystem || pod.Spec.NodeName != "n1" {
		t.Fatalf("helper pod is %s/%s on %q, want kube-system/%s on n1", pod.Namespace, pod.Name, pod.Spec.NodeName, name)
	}
	if !slices.Equal(c.Command, []string{"/bin/sh", "/script/" + script}) || c.Image != "rancher/mirrored-library-busybox:1.37.0" {
		t.Fatalf("helper runs %v in %s", c.Command, c.Image)
	}
	if env(c, "VOL_DIR") != volDir || env(c, "VOL_MODE") != "Filesystem" || env(c, "VOL_SIZE_BYTES") != "16777216" {
		t.Fatalf("helper env %v", c.Env)
	}
	if hostPathOf(pod, "data") != storageRoot || configMapOf(pod, "script") != "local-path-config" {
		t.Fatalf("helper volumes %+v", pod.Spec.Volumes)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever || pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("helper restartPolicy %q automountServiceAccountToken %v", pod.Spec.RestartPolicy, pod.Spec.AutomountServiceAccountToken)
	}
	if c.SecurityContext != nil || pod.Spec.SecurityContext != nil {
		t.Fatalf("helper security context %+v %+v, upstream sets none", c.SecurityContext, pod.Spec.SecurityContext)
	}
	wantTolerations := []corev1.Toleration{{Key: "node.kubernetes.io/disk-pressure", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	if !slices.Equal(pod.Spec.Tolerations, wantTolerations) {
		t.Fatalf("helper tolerations %+v, want %+v", pod.Spec.Tolerations, wantTolerations)
	}
	var names []string
	for _, e := range c.Env {
		names = append(names, e.Name)
	}
	if !slices.Equal(names, []string{"VOL_DIR", "VOL_MODE", "VOL_SIZE_BYTES", "VOL_USER", "VOL_GROUP", "VOL_PERM"}) {
		t.Fatalf("helper env names %v", names)
	}
	action := map[string]string{"setup": "create", "teardown": "delete"}[script]
	if !slices.Equal(c.Args, []string{"-p", volDir, "-s", "16777216", "-m", "Filesystem", "-a", action}) {
		t.Fatalf("helper args %v", c.Args)
	}
	var mounts []string
	for _, m := range c.VolumeMounts {
		mounts = append(mounts, m.Name+":"+m.MountPath)
	}
	if !slices.Equal(mounts, []string{"script:/script", "data:" + storageRoot}) {
		t.Fatalf("helper mounts %v", mounts)
	}
	if pod.Spec.PriorityClassName != "" || c.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Fatalf("helper priorityClassName %q imagePullPolicy %q", pod.Spec.PriorityClassName, c.ImagePullPolicy)
	}
}

const setupHelper = "helper-pod-create-pvc-u1"

func TestClaimWithSelectedNodeGetsSetupHelperPodOnThatNode(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	c, _ := newController(client)

	wake := reconcile(t, c)

	got := pods(t, client)
	if len(got) != 1 {
		t.Fatalf("pods %v, want one helper", got)
	}
	checkHelper(t, got[0], setupHelper, "setup", storageRoot+"pvc-u1_app_data")
	if len(volumes(t, client)) != 0 {
		t.Fatal("a volume exists before the helper finished")
	}
	if !wake.Equal(start.Add(helperTimeout)) {
		t.Fatalf("wake %v, want the helper timeout %v", wake, start.Add(helperTimeout))
	}
}

func TestSucceededSetupHelperCreatesTheVolumeAndGoes(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	c, clk := newController(client)
	reconcile(t, c)
	finish(t, client, setupHelper, corev1.PodSucceeded, start, start.Add(time.Second))
	clk.now = start.Add(2 * time.Second)

	if wake := reconcile(t, c); !wake.IsZero() {
		t.Fatalf("wake %v after the volume exists", wake)
	}

	if got := pods(t, client); len(got) != 0 {
		t.Fatalf("helper pods left: %v", got)
	}
	got := volumes(t, client)
	if len(got) != 1 {
		t.Fatalf("volumes %v, want one", got)
	}
	pv := got[0]
	if pv.Name != "pvc-u1" || pv.Annotations[annDynamicallyProvisioned] != Provisioner {
		t.Fatalf("volume %s annotations %v", pv.Name, pv.Annotations)
	}
	if pv.Spec.Local == nil || pv.Spec.Local.Path != storageRoot+"pvc-u1_app_data" || pv.Spec.HostPath != nil {
		t.Fatalf("volume source %+v", pv.Spec.PersistentVolumeSource)
	}
	if pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimDelete || pv.Spec.StorageClassName != "local-path" {
		t.Fatalf("volume reclaim %q class %q", pv.Spec.PersistentVolumeReclaimPolicy, pv.Spec.StorageClassName)
	}
	if !slices.Equal(pv.Spec.AccessModes, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}) || pv.Spec.VolumeMode == nil || *pv.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		t.Fatalf("volume access %v mode %v", pv.Spec.AccessModes, pv.Spec.VolumeMode)
	}
	if size := pv.Spec.Capacity[corev1.ResourceStorage]; size.Cmp(resource.MustParse("16Mi")) != 0 {
		t.Fatalf("volume capacity %v", pv.Spec.Capacity)
	}
	ref := pv.Spec.ClaimRef
	if ref == nil || ref.Kind != "PersistentVolumeClaim" || ref.APIVersion != "v1" || ref.Namespace != "app" || ref.Name != "data" || ref.UID != "u1" || ref.ResourceVersion != "7" {
		t.Fatalf("volume claimRef %+v", ref)
	}
	terms := pv.Spec.NodeAffinity.Required.NodeSelectorTerms
	want := corev1.NodeSelectorRequirement{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"n1"}}
	if len(terms) != 1 || len(terms[0].MatchExpressions) != 1 || terms[0].MatchExpressions[0].Key != want.Key || terms[0].MatchExpressions[0].Operator != want.Operator || !slices.Equal(terms[0].MatchExpressions[0].Values, want.Values) {
		t.Fatalf("volume node affinity %+v", terms)
	}

	reconcile(t, c)
	if len(pods(t, client)) != 0 || len(volumes(t, client)) != 1 {
		t.Fatal("a second pass changed something")
	}
}

func TestFailedHelperIsRetriedWithBackoffAndNoVolume(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	c, clk := newController(client)
	reconcile(t, c)
	finished := start.Add(time.Second)
	finish(t, client, setupHelper, corev1.PodFailed, start, finished)
	clk.now = finished

	if wake := reconcile(t, c); !wake.Equal(finished.Add(5 * time.Second)) {
		t.Fatalf("wake %v, want %v", wake, finished.Add(5*time.Second))
	}
	if got := pods(t, client); len(got) != 1 || got[0].Status.Phase != corev1.PodFailed {
		t.Fatalf("the failed helper was retried before its backoff: %v", got)
	}

	clk.now = finished.Add(5 * time.Second)
	reconcile(t, c)
	got := pods(t, client)
	if len(got) != 1 || got[0].Status.Phase != "" || got[0].Annotations[annFailures] != "1" {
		t.Fatalf("after the backoff, pods %v", got)
	}

	again := clk.now.Add(time.Second)
	finish(t, client, setupHelper, corev1.PodFailed, clk.now, again)
	clk.now = again
	if wake := reconcile(t, c); !wake.Equal(again.Add(10 * time.Second)) {
		t.Fatalf("second failure wake %v, want %v", wake, again.Add(10*time.Second))
	}
	if len(volumes(t, client)) != 0 {
		t.Fatal("a failed helper created a volume")
	}
}

func TestHelperThatOutlivesTheTimeoutIsDeleted(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	c, clk := newController(client)
	reconcile(t, c)
	finish(t, client, setupHelper, corev1.PodRunning, start, time.Time{})
	clk.now = start.Add(helperTimeout)

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("a timed out helper reported no error")
	}
	if got := pods(t, client); len(got) != 0 {
		t.Fatalf("pods %v after the timeout", got)
	}
}

func releasedVolume(t *testing.T) *unstructured.Unstructured {
	mode := corev1.PersistentVolumeFilesystem
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-u1", Annotations: map[string]string{annDynamicallyProvisioned: Provisioner}},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			VolumeMode:                    &mode,
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("16Mi")},
			PersistentVolumeSource:        corev1.PersistentVolumeSource{Local: &corev1.LocalVolumeSource{Path: storageRoot + "pvc-u1_app_data"}},
			NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"n1"}}},
			}}}},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeReleased},
	}
	return toUnstructured(t, pv, corev1.SchemeGroupVersion.WithKind("PersistentVolume"))
}

func TestReleasedVolumeIsTornDownAndDeleted(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), releasedVolume(t))
	c, clk := newController(client)

	reconcile(t, c)
	got := pods(t, client)
	if len(got) != 1 {
		t.Fatalf("pods %v, want one teardown helper", got)
	}
	checkHelper(t, got[0], "helper-pod-delete-pvc-u1", "teardown", storageRoot+"pvc-u1_app_data")
	if len(volumes(t, client)) != 1 {
		t.Fatal("the volume went before its teardown finished")
	}

	finish(t, client, "helper-pod-delete-pvc-u1", corev1.PodSucceeded, start, start.Add(time.Second))
	clk.now = start.Add(2 * time.Second)
	reconcile(t, c)
	if len(pods(t, client)) != 0 || len(volumes(t, client)) != 0 {
		t.Fatalf("after teardown, pods %v volumes %v", pods(t, client), volumes(t, client))
	}
}

func TestClaimWithoutSelectedNodeIsLeftAlone(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), claimObject(t, map[string]string{annStorageProvisioner: Provisioner}))
	c, _ := newController(client)

	if wake := reconcile(t, c); !wake.IsZero() {
		t.Fatalf("wake %v", wake)
	}
	if len(pods(t, client)) != 0 || len(volumes(t, client)) != 0 {
		t.Fatal("a claim with no selected node got a helper or a volume")
	}
}

func TestDisabledAddonDoesNothing(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t), releasedVolume(t))
	c, _ := newController(client)
	c.Disabled = true

	if wake := reconcile(t, c); !wake.IsZero() {
		t.Fatalf("wake %v", wake)
	}
	if len(pods(t, client)) != 0 || len(volumes(t, client)) != 1 {
		t.Fatal("a disabled provisioner acted")
	}
}

func TestHelperWhoseClaimWentIsDeleted(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	c, _ := newController(client)
	reconcile(t, c)
	if err := client.Resource(claimsGVR).Namespace("app").Delete(context.Background(), "data", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}

	reconcile(t, c)

	if _, err := client.Resource(podsGVR).Namespace(metav1.NamespaceSystem).Get(context.Background(), setupHelper, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("the helper of a deleted claim is still there: %v", err)
	}
}
