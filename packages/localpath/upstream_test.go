package localpath

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func setConfig(t *testing.T, client *fake.FakeDynamicClient, key, value string) {
	t.Helper()
	u, err := client.Resource(configMapsGVR).Namespace(metav1.NamespaceSystem).Get(context.Background(), configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(u.Object, value, "data", key); err != nil {
		t.Fatal(err)
	}
	if err := client.Tracker().Update(configMapsGVR, u, metav1.NamespaceSystem); err != nil {
		t.Fatal(err)
	}
}

func classObject(t *testing.T, name, binding string, annotations, parameters map[string]string) *unstructured.Unstructured {
	t.Helper()
	reclaim := corev1.PersistentVolumeReclaimDelete
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&storageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: name, Annotations: annotations},
		Provisioner:       Provisioner,
		Parameters:        parameters,
		ReclaimPolicy:     &reclaim,
		VolumeBindingMode: &binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	u := &unstructured.Unstructured{Object: m}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"})
	return u
}

func claimIn(t *testing.T, class string, annotations map[string]string, edit func(*corev1.PersistentVolumeClaim)) *unstructured.Unstructured {
	t.Helper()
	var claim corev1.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(claimObject(t, annotations).Object, &claim); err != nil {
		t.Fatal(err)
	}
	claim.Spec.StorageClassName = &class
	if edit != nil {
		edit(&claim)
	}
	return toUnstructured(t, &claim, corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"))
}

func selectedIn(t *testing.T, class string, edit func(*corev1.PersistentVolumeClaim)) *unstructured.Unstructured {
	return claimIn(t, class, map[string]string{annStorageProvisioner: Provisioner, annSelectedNode: "n1"}, edit)
}

func events(t *testing.T, client *fake.FakeDynamicClient) []corev1.Event {
	t.Helper()
	list, err := client.Resource(eventsGVR).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]corev1.Event, len(list.Items))
	for i := range list.Items {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &out[i]); err != nil {
			t.Fatal(err)
		}
	}
	slices.SortStableFunc(out, func(a, b corev1.Event) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func eventLines(t *testing.T, client *fake.FakeDynamicClient) []string {
	t.Helper()
	var out []string
	for _, e := range events(t, client) {
		out = append(out, e.Type+" "+e.Reason+" "+e.InvolvedObject.Kind+"/"+e.InvolvedObject.Name+": "+e.Message)
	}
	return out
}

func controllerFor(client *fake.FakeDynamicClient) *Controller {
	c, _ := newController(client)
	return c
}

func reconcileErr(client *fake.FakeDynamicClient) error {
	_, err := controllerFor(client).Reconcile(context.Background())
	return err
}

func wantErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v, want it to contain %q", err, want)
	}
}

func provisionWith(t *testing.T, client *fake.FakeDynamicClient) (corev1.Pod, corev1.PersistentVolume) {
	t.Helper()
	c, clk := newController(client)
	reconcile(t, c)
	got := pods(t, client)
	if len(got) != 1 {
		t.Fatalf("pods %v, want one helper", got)
	}
	finish(t, client, got[0].Name, corev1.PodSucceeded, start, start.Add(time.Second))
	clk.now = start.Add(2 * time.Second)
	reconcile(t, c)
	vols := volumes(t, client)
	if len(vols) != 1 {
		t.Fatalf("volumes %v, want one", vols)
	}
	return got[0], vols[0]
}

func TestVolumeCarriesTheSelectedNodeAnnotation(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	_, pv := provisionWith(t, client)
	want := map[string]string{"local.path.provisioner/selected-node": "n1", "pv.kubernetes.io/provisioned-by": Provisioner}
	if !maps.Equal(pv.Annotations, want) || len(pv.Labels) != 0 || len(pv.Spec.MountOptions) != 0 || len(pv.Finalizers) != 0 {
		t.Fatalf("volume annotations %v labels %v mountOptions %v finalizers %v", pv.Annotations, pv.Labels, pv.Spec.MountOptions, pv.Finalizers)
	}
}

func TestVolumeTypeDefaultsToHostPath(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), classObject(t, "plain", "WaitForFirstConsumer", nil, nil), selectedIn(t, "plain", nil))
	_, pv := provisionWith(t, client)
	if pv.Spec.HostPath == nil || pv.Spec.HostPath.Type == nil || *pv.Spec.HostPath.Type != corev1.HostPathDirectoryOrCreate || pv.Spec.Local != nil {
		t.Fatalf("volume source %+v, want hostPath DirectoryOrCreate", pv.Spec.PersistentVolumeSource)
	}

	client = newClient(t, nodeObject(t, "n1"), claimObject(t, map[string]string{annStorageProvisioner: Provisioner, annSelectedNode: "n1", "volumeType": "hostPath"}))
	if _, pv = provisionWith(t, client); pv.Spec.HostPath == nil || pv.Spec.Local != nil {
		t.Fatalf("claim volumeType hostPath gave %+v", pv.Spec.PersistentVolumeSource)
	}
}

func TestPresentButEmptyVolumeTypeIsAnError(t *testing.T) {
	for _, objs := range [][]runtime.Object{
		{claimObject(t, map[string]string{annStorageProvisioner: Provisioner, annSelectedNode: "n1", "volumeType": ""})},
		{classObject(t, "empty", "WaitForFirstConsumer", map[string]string{"defaultVolumeType": ""}, nil), selectedIn(t, "empty", nil)},
	} {
		client := newClient(t, append(objs, nodeObject(t, "n1"))...)
		c, clk := newController(client)
		reconcile(t, c)
		finish(t, client, setupHelper, corev1.PodSucceeded, start, start.Add(time.Second))
		clk.now = start.Add(2 * time.Second)
		_, err := c.Reconcile(context.Background())
		wantErr(t, err, `failed to create persistent volume source: "" is not a recognised volume type`)
		if len(volumes(t, client)) != 0 {
			t.Fatal("a volume exists with an unknown volume type")
		}
	}
}

func TestNodeAffinityKeyParameter(t *testing.T) {
	node := toUnstructured(t, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"topology.example/zone": "z1"}}}, corev1.SchemeGroupVersion.WithKind("Node"))
	client := newClient(t, node, classObject(t, "zoned", "WaitForFirstConsumer", nil, map[string]string{"nodeAffinityKey": "topology.example/zone"}), selectedIn(t, "zoned", nil))
	_, pv := provisionWith(t, client)
	expr := pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0]
	if expr.Key != "topology.example/zone" || expr.Operator != corev1.NodeSelectorOpIn || !slices.Equal(expr.Values, []string{"z1"}) {
		t.Fatalf("node affinity %+v", expr)
	}
}

func TestHelperTemplateTolerationsReplaceTheDefault(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	setConfig(t, client, "helperPod.yaml", "metadata:\n  name: helper-pod\nspec:\n  tolerations:\n  - key: dedicated\n    operator: Exists\n  containers:\n  - name: helper-pod\n    image: busybox\n")
	reconcile(t, controllerFor(client))
	got := pods(t, client)
	if len(got) != 1 || !slices.Equal(got[0].Spec.Tolerations, []corev1.Toleration{{Key: "dedicated", Operator: corev1.TolerationOpExists}}) {
		t.Fatalf("helpers %+v", got)
	}
}

func TestUnsafeHelperTemplateIsRejected(t *testing.T) {
	for template, want := range map[string]string{
		"spec:\n  containers:\n  - name: a\n    image: busybox\n    securityContext:\n      privileged: true\n": "helper pod template must not set privileged: true unless allow-unsafe-helper-pod-template is enabled",
		"spec:\n  serviceAccountName: x\n  containers:\n  - name: a\n    image: busybox\n":                      "helper pod template must not set spec.serviceAccountName unless allow-unsafe-helper-pod-template is enabled",
		"spec:\n  containers:\n  - name: a\n  - name: b\n":                                                      "helper pod template must specify exactly one container",
	} {
		client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
		setConfig(t, client, "helperPod.yaml", template)
		wantErr(t, reconcileErr(client), want)
		if len(pods(t, client)) != 0 {
			t.Fatalf("an unsafe template made a helper: %s", template)
		}
	}
}

func TestSetupCommandAndTimeoutFromConfig(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	setConfig(t, client, "config.json", `{"cmdTimeoutSeconds":30,"setupCommand":"/opt/setup","nodePathMap":[{"node":"DEFAULT_PATH_FOR_NON_LISTED_NODES","paths":["/var/lib/rancher/k3s/storage"]}]}`)
	if wake := reconcile(t, controllerFor(client)); !wake.Equal(start.Add(30 * time.Second)) {
		t.Fatalf("wake %v, want the configured 30 s timeout", wake)
	}
	pod := pods(t, client)[0]
	if !slices.Equal(pod.Spec.Containers[0].Command, []string{"/opt/setup"}) {
		t.Fatalf("command %v", pod.Spec.Containers[0].Command)
	}
	for _, v := range pod.Spec.Volumes {
		if v.ConfigMap != nil && (len(v.ConfigMap.Items) != 1 || v.ConfigMap.Items[0].Key != "teardown") {
			t.Fatalf("script items %+v, want only teardown", v.ConfigMap.Items)
		}
	}

	client = newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	setConfig(t, client, "config.json", `{"setupCommand":"/opt/setup","teardownCommand":"/opt/teardown","nodePathMap":[{"node":"n1","paths":["/x"]}]}`)
	reconcile(t, controllerFor(client))
	pod = pods(t, client)[0]
	for _, v := range pod.Spec.Volumes {
		if v.ConfigMap != nil {
			t.Fatalf("script volume %+v with both commands set", v)
		}
	}
	for _, m := range pod.Spec.Containers[0].VolumeMounts {
		if m.Name == "script" {
			t.Fatalf("script mount %+v with both commands set", m)
		}
	}
}

func TestNodePathParameterAndCanonicalPaths(t *testing.T) {
	config := `{"nodePathMap":[{"node":"n1","paths":["/a/","/b"]}]}`
	client := newClient(t, nodeObject(t, "n1"), classObject(t, "pinned", "WaitForFirstConsumer", nil, map[string]string{"nodePath": "/a"}), selectedIn(t, "pinned", nil))
	setConfig(t, client, "config.json", config)
	pod, pv := provisionWith(t, client)
	if env(pod.Spec.Containers[0], "VOL_DIR") != "/a/pvc-u1_app_data" || pv.Spec.HostPath.Path != "/a/pvc-u1_app_data" {
		t.Fatalf("VOL_DIR %q volume path %q", env(pod.Spec.Containers[0], "VOL_DIR"), pv.Spec.HostPath.Path)
	}

	client = newClient(t, nodeObject(t, "n1"), classObject(t, "pinned", "WaitForFirstConsumer", nil, map[string]string{"nodePath": "/c"}), selectedIn(t, "pinned", nil))
	setConfig(t, client, "config.json", config)
	wantErr(t, reconcileErr(client), "config doesn't contain path /c on node n1")

	for config, want := range map[string]string{
		`{"nodePathMap":[{"node":"n1","paths":["a"]}]}`:               "path must start with / for path a on node n1",
		`{"nodePathMap":[{"node":"n1","paths":["/"]}]}`:               "cannot use root ('/') as path on node n1",
		`{"nodePathMap":[{"node":"n1","paths":["/a","/a/"]}]}`:        "duplicate path /a/ on node n1",
		`{"nodePathMap":[{"node":"n1"},{"node":"n1"}]}`:               "duplicate node n1",
		`{"nodePathMap":[{"node":"n1"}],"sharedFileSystemPath":"/s"}`: "both nodePathMap and sharedFileSystemPath are defined. Please make sure only one is in use",
		`{}`: "both nodePathMap and sharedFileSystemPath are unconfigured",
		`{"nodePathMap":[{"node":"n2","paths":["/a"]}]}`: "config doesn't contain node n1, and no DEFAULT_PATH_FOR_NON_LISTED_NODES available",
		`{"nodePathMap":[{"node":"n1","paths":[]}]}`:     "no local path available on node n1",
	} {
		client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
		setConfig(t, client, "config.json", config)
		wantErr(t, reconcileErr(client), want)
	}
}

func TestStorageClassConfigs(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	setConfig(t, client, "config.json", `{"storageClassConfigs":{"local-path":{"nodePathMap":[{"node":"DEFAULT_PATH_FOR_NON_LISTED_NODES","paths":["/per-class"]}]}}}`)
	if _, pv := provisionWith(t, client); pv.Spec.Local.Path != "/per-class/pvc-u1_app_data" {
		t.Fatalf("volume path %q", pv.Spec.Local.Path)
	}

	client = newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	setConfig(t, client, "config.json", `{"storageClassConfigs":{"other":{"sharedFileSystemPath":"/s"}}}`)
	wantErr(t, reconcileErr(client), "BUG: Got request for unexpected storage class local-path")
}

func TestSharedFileSystemPathNeedsNoNode(t *testing.T) {
	client := newClient(t, classObject(t, "shared", "Immediate", nil, nil),
		claimIn(t, "shared", map[string]string{annStorageProvisioner: Provisioner}, func(c *corev1.PersistentVolumeClaim) {
			c.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		}))
	setConfig(t, client, "config.json", `{"sharedFileSystemPath":"/shared"}`)
	pod, pv := provisionWith(t, client)
	if pod.Spec.NodeName != "" || env(pod.Spec.Containers[0], "VOL_DIR") != "/shared/pvc-u1_app_data" {
		t.Fatalf("helper on %q with VOL_DIR %q", pod.Spec.NodeName, env(pod.Spec.Containers[0], "VOL_DIR"))
	}
	if pv.Spec.NodeAffinity != nil || pv.Spec.HostPath.Path != "/shared/pvc-u1_app_data" || pv.Annotations["local.path.provisioner/selected-node"] != "" {
		t.Fatalf("shared volume %+v", pv)
	}

	client = newClient(t, classObject(t, "shared", "Immediate", nil, nil), volumeEdited(t, func(pv *corev1.PersistentVolume) {
		pv.Spec.StorageClassName = "shared"
		pv.Spec.NodeAffinity = nil
	}))
	setConfig(t, client, "config.json", `{"sharedFileSystemPath":"/shared"}`)
	reconcile(t, controllerFor(client))
	if got := pods(t, client); len(got) != 1 || got[0].Spec.NodeName != "" {
		t.Fatalf("shared teardown helpers %v", got)
	}
}

func TestPathPattern(t *testing.T) {
	cases := []struct {
		params map[string]string
		path   string
		err    string
	}{
		{map[string]string{"pathPattern": "{{ .PVC.Namespace }}/{{ .PVC.Name }}/{{ .PVName }}"}, storageRoot + "app/data/pvc-u1", ""},
		{map[string]string{"pathPattern": "{{ .PVName }}"}, "", "failed to create path from pattern {{ .PVName }}: pathPattern must start with {{ .PVC.Namespace }}/{{ .PVC.Name }}/: pvc-u1"},
		{map[string]string{"pathPattern": "{{ .PVC.Namespace }}/{{ .PVC.Name }}/../../../x"}, "", "folder path contains invalid references: app/data/../../../x"},
		{map[string]string{"pathPattern": "../{{ .PVName }}", "allowUnsafePathPattern": "true"}, "/var/lib/rancher/k3s/pvc-u1", ""},
	}
	for _, tc := range cases {
		client := newClient(t, nodeObject(t, "n1"), classObject(t, "patterned", "WaitForFirstConsumer", nil, tc.params), selectedIn(t, "patterned", nil))
		if tc.err != "" {
			wantErr(t, reconcileErr(client), tc.err)
			continue
		}
		if _, pv := provisionWith(t, client); pv.Spec.HostPath.Path != tc.path {
			t.Fatalf("%v: volume path %q, want %q", tc.params, pv.Spec.HostPath.Path, tc.path)
		}
	}

	client := newClient(t, nodeObject(t, "n1"), classObject(t, "patterned", "WaitForFirstConsumer", map[string]string{"allowUnsafePathPattern": "true"}, map[string]string{"pathPattern": "../{{ .PVName }}"}), selectedIn(t, "patterned", nil))
	if _, pv := provisionWith(t, client); pv.Spec.HostPath.Path != "/var/lib/rancher/k3s/pvc-u1" {
		t.Fatalf("annotation allowUnsafePathPattern: volume path %q", pv.Spec.HostPath.Path)
	}
}

func TestUserGroupPermPatterns(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"),
		classObject(t, "owned", "WaitForFirstConsumer", map[string]string{"groupPattern": `{{ index .PVC.Annotations "group" }}`, "userPattern": "1"}, map[string]string{"userPattern": " 1000 ", "permPattern": "0750"}),
		claimIn(t, "owned", map[string]string{annStorageProvisioner: Provisioner, annSelectedNode: "n1", "group": "2000"}, nil))
	pod, _ := provisionWith(t, client)
	c := pod.Spec.Containers[0]
	if env(c, "VOL_USER") != "1000" || env(c, "VOL_GROUP") != "2000" || env(c, "VOL_PERM") != "0750" {
		t.Fatalf("helper env %v", c.Env)
	}

	client = newClient(t, nodeObject(t, "n1"), classObject(t, "owned", "WaitForFirstConsumer", nil, map[string]string{"permPattern": "{{ .PVC.Annotations.missing }}"}), selectedIn(t, "owned", nil))
	wantErr(t, reconcileErr(client), "failed to create perm from pattern {{ .PVC.Annotations.missing }}")
}

func TestClaimsTheProvisionerRefuses(t *testing.T) {
	cases := []struct {
		edit func(*corev1.PersistentVolumeClaim)
		err  string
	}{
		{func(c *corev1.PersistentVolumeClaim) { c.Spec.Selector = &metav1.LabelSelector{} }, "claim.Spec.Selector is not supported"},
		{func(c *corev1.PersistentVolumeClaim) {
			c.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		}, "NodePath only supports ReadWriteOnce and ReadWriteOncePod (1.22+) access modes"},
	}
	for _, tc := range cases {
		client := newClient(t, nodeObject(t, "n1"), selectedIn(t, "local-path", tc.edit))
		wantErr(t, reconcileErr(client), `failed to provision volume with StorageClass "local-path": `+tc.err)
		want := []string{
			`Normal Provisioning PersistentVolumeClaim/data: External provisioner is provisioning volume for claim "app/data"`,
			`Warning ProvisioningFailed PersistentVolumeClaim/data: failed to provision volume with StorageClass "local-path": ` + tc.err,
		}
		if got := eventLines(t, client); !slices.Equal(got, want) || len(pods(t, client)) != 0 {
			t.Fatalf("events %q, want %q", got, want)
		}
	}

	client := newClient(t, nodeObject(t, "n1"), selectedIn(t, "local-path", func(c *corev1.PersistentVolumeClaim) {
		c.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod}
	}))
	reconcile(t, controllerFor(client))
	if len(pods(t, client)) != 1 {
		t.Fatal("a ReadWriteOncePod claim got no helper")
	}
}

func TestBlockClaimIsRefusedWithAnEventAndNoRetry(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedIn(t, "local-path", func(c *corev1.PersistentVolumeClaim) {
		block := corev1.PersistentVolumeBlock
		c.Spec.VolumeMode = &block
	}))
	if err := reconcileErr(client); err != nil {
		t.Fatal(err)
	}
	want := []string{"Warning ProvisioningFailed PersistentVolumeClaim/data: rancher.io/local-path does not support block volume provisioning"}
	if got := eventLines(t, client); !slices.Equal(got, want) || len(pods(t, client)) != 0 {
		t.Fatalf("events %q pods %v", got, pods(t, client))
	}
}

func TestImmediateClaimWithoutNodeFailsOnNodePaths(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), classObject(t, "now", "Immediate", nil, nil), claimIn(t, "now", map[string]string{annStorageProvisioner: Provisioner}, nil))
	wantErr(t, reconcileErr(client), `failed to provision volume with StorageClass "now": configuration error, no node was specified`)
}

func TestClaimWhoseSelectedNodeIsGoneIsRescheduled(t *testing.T) {
	client := newClient(t, selectedClaim(t))
	if err := reconcileErr(client); err != nil {
		t.Fatal(err)
	}
	u, err := client.Resource(claimsGVR).Namespace("app").Get(context.Background(), "data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := u.GetAnnotations()[annSelectedNode]; ok {
		t.Fatalf("selected-node still set: %v", u.GetAnnotations())
	}
	want := []string{`Warning ProvisioningFailed PersistentVolumeClaim/data: nodes "n1" not found`}
	if got := eventLines(t, client); !slices.Equal(got, want) || len(pods(t, client)) != 0 {
		t.Fatalf("events %q", got)
	}
}

func TestProvisioningEvents(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	provisionWith(t, client)
	want := []string{
		`Normal Provisioning PersistentVolumeClaim/data: External provisioner is provisioning volume for claim "app/data"`,
		"Normal ProvisioningSucceeded PersistentVolumeClaim/data: Successfully provisioned volume pvc-u1",
	}
	if got := eventLines(t, client); !slices.Equal(got, want) {
		t.Fatalf("events %q, want %q", got, want)
	}
	if e := events(t, client)[0]; e.Namespace != "app" || e.InvolvedObject.UID != "u1" || e.InvolvedObject.APIVersion != "v1" || e.Source.Component != Provisioner || e.Count != 1 {
		t.Fatalf("event %+v", e)
	}
}

func TestHelperTimeoutIsReportedLikeUpstream(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), selectedClaim(t))
	c, clk := newController(client)
	reconcile(t, c)
	finish(t, client, setupHelper, corev1.PodRunning, start, time.Time{})
	clk.now = start.Add(helperTimeout)
	_, err := c.Reconcile(context.Background())
	wantErr(t, err, `failed to provision volume with StorageClass "local-path": failed to create volume pvc-u1: create process timeout after 120 seconds`)
}

func volumeEdited(t *testing.T, edit func(*corev1.PersistentVolume)) *unstructured.Unstructured {
	t.Helper()
	var pv corev1.PersistentVolume
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(releasedVolume(t).Object, &pv); err != nil {
		t.Fatal(err)
	}
	pv.Spec.StorageClassName = "local-path"
	edit(&pv)
	return toUnstructured(t, &pv, corev1.SchemeGroupVersion.WithKind("PersistentVolume"))
}

func TestVolumeBeingDeletedIsNotTornDown(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), volumeEdited(t, func(pv *corev1.PersistentVolume) {
		now := metav1.NewTime(start)
		pv.DeletionTimestamp = &now
		pv.Finalizers = []string{"kubernetes.io/pv-protection"}
	}))
	reconcile(t, controllerFor(client))
	if len(pods(t, client)) != 0 {
		t.Fatal("a volume with a deletionTimestamp got a teardown helper")
	}
}

func TestTeardownUsesAnyInAffinityValueAsTheNode(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), volumeEdited(t, func(pv *corev1.PersistentVolume) {
		pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key = "topology.example/zone"
	}))
	reconcile(t, controllerFor(client))
	if got := pods(t, client); len(got) != 1 || got[0].Spec.NodeName != "n1" {
		t.Fatalf("teardown helpers %v", got)
	}
}

func TestVolumeOnAGoneNodeIsDeletedWithoutTeardown(t *testing.T) {
	client := newClient(t, volumeEdited(t, func(*corev1.PersistentVolume) {}))
	reconcile(t, controllerFor(client))
	if len(pods(t, client)) != 0 || len(volumes(t, client)) != 0 {
		t.Fatalf("pods %v volumes %v", pods(t, client), volumes(t, client))
	}
}

func TestFailedDeleteEmitsVolumeFailedDelete(t *testing.T) {
	client := newClient(t, nodeObject(t, "n1"), volumeEdited(t, func(pv *corev1.PersistentVolume) {
		pv.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values = []string{"n1", "n2"}
	}))
	wantErr(t, reconcileErr(client), "failed to delete volume pvc-u1: multiple values for the node affinity")
	want := []string{"Warning VolumeFailedDelete PersistentVolume/pvc-u1: failed to delete volume pvc-u1: multiple values for the node affinity"}
	if got := eventLines(t, client); !slices.Equal(got, want) {
		t.Fatalf("events %q", got)
	}
	if e := events(t, client)[0]; e.Namespace != metav1.NamespaceDefault {
		t.Fatalf("event namespace %q, want default for a cluster-scoped object", e.Namespace)
	}
}
