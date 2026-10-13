package localpath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strconv"
	"strings"
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
	annFailures               = "k8flare.io/local-path-failures"

	annVolumeType        = "volumeType"
	annDefaultVolumeType = "defaultVolumeType"
	keyNode              = "kubernetes.io/hostname"

	configMapName          = "local-path-config"
	defaultPathKey         = "DEFAULT_PATH_FOR_NON_LISTED_NODES"
	helperPodNameMaxLength = 128
	helperScriptDir        = "/script"
	helperDataVolName      = "data"
	helperScriptVolName    = "script"
	actionCreate           = "create"
	actionDelete           = "delete"

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
)

type Controller struct {
	Client   dynamic.Interface
	Disabled bool
	Now      func() time.Time
}

type storageClass struct {
	metav1.ObjectMeta `json:"metadata"`
	Provisioner       string                                `json:"provisioner"`
	ReclaimPolicy     *corev1.PersistentVolumeReclaimPolicy `json:"reclaimPolicy"`
}

type config struct {
	nodePaths map[string][]string
	helper    *corev1.Pod
}

type volume struct {
	name string
	path string
	mode corev1.PersistentVolumeMode
	size int64
	node string
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
		if claim.Spec.StorageClassName == nil || ours[*claim.Spec.StorageClassName] == nil || !needsVolume(claim, provisioned) {
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

func needsVolume(claim *corev1.PersistentVolumeClaim, provisioned map[string]bool) bool {
	provisioner, ok := claim.Annotations[annStorageProvisioner]
	if !ok {
		provisioner = claim.Annotations[annBetaStorageProvisioner]
	}
	return claim.Spec.VolumeName == "" && provisioner == Provisioner && claim.Annotations[annSelectedNode] != "" && !provisioned[volumeName(claim)]
}

func released(pv *corev1.PersistentVolume) bool {
	return pv.Annotations[annDynamicallyProvisioned] == Provisioner &&
		pv.Status.Phase == corev1.VolumeReleased &&
		pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete
}

func volumeName(claim *corev1.PersistentVolumeClaim) string {
	return "pvc-" + string(claim.UID)
}

func (c *Controller) provision(ctx context.Context, cfg *config, class *storageClass, claim *corev1.PersistentVolumeClaim, helper *corev1.Pod) (time.Time, error) {
	nodeName := claim.Annotations[annSelectedNode]
	node, err := get[corev1.Node](ctx, c.Client.Resource(nodesGVR), nodeName)
	if err != nil {
		return time.Time{}, err
	}
	name := volumeName(claim)
	base, err := cfg.pathOn(nodeName, name)
	if err != nil {
		return time.Time{}, err
	}
	path := filepath.Join(base, strings.Join([]string{name, claim.Namespace, claim.Name}, "_"))
	pv, err := persistentVolume(class, claim, node, name, path)
	if err != nil {
		return time.Time{}, err
	}
	size := claim.Spec.Resources.Requests[corev1.ResourceStorage]
	want, err := cfg.helperPod(actionCreate, "setup", volume{name: name, path: path, mode: modeOf(claim.Spec.VolumeMode), size: size.Value(), node: nodeName})
	if err != nil {
		return time.Time{}, err
	}
	return c.run(ctx, want, helper, func() error {
		err := create(ctx, c.Client.Resource(volumesGVR), pv)
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	})
}

func (c *Controller) teardown(ctx context.Context, cfg *config, pv *corev1.PersistentVolume, helper *corev1.Pod) (time.Time, error) {
	path, node, err := pathAndNode(pv)
	if err != nil {
		return time.Time{}, err
	}
	size := pv.Spec.Capacity[corev1.ResourceStorage]
	want, err := cfg.helperPod(actionDelete, "teardown", volume{name: pv.Name, path: path, mode: modeOf(pv.Spec.VolumeMode), size: size.Value(), node: node})
	if err != nil {
		return time.Time{}, err
	}
	return c.run(ctx, want, helper, func() error {
		err := c.Client.Resource(volumesGVR).Delete(ctx, pv.Name, metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	})
}

func (c *Controller) run(ctx context.Context, want, have *corev1.Pod, done func() error) (time.Time, error) {
	if have == nil {
		return c.createHelper(ctx, want, 0)
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
		return c.createHelper(ctx, want, failures+1)
	}
	if deadline := have.CreationTimestamp.Add(helperTimeout); now.Before(deadline) {
		return deadline, nil
	}
	if err := c.deletePod(ctx, have.Name); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, fmt.Errorf("helper pod %s did not succeed within %v", have.Name, helperTimeout)
}

func (c *Controller) createHelper(ctx context.Context, pod *corev1.Pod, failures int) (time.Time, error) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[annFailures] = strconv.Itoa(failures)
	if err := create(ctx, c.pods(), pod); err != nil {
		return time.Time{}, err
	}
	return c.now().Add(helperTimeout), nil
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
	var file struct {
		NodePathMap []struct {
			Node  string   `json:"node"`
			Paths []string `json:"paths"`
		} `json:"nodePathMap"`
	}
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
	cfg := &config{nodePaths: map[string][]string{}, helper: helper}
	for _, entry := range file.NodePathMap {
		cfg.nodePaths[entry.Node] = entry.Paths
	}
	return cfg, nil
}

func (cfg *config) pathOn(node, volume string) (string, error) {
	paths, ok := cfg.nodePaths[node]
	if !ok {
		paths, ok = cfg.nodePaths[defaultPathKey]
	}
	if !ok {
		return "", fmt.Errorf("%s has no path for node %s and no %s", configMapName, node, defaultPathKey)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no local path available on node %s", node)
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

func (cfg *config) helperPod(action, script string, v volume) (*corev1.Pod, error) {
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
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{Operator: corev1.TolerationOpExists})
	hostPathType := corev1.HostPathDirectoryOrCreate
	pod.Spec.Volumes = append(pod.Spec.Volumes,
		corev1.Volume{Name: helperDataVolName, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: parentDir, Type: &hostPathType}}},
		corev1.Volume{Name: helperScriptVolName, VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
			Items:                []corev1.KeyToPath{{Key: "setup", Path: "setup"}, {Key: "teardown", Path: "teardown"}},
		}}},
	)
	container := &pod.Spec.Containers[0]
	addVolumeMount(container, helperScriptVolName, helperScriptDir).MountPath = helperScriptDir
	dataDir := strings.TrimSuffix(addVolumeMount(container, helperDataVolName, parentDir).MountPath, string(filepath.Separator))
	volumeDir = strings.TrimSuffix(volumeDir, string(filepath.Separator))
	if dataDir == "" || volumeDir == "" || !filepath.IsAbs(dataDir) {
		return nil, fmt.Errorf("invalid path %s for %s: cannot find parent dir or volume dir or parent dir is relative", v.path, action)
	}
	volDir := filepath.Join(dataDir, volumeDir)
	size := strconv.FormatInt(v.size, 10)
	container.Command = []string{"/bin/sh", helperScriptDir + "/" + script}
	container.Env = append(container.Env,
		corev1.EnvVar{Name: "VOL_DIR", Value: volDir},
		corev1.EnvVar{Name: "VOL_MODE", Value: string(v.mode)},
		corev1.EnvVar{Name: "VOL_SIZE_BYTES", Value: size},
	)
	container.Args = []string{"-p", volDir, "-s", size, "-m", string(v.mode), "-a", action}
	privileged := true
	container.SecurityContext = &corev1.SecurityContext{Privileged: &privileged}
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

func persistentVolume(class *storageClass, claim *corev1.PersistentVolumeClaim, node *corev1.Node, name, path string) (*corev1.PersistentVolume, error) {
	volumeType := claim.Annotations[annVolumeType]
	if volumeType == "" {
		volumeType = class.Annotations[annDefaultVolumeType]
	}
	var source corev1.PersistentVolumeSource
	switch strings.ToLower(volumeType) {
	case "local":
		source.Local = &corev1.LocalVolumeSource{Path: path}
	case "", "hostpath":
		hostPathType := corev1.HostPathDirectoryOrCreate
		source.HostPath = &corev1.HostPathVolumeSource{Path: path, Type: &hostPathType}
	default:
		return nil, fmt.Errorf("unknown volume type %q", volumeType)
	}
	hostname, ok := node.Labels[keyNode]
	if !ok {
		hostname = node.Name
	}
	reclaim := corev1.PersistentVolumeReclaimDelete
	if class.ReclaimPolicy != nil {
		reclaim = *class.ReclaimPolicy
	}
	fs := corev1.PersistentVolumeFilesystem
	return &corev1.PersistentVolume{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolume"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{annDynamicallyProvisioned: Provisioner}},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: reclaim,
			AccessModes:                   claim.Spec.AccessModes,
			VolumeMode:                    &fs,
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: claim.Spec.Resources.Requests[corev1.ResourceStorage]},
			PersistentVolumeSource:        source,
			NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{Key: keyNode, Operator: corev1.NodeSelectorOpIn, Values: []string{hostname}}},
			}}}},
			ClaimRef: &corev1.ObjectReference{
				Kind:            "PersistentVolumeClaim",
				APIVersion:      "v1",
				Namespace:       claim.Namespace,
				Name:            claim.Name,
				UID:             claim.UID,
				ResourceVersion: claim.ResourceVersion,
			},
			StorageClassName: class.Name,
		},
	}, nil
}

func pathAndNode(pv *corev1.PersistentVolume) (string, string, error) {
	var path string
	switch {
	case pv.Spec.Local != nil:
		path = pv.Spec.Local.Path
	case pv.Spec.HostPath != nil:
		path = pv.Spec.HostPath.Path
	default:
		return "", "", fmt.Errorf("no local or hostPath source")
	}
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return "", "", fmt.Errorf("no required node affinity")
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Key == keyNode && expr.Operator == corev1.NodeSelectorOpIn && len(expr.Values) == 1 {
				return path, expr.Values[0], nil
			}
		}
	}
	return "", "", fmt.Errorf("no %s node affinity with one value", keyNode)
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
