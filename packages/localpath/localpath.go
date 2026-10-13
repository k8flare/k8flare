package localpath

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

const (
	Provisioner = "rancher.io/local-path"
	AddonName   = "local-storage"

	annSelectedNode           = "volume.kubernetes.io/selected-node"
	annDynamicallyProvisioned = "pv.kubernetes.io/provisioned-by"
	annStorageProvisioner     = "volume.kubernetes.io/storage-provisioner"
	annBetaStorageProvisioner = "volume.beta.kubernetes.io/storage-provisioner"
	annNodeName               = "local.path.provisioner/selected-node"
	annFailures               = "k8flare.io/local-path-failures"

	annVolumeType        = "volumeType"
	annDefaultVolumeType = "defaultVolumeType"
	defaultVolumeType    = "hostPath"
	keyNode              = "kubernetes.io/hostname"
	waitForFirstConsumer = "WaitForFirstConsumer"

	configMapName          = "local-path-config"
	defaultPathKey         = "DEFAULT_PATH_FOR_NON_LISTED_NODES"
	helperPodNameMaxLength = 128
	helperScriptDir        = "/script"
	helperDataVolName      = "data"
	helperScriptVolName    = "script"
	actionCreate           = "create"
	actionDelete           = "delete"
	unsafeTemplateFlag     = "allow-unsafe-helper-pod-template"

	helperTimeout  = 120 * time.Second
	failedRetry    = 5 * time.Second
	failedRetryMax = 1000 * time.Second
)

var (
	configMapsGVR     = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	nodesGVR          = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	podsGVR           = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	claimsGVR         = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	volumesGVR        = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}
	storageClassesGVR = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}
	eventsGVR         = schema.GroupVersionResource{Version: "v1", Resource: "events"}
)

type Controller struct {
	Client   dynamic.Interface
	Disabled bool
	Now      func() time.Time
}

type storageClass struct {
	metav1.ObjectMeta `json:"metadata"`
	Provisioner       string                                `json:"provisioner"`
	Parameters        map[string]string                     `json:"parameters,omitempty"`
	ReclaimPolicy     *corev1.PersistentVolumeReclaimPolicy `json:"reclaimPolicy"`
	VolumeBindingMode *string                               `json:"volumeBindingMode"`
}

type classConfigFile struct {
	NodePathMap []struct {
		Node  string   `json:"node,omitempty"`
		Paths []string `json:"paths,omitempty"`
	} `json:"nodePathMap,omitempty"`
	SharedFileSystemPath string `json:"sharedFileSystemPath,omitempty"`
}

type configFile struct {
	CmdTimeoutSeconds int    `json:"cmdTimeoutSeconds,omitempty"`
	SetupCommand      string `json:"setupCommand,omitempty"`
	TeardownCommand   string `json:"teardownCommand,omitempty"`
	classConfigFile
	StorageClassConfigs map[string]classConfigFile `json:"storageClassConfigs"`
}

type classConfig struct {
	nodePaths  map[string][]string
	sharedPath string
}

type config struct {
	timeout         time.Duration
	setupCommand    string
	teardownCommand string
	defaults        classConfig
	classes         map[string]classConfig
	helper          *corev1.Pod
}

type volume struct {
	name  string
	path  string
	mode  corev1.PersistentVolumeMode
	size  int64
	node  string
	user  string
	group string
	perm  string
}

type pass struct {
	wake time.Time
	errs []error
}

func (p *pass) add(until time.Time, err error) {
	if err != nil {
		p.errs = append(p.errs, err)
	}
	if !until.IsZero() && (p.wake.IsZero() || until.Before(p.wake)) {
		p.wake = until
	}
}

func (c *Controller) Reconcile(ctx context.Context) (time.Time, error) {
	if c.Disabled {
		return time.Time{}, nil
	}
	cfg, err := c.config(ctx)
	if apierrors.IsNotFound(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	classes, err := list[storageClass](ctx, c.Client.Resource(storageClassesGVR))
	if err != nil {
		return time.Time{}, err
	}
	claims, err := list[corev1.PersistentVolumeClaim](ctx, c.Client.Resource(claimsGVR).Namespace(metav1.NamespaceAll))
	if err != nil {
		return time.Time{}, err
	}
	volumes, err := list[corev1.PersistentVolume](ctx, c.Client.Resource(volumesGVR))
	if err != nil {
		return time.Time{}, err
	}
	pods, err := list[corev1.Pod](ctx, c.pods())
	if err != nil {
		return time.Time{}, err
	}
	ours := map[string]*storageClass{}
	for i := range classes {
		if classes[i].Provisioner == Provisioner {
			ours[classes[i].Name] = &classes[i]
		}
	}
	provisioned := map[string]bool{}
	for i := range volumes {
		provisioned[volumes[i].Name] = true
	}
	helpers := map[string]*corev1.Pod{}
	for i := range pods {
		if _, ok := pods[i].Annotations[annFailures]; ok {
			helpers[pods[i].Name] = &pods[i]
		}
	}
	var p pass
	wanted := map[string]bool{}
	for i := range claims {
		claim := &claims[i]
		if claim.Spec.StorageClassName == nil || ours[*claim.Spec.StorageClassName] == nil || !needsVolume(claim, ours[*claim.Spec.StorageClassName], provisioned) {
			continue
		}
		name := cfg.helperName(actionCreate, volumeName(claim))
		wanted[name] = true
		until, err := c.provision(ctx, cfg, ours[*claim.Spec.StorageClassName], claim, helpers[name])
		if err != nil {
			err = fmt.Errorf("PersistentVolumeClaim %s/%s: %w", claim.Namespace, claim.Name, err)
		}
		p.add(until, err)
	}
	for i := range volumes {
		pv := &volumes[i]
		if !released(pv) {
			continue
		}
		name := cfg.helperName(actionDelete, pv.Name)
		wanted[name] = true
		until, err := c.teardown(ctx, cfg, pv, helpers[name])
		if err != nil {
			err = fmt.Errorf("failed to delete volume %v: %w", pv.Name, err)
			c.event(ctx, corev1.ObjectReference{Kind: "PersistentVolume", APIVersion: "v1", Name: pv.Name, UID: pv.UID, ResourceVersion: pv.ResourceVersion}, corev1.EventTypeWarning, "VolumeFailedDelete", err.Error())
			err = fmt.Errorf("PersistentVolume %s: %w", pv.Name, err)
		}
		p.add(until, err)
	}
	for name := range helpers {
		if !wanted[name] {
			p.add(time.Time{}, c.deletePod(ctx, name))
		}
	}
	return p.wake, errors.Join(p.errs...)
}

func needsVolume(claim *corev1.PersistentVolumeClaim, class *storageClass, provisioned map[string]bool) bool {
	provisioner, ok := claim.Annotations[annStorageProvisioner]
	if !ok {
		provisioner = claim.Annotations[annBetaStorageProvisioner]
	}
	if claim.Spec.VolumeName != "" || provisioner != Provisioner || provisioned[volumeName(claim)] {
		return false
	}
	if class.VolumeBindingMode != nil && *class.VolumeBindingMode == waitForFirstConsumer {
		return claim.Annotations[annSelectedNode] != ""
	}
	return true
}

func released(pv *corev1.PersistentVolume) bool {
	return pv.Annotations[annDynamicallyProvisioned] == Provisioner &&
		pv.DeletionTimestamp == nil &&
		pv.Status.Phase == corev1.VolumeReleased &&
		pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete
}

func volumeName(claim *corev1.PersistentVolumeClaim) string {
	return "pvc-" + string(claim.UID)
}

func claimRef(claim *corev1.PersistentVolumeClaim) corev1.ObjectReference {
	return corev1.ObjectReference{
		Kind:            "PersistentVolumeClaim",
		APIVersion:      "v1",
		Namespace:       claim.Namespace,
		Name:            claim.Name,
		UID:             claim.UID,
		ResourceVersion: claim.ResourceVersion,
	}
}

func (c *Controller) provision(ctx context.Context, cfg *config, class *storageClass, claim *corev1.PersistentVolumeClaim, helper *corev1.Pod) (time.Time, error) {
	ref := claimRef(claim)
	if modeOf(claim.Spec.VolumeMode) == corev1.PersistentVolumeBlock {
		c.event(ctx, ref, corev1.EventTypeWarning, "ProvisioningFailed", Provisioner+" does not support block volume provisioning")
		return time.Time{}, nil
	}
	var node *corev1.Node
	if nodeName, ok := claim.Annotations[annSelectedNode]; ok {
		n, err := get[corev1.Node](ctx, c.Client.Resource(nodesGVR), nodeName)
		if apierrors.IsNotFound(err) {
			c.event(ctx, ref, corev1.EventTypeWarning, "ProvisioningFailed", err.Error())
			if err := c.reschedule(ctx, claim); err != nil {
				return time.Time{}, fmt.Errorf("delete annotation 'annSelectedNode' for PersistentVolumeClaim %q: %v", claim.Namespace+"/"+claim.Name, err)
			}
			return time.Time{}, nil
		}
		if err != nil {
			err = fmt.Errorf("failed to get target node: %v", err)
			c.event(ctx, ref, corev1.EventTypeWarning, "ProvisioningFailed", err.Error())
			return time.Time{}, err
		}
		node = n
	}
	if helper == nil {
		c.event(ctx, ref, corev1.EventTypeNormal, "Provisioning", fmt.Sprintf("External provisioner is provisioning volume for claim %q", claim.Namespace+"/"+claim.Name))
	}
	until, err := c.provisionFor(ctx, cfg, class, claim, node, helper)
	if err != nil {
		err = fmt.Errorf("failed to provision volume with StorageClass %q: %w", class.Name, err)
		c.event(ctx, ref, corev1.EventTypeWarning, "ProvisioningFailed", err.Error())
	}
	return until, err
}

func (c *Controller) reschedule(ctx context.Context, claim *corev1.PersistentVolumeClaim) error {
	claim = claim.DeepCopy()
	claim.APIVersion, claim.Kind = "v1", "PersistentVolumeClaim"
	delete(claim.Annotations, annSelectedNode)
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(claim)
	if err != nil {
		return err
	}
	_, err = c.Client.Resource(claimsGVR).Namespace(claim.Namespace).Update(ctx, &unstructured.Unstructured{Object: m}, metav1.UpdateOptions{})
	return err
}

func (c *Controller) provisionFor(ctx context.Context, cfg *config, class *storageClass, claim *corev1.PersistentVolumeClaim, node *corev1.Node, helper *corev1.Pod) (time.Time, error) {
	classCfg, err := cfg.pick(class.Name)
	if err != nil {
		return time.Time{}, err
	}
	shared, err := classCfg.shared()
	if err != nil {
		return time.Time{}, err
	}
	if !shared {
		if claim.Spec.Selector != nil {
			return time.Time{}, fmt.Errorf("claim.Spec.Selector is not supported")
		}
		for _, mode := range claim.Spec.AccessModes {
			if mode != corev1.ReadWriteOnce && mode != corev1.ReadWriteOncePod {
				return time.Time{}, fmt.Errorf("NodePath only supports ReadWriteOnce and ReadWriteOncePod (1.22+) access modes")
			}
		}
		if node == nil {
			return time.Time{}, fmt.Errorf("configuration error, no node was specified")
		}
	}
	nodeName := ""
	if node != nil {
		nodeName = node.Name
	}
	name := volumeName(claim)
	base, err := classCfg.pathOn(nodeName, class.Parameters["nodePath"], name)
	if err != nil {
		return time.Time{}, err
	}
	data := patternData{PVName: name, PVC: claim.ObjectMeta}
	folder := strings.Join([]string{name, claim.Namespace, claim.Name}, "_")
	if pattern, ok := class.Parameters["pathPattern"]; ok {
		allowUnsafe := allowUnsafePath(class)
		folder, err = pathFromPattern(pattern, data, allowUnsafe)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to create path from pattern %v: %w", pattern, err)
		}
		if !allowUnsafe && !filepath.IsLocal(folder) {
			return time.Time{}, fmt.Errorf("folder path contains invalid references: %s", folder)
		}
	}
	path := filepath.Join(base, folder)
	owner := map[string]string{}
	for _, kind := range []string{"user", "group", "perm"} {
		pattern, ok := class.Parameters[kind+"Pattern"]
		if !ok {
			pattern, ok = class.Annotations[kind+"Pattern"]
		}
		if !ok {
			continue
		}
		if owner[kind], err = dataFromPattern(pattern, data); err != nil {
			return time.Time{}, fmt.Errorf("failed to create %s from pattern %v: %w", kind, pattern, err)
		}
	}
	size := claim.Spec.Resources.Requests[corev1.ResourceStorage]
	v := volume{name: name, path: path, mode: modeOf(claim.Spec.VolumeMode), size: size.Value(), node: nodeName, user: owner["user"], group: owner["group"], perm: owner["perm"]}
	return c.run(ctx, cfg, actionCreate, v, helper, func() error {
		pv, err := persistentVolume(class, claim, node, shared, name, path)
		if err != nil {
			return err
		}
		if err := create(ctx, c.Client.Resource(volumesGVR), pv); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		c.event(ctx, claimRef(claim), corev1.EventTypeNormal, "ProvisioningSucceeded", fmt.Sprintf("Successfully provisioned volume %s", name))
		return nil
	})
}

type patternData struct {
	PVName string
	PVC    metav1.ObjectMeta
}

func allowUnsafePath(class *storageClass) bool {
	value, ok := class.Parameters["allowUnsafePathPattern"]
	if !ok {
		value = class.Annotations["allowUnsafePathPattern"]
	}
	allow, _ := strconv.ParseBool(value)
	return allow
}

func pathFromPattern(pattern string, data patternData, allowUnsafe bool) (string, error) {
	tpl, err := template.New("pathPattern").Parse(pattern)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, data); err != nil {
		return "", err
	}
	path := buf.String()
	if allowUnsafe {
		return path, nil
	}
	if !strings.HasPrefix(path, filepath.Join(data.PVC.Namespace, data.PVC.Name)+string(filepath.Separator)) {
		return "", fmt.Errorf("pathPattern must start with {{ .PVC.Namespace }}/{{ .PVC.Name }}/: %s", path)
	}
	return path, nil
}

func dataFromPattern(pattern string, data patternData) (string, error) {
	tpl, err := template.New("dataPattern").Option("missingkey=error").Parse(pattern)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func (c *Controller) teardown(ctx context.Context, cfg *config, pv *corev1.PersistentVolume, helper *corev1.Pod) (time.Time, error) {
	classCfg, err := cfg.pick(pv.Spec.StorageClassName)
	if err != nil {
		return time.Time{}, err
	}
	path, node, err := pathAndNode(pv, classCfg)
	if err != nil {
		return time.Time{}, err
	}
	deleteVolume := func() error {
		err := c.Client.Resource(volumesGVR).Delete(ctx, pv.Name, metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if node != "" {
		if _, err := get[corev1.Node](ctx, c.Client.Resource(nodesGVR), node); apierrors.IsNotFound(err) {
			return time.Time{}, deleteVolume()
		}
	}
	size := pv.Spec.Capacity[corev1.ResourceStorage]
	return c.run(ctx, cfg, actionDelete, volume{name: pv.Name, path: path, mode: modeOf(pv.Spec.VolumeMode), size: size.Value(), node: node}, helper, deleteVolume)
}

func (c *Controller) run(ctx context.Context, cfg *config, action string, v volume, have *corev1.Pod, done func() error) (time.Time, error) {
	want, err := cfg.helperPod(action, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to %v volume %v: %w", action, v.name, err)
	}
	if have == nil {
		return c.createHelper(ctx, cfg, want, 0)
	}
	if have.DeletionTimestamp != nil {
		return time.Time{}, nil
	}
	now := c.now()
	switch have.Status.Phase {
	case corev1.PodSucceeded:
		if err := done(); err != nil {
			return time.Time{}, err
		}
		return time.Time{}, c.deletePod(ctx, have.Name)
	case corev1.PodFailed:
		failures, _ := strconv.Atoi(have.Annotations[annFailures])
		retryAt := finishedAt(have).Add(failedRetryDelay(failures))
		if now.Before(retryAt) {
			return retryAt, nil
		}
		if err := c.deletePod(ctx, have.Name); err != nil {
			return time.Time{}, err
		}
		return c.createHelper(ctx, cfg, want, failures+1)
	}
	if deadline := have.CreationTimestamp.Add(cfg.timeout); now.Before(deadline) {
		return deadline, nil
	}
	if err := c.deletePod(ctx, have.Name); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, fmt.Errorf("failed to %v volume %v: create process timeout after %v seconds", action, v.name, int(cfg.timeout.Seconds()))
}

func (c *Controller) createHelper(ctx context.Context, cfg *config, pod *corev1.Pod, failures int) (time.Time, error) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[annFailures] = strconv.Itoa(failures)
	if err := create(ctx, c.pods(), pod); err != nil {
		return time.Time{}, err
	}
	return c.now().Add(cfg.timeout), nil
}

func (c *Controller) deletePod(ctx context.Context, name string) error {
	err := c.pods().Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Controller) pods() dynamic.ResourceInterface {
	return c.Client.Resource(podsGVR).Namespace(metav1.NamespaceSystem)
}

func (c *Controller) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Controller) event(ctx context.Context, ref corev1.ObjectReference, eventType, reason, message string) {
	now := time.Now()
	ns := ref.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}
	stamp := metav1.NewTime(now)
	event := &corev1.Event{
		TypeMeta:       metav1.TypeMeta{APIVersion: "v1", Kind: "Event"},
		ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("%v.%x", ref.Name, now.UnixNano()), Namespace: ns},
		InvolvedObject: ref,
		Reason:         reason,
		Message:        message,
		Source:         corev1.EventSource{Component: Provisioner},
		FirstTimestamp: stamp,
		LastTimestamp:  stamp,
		Count:          1,
		Type:           eventType,
	}
	if err := create(ctx, c.Client.Resource(eventsGVR).Namespace(ns), event); err != nil {
		println("local-path: event create failed:", err.Error())
	}
}

func finishedAt(pod *corev1.Pod) time.Time {
	var latest time.Time
	for _, s := range pod.Status.ContainerStatuses {
		if t := s.State.Terminated; t != nil && t.FinishedAt.After(latest) {
			latest = t.FinishedAt.Time
		}
	}
	if latest.IsZero() {
		return pod.CreationTimestamp.Time
	}
	return latest
}

func failedRetryDelay(failures int) time.Duration {
	delay := failedRetry
	for range failures {
		if delay >= failedRetryMax {
			break
		}
		delay *= 2
	}
	return min(delay, failedRetryMax)
}

func modeOf(mode *corev1.PersistentVolumeMode) corev1.PersistentVolumeMode {
	if mode == nil {
		return corev1.PersistentVolumeFilesystem
	}
	return *mode
}

func (c *Controller) config(ctx context.Context) (*config, error) {
	cm, err := get[corev1.ConfigMap](ctx, c.Client.Resource(configMapsGVR).Namespace(metav1.NamespaceSystem), configMapName)
	if err != nil {
		return nil, err
	}
	var file configFile
	if err := json.Unmarshal([]byte(cm.Data["config.json"]), &file); err != nil {
		return nil, fmt.Errorf("%s config.json: %w", configMapName, err)
	}
	helper := &corev1.Pod{}
	if err := yaml.Unmarshal([]byte(cm.Data["helperPod.yaml"]), helper); err != nil {
		return nil, fmt.Errorf("%s helperPod.yaml: %w", configMapName, err)
	}
	if len(helper.Spec.Containers) == 0 {
		return nil, fmt.Errorf("%s helperPod.yaml has no container", configMapName)
	}
	if err := validateHelperTemplate(helper); err != nil {
		return nil, fmt.Errorf("%s helperPod.yaml: %w", configMapName, err)
	}
	cfg := &config{timeout: helperTimeout, setupCommand: file.SetupCommand, teardownCommand: file.TeardownCommand, helper: helper}
	if file.CmdTimeoutSeconds > 0 {
		cfg.timeout = time.Duration(file.CmdTimeoutSeconds) * time.Second
	}
	if len(file.StorageClassConfigs) == 0 {
		if cfg.defaults, err = canonicalClassConfig(file.classConfigFile); err != nil {
			return nil, fmt.Errorf("%s config.json: %w", configMapName, err)
		}
		return cfg, nil
	}
	cfg.classes = map[string]classConfig{}
	for name, classFile := range file.StorageClassConfigs {
		if cfg.classes[name], err = canonicalClassConfig(classFile); err != nil {
			return nil, fmt.Errorf("%s config.json: config for class %s is invalid: %w", configMapName, name, err)
		}
	}
	return cfg, nil
}

func canonicalClassConfig(file classConfigFile) (classConfig, error) {
	cfg := classConfig{nodePaths: map[string][]string{}, sharedPath: file.SharedFileSystemPath}
	for _, entry := range file.NodePathMap {
		if _, ok := cfg.nodePaths[entry.Node]; ok {
			return cfg, fmt.Errorf("duplicate node %v", entry.Node)
		}
		paths := []string{}
		for _, p := range entry.Paths {
			if !strings.HasPrefix(p, "/") {
				return cfg, fmt.Errorf("path must start with / for path %v on node %v", p, entry.Node)
			}
			path := filepath.Clean(p)
			if path == "/" {
				return cfg, fmt.Errorf("cannot use root ('/') as path on node %v", entry.Node)
			}
			if slices.Contains(paths, path) {
				return cfg, fmt.Errorf("duplicate path %v on node %v", p, entry.Node)
			}
			paths = append(paths, path)
		}
		cfg.nodePaths[entry.Node] = paths
	}
	return cfg, nil
}

func validateHelperTemplate(p *corev1.Pod) error {
	if len(p.Spec.Containers) != 1 {
		return fmt.Errorf("helper pod template must specify exactly one container")
	}
	container := p.Spec.Containers[0]
	forbidden := []struct {
		invalid bool
		field   string
	}{
		{len(p.Spec.InitContainers) > 0, "define initContainers"},
		{len(p.Spec.EphemeralContainers) > 0, "define ephemeralContainers"},
		{len(p.Spec.Volumes) > 0, "define custom volumes"},
		{p.Spec.HostNetwork || p.Spec.HostPID || p.Spec.HostIPC, "enable host namespaces"},
		{p.Spec.NodeName != "", "set spec.nodeName"},
		{p.Spec.ServiceAccountName != "", "set spec.serviceAccountName"},
		{len(container.VolumeMounts) > 0, "define custom volumeMounts"},
		{len(container.EnvFrom) > 0, "define envFrom"},
		{container.Lifecycle != nil, "define container lifecycle hooks"},
		{container.LivenessProbe != nil, "define container livenessProbe"},
		{container.ReadinessProbe != nil, "define container readinessProbe"},
		{container.StartupProbe != nil, "define container startupProbe"},
	}
	for _, env := range container.Env {
		forbidden = append(forbidden, struct {
			invalid bool
			field   string
		}{env.ValueFrom != nil, fmt.Sprintf("define env.valueFrom (env %q)", env.Name)})
	}
	if psc := p.Spec.SecurityContext; psc != nil {
		forbidden = append(forbidden, []struct {
			invalid bool
			field   string
		}{
			{psc.RunAsUser != nil && *psc.RunAsUser == 0, "set pod runAsUser to 0 (root)"},
			{psc.RunAsGroup != nil && *psc.RunAsGroup == 0, "set pod runAsGroup to 0 (root)"},
			{len(psc.Sysctls) > 0, "define sysctls"},
		}...)
	}
	if sc := container.SecurityContext; sc != nil {
		forbidden = append(forbidden, []struct {
			invalid bool
			field   string
		}{
			{sc.Privileged != nil && *sc.Privileged, "set privileged: true"},
			{sc.Capabilities != nil && len(sc.Capabilities.Add) > 0, "add capabilities"},
			{sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation, "set allowPrivilegeEscalation: true"},
		}...)
	}
	for _, check := range forbidden {
		if check.invalid {
			return fmt.Errorf("helper pod template must not %s unless %s is enabled", check.field, unsafeTemplateFlag)
		}
	}
	return nil
}

func (cfg *config) pick(class string) (*classConfig, error) {
	if len(cfg.classes) == 0 {
		return &cfg.defaults, nil
	}
	c, ok := cfg.classes[class]
	if !ok {
		return nil, fmt.Errorf("BUG: Got request for unexpected storage class %s", class)
	}
	return &c, nil
}

func (c *classConfig) shared() (bool, error) {
	if c.sharedPath != "" && len(c.nodePaths) != 0 {
		return false, fmt.Errorf("both nodePathMap and sharedFileSystemPath are defined. Please make sure only one is in use")
	}
	if len(c.nodePaths) != 0 {
		return false, nil
	}
	if c.sharedPath != "" {
		return true, nil
	}
	return false, fmt.Errorf("both nodePathMap and sharedFileSystemPath are unconfigured")
}

func (c *classConfig) pathOn(node, requested, volume string) (string, error) {
	shared, err := c.shared()
	if err != nil {
		return "", err
	}
	if shared {
		return c.sharedPath, nil
	}
	paths, ok := c.nodePaths[node]
	if !ok {
		paths, ok = c.nodePaths[defaultPathKey]
	}
	if !ok {
		return "", fmt.Errorf("config doesn't contain node %v, and no %v available", node, defaultPathKey)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no local path available on node %v", node)
	}
	if requested != "" {
		if !slices.Contains(paths, requested) {
			return "", fmt.Errorf("config doesn't contain path %v on node %v", requested, node)
		}
		return requested, nil
	}
	h := fnv.New32a()
	h.Write([]byte(volume))
	return paths[h.Sum32()%uint32(len(paths))], nil
}

func (cfg *config) helperName(action, volume string) string {
	name := cfg.helper.Name + "-" + action + "-" + volume
	if len(name) > helperPodNameMaxLength {
		name = name[:helperPodNameMaxLength]
	}
	return name
}

func (cfg *config) helperPod(action string, v volume) (*corev1.Pod, error) {
	if !filepath.IsAbs(v.path) {
		return nil, fmt.Errorf("volume path %s is not absolute", v.path)
	}
	parentDir, volumeDir := filepath.Split(filepath.Clean(v.path))
	pod := cfg.helper.DeepCopy()
	pod.APIVersion, pod.Kind = "v1", "Pod"
	pod.Name = cfg.helperName(action, v.name)
	pod.Namespace = metav1.NamespaceSystem
	pod.Spec.NodeName = v.node
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	automount := false
	pod.Spec.AutomountServiceAccountToken = &automount
	if pod.Spec.Tolerations == nil {
		pod.Spec.Tolerations = []corev1.Toleration{{Key: corev1.TaintNodeDiskPressure, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	}
	hostPathType := corev1.HostPathDirectoryOrCreate
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: helperDataVolName, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: parentDir, Type: &hostPathType}}})
	var items []corev1.KeyToPath
	if cfg.setupCommand == "" {
		items = append(items, corev1.KeyToPath{Key: "setup", Path: "setup"})
	}
	if cfg.teardownCommand == "" {
		items = append(items, corev1.KeyToPath{Key: "teardown", Path: "teardown"})
	}
	container := &pod.Spec.Containers[0]
	if len(items) > 0 {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: helperScriptVolName, VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
			Items:                items,
		}}})
		addVolumeMount(container, helperScriptVolName, helperScriptDir).MountPath = helperScriptDir
	}
	dataDir := strings.TrimSuffix(addVolumeMount(container, helperDataVolName, parentDir).MountPath, string(filepath.Separator))
	volumeDir = strings.TrimSuffix(volumeDir, string(filepath.Separator))
	if dataDir == "" || volumeDir == "" || !filepath.IsAbs(dataDir) {
		return nil, fmt.Errorf("invalid path %s for %s: cannot find parent dir or volume dir or parent dir is relative", v.path, action)
	}
	volDir := filepath.Join(dataDir, volumeDir)
	size := strconv.FormatInt(v.size, 10)
	container.Command = []string{"/bin/sh", helperScriptDir + "/setup"}
	if cfg.setupCommand != "" {
		container.Command = []string{cfg.setupCommand}
	}
	if action == actionDelete {
		container.Command = []string{"/bin/sh", helperScriptDir + "/teardown"}
		if cfg.teardownCommand != "" {
			container.Command = []string{cfg.teardownCommand}
		}
	}
	container.Env = append(container.Env,
		corev1.EnvVar{Name: "VOL_DIR", Value: volDir},
		corev1.EnvVar{Name: "VOL_MODE", Value: string(v.mode)},
		corev1.EnvVar{Name: "VOL_SIZE_BYTES", Value: size},
		corev1.EnvVar{Name: "VOL_USER", Value: v.user},
		corev1.EnvVar{Name: "VOL_GROUP", Value: v.group},
		corev1.EnvVar{Name: "VOL_PERM", Value: v.perm},
	)
	container.Args = []string{"-p", volDir, "-s", size, "-m", string(v.mode), "-a", action}
	return pod, nil
}

func addVolumeMount(container *corev1.Container, name, mountPath string) *corev1.VolumeMount {
	for i := range container.VolumeMounts {
		if container.VolumeMounts[i].Name == name {
			return &container.VolumeMounts[i]
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: name, MountPath: mountPath})
	return &container.VolumeMounts[len(container.VolumeMounts)-1]
}

func persistentVolume(class *storageClass, claim *corev1.PersistentVolumeClaim, node *corev1.Node, shared bool, name, path string) (*corev1.PersistentVolume, error) {
	volumeType, ok := class.Annotations[annDefaultVolumeType]
	if !ok {
		volumeType = defaultVolumeType
	}
	if value, ok := claim.Annotations[annVolumeType]; ok {
		volumeType = value
	}
	var source corev1.PersistentVolumeSource
	switch strings.ToLower(volumeType) {
	case "local":
		source.Local = &corev1.LocalVolumeSource{Path: path}
	case "hostpath":
		hostPathType := corev1.HostPathDirectoryOrCreate
		source.HostPath = &corev1.HostPathVolumeSource{Path: path, Type: &hostPathType}
	default:
		return nil, fmt.Errorf("failed to create persistent volume source: %q is not a recognised volume type", volumeType)
	}
	nodeName := ""
	var affinity *corev1.VolumeNodeAffinity
	if node != nil {
		nodeName = node.Name
	}
	if !shared {
		key := keyNode
		if k := class.Parameters["nodeAffinityKey"]; k != "" {
			key = k
		}
		value, ok := node.Labels[key]
		if !ok {
			value = node.Name
		}
		affinity = &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{Key: key, Operator: corev1.NodeSelectorOpIn, Values: []string{value}}},
		}}}}
	}
	reclaim := corev1.PersistentVolumeReclaimDelete
	if class.ReclaimPolicy != nil {
		reclaim = *class.ReclaimPolicy
	}
	fs := corev1.PersistentVolumeFilesystem
	ref := claimRef(claim)
	return &corev1.PersistentVolume{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolume"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{annNodeName: nodeName, annDynamicallyProvisioned: Provisioner}},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: reclaim,
			AccessModes:                   claim.Spec.AccessModes,
			VolumeMode:                    &fs,
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: claim.Spec.Resources.Requests[corev1.ResourceStorage]},
			PersistentVolumeSource:        source,
			NodeAffinity:                  affinity,
			ClaimRef:                      &ref,
			StorageClassName:              class.Name,
		},
	}, nil
}

func pathAndNode(pv *corev1.PersistentVolume, classCfg *classConfig) (string, string, error) {
	var path string
	switch {
	case pv.Spec.HostPath != nil && pv.Spec.Local == nil:
		path = pv.Spec.HostPath.Path
	case pv.Spec.Local != nil && pv.Spec.HostPath == nil:
		path = pv.Spec.Local.Path
	default:
		return "", "", fmt.Errorf("no path set")
	}
	shared, err := classCfg.shared()
	if err != nil {
		return "", "", err
	}
	if shared {
		return path, "", nil
	}
	if pv.Spec.NodeAffinity == nil {
		return "", "", fmt.Errorf("no NodeAffinity set")
	}
	if pv.Spec.NodeAffinity.Required == nil {
		return "", "", fmt.Errorf("no NodeAffinity.Required set")
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Operator != corev1.NodeSelectorOpIn {
				continue
			}
			if len(expr.Values) != 1 {
				return "", "", fmt.Errorf("multiple values for the node affinity")
			}
			return path, expr.Values[0], nil
		}
	}
	return "", "", fmt.Errorf("cannot find affinited node")
}

func list[T any](ctx context.Context, r dynamic.ResourceInterface) ([]T, error) {
	l, err := r.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]T, len(l.Items))
	for i := range l.Items {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(l.Items[i].Object, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func get[T any](ctx context.Context, r dynamic.ResourceInterface, name string) (*T, error) {
	u, err := r.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	out := new(T)
	return out, runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, out)
}

func create(ctx context.Context, r dynamic.ResourceInterface, obj runtime.Object) error {
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}
	_, err = r.Create(ctx, &unstructured.Unstructured{Object: m}, metav1.CreateOptions{})
	return err
}
