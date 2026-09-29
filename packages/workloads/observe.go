package workloads

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
)

func observeWrites(client kubernetes.Interface, all []loadedSource) kubernetes.Interface {
	observed := &observeClient{Interface: client}
	for _, l := range all {
		switch l.informer.example.(type) {
		case *corev1.PersistentVolume:
			observed.volumes = l.informer.GetIndexer()
		case *corev1.PersistentVolumeClaim:
			observed.claims = l.informer.GetIndexer()
		case *corev1.Pod:
			l.informer.list = func(ctx context.Context) ([]runtime.Object, error) {
				return listPods(ctx, client.CoreV1().Pods(""))
			}
			observed.pods = l.informer
		case *corev1.ReplicationController:
			observed.rcs = l.informer
		}
	}
	if observed.volumes == nil && observed.claims == nil && observed.pods == nil && observed.rcs == nil {
		return client
	}
	return observed
}

func listPods(ctx context.Context, pods corev1client.PodInterface) ([]runtime.Object, error) {
	var out []runtime.Object
	opts := metav1.ListOptions{Limit: listPage}
	for {
		listed, err := pods.List(ctx, opts)
		if err != nil {
			return nil, err
		}
		for i := range listed.Items {
			out = append(out, listed.Items[i].DeepCopy())
		}
		if listed.Continue == "" {
			return out, nil
		}
		opts.Continue = listed.Continue
	}
}

func remember(indexer cache.Indexer, obj runtime.Object) {
	if indexer == nil || obj == nil {
		return
	}
	if err := indexer.Update(obj); err != nil {
		_ = indexer.Add(obj)
	}
}

type observeClient struct {
	kubernetes.Interface
	volumes cache.Indexer
	claims  cache.Indexer
	pods    *snapshotInformer
	rcs     *snapshotInformer
}

func (c *observeClient) CoreV1() corev1client.CoreV1Interface {
	return observeCore{CoreV1Interface: c.Interface.CoreV1(), volumes: c.volumes, claims: c.claims, pods: c.pods, rcs: c.rcs}
}

type observeCore struct {
	corev1client.CoreV1Interface
	volumes cache.Indexer
	claims  cache.Indexer
	pods    *snapshotInformer
	rcs     *snapshotInformer
}

func (c observeCore) PersistentVolumes() corev1client.PersistentVolumeInterface {
	return observeVolumes{PersistentVolumeInterface: c.CoreV1Interface.PersistentVolumes(), index: c.volumes}
}

func (c observeCore) PersistentVolumeClaims(namespace string) corev1client.PersistentVolumeClaimInterface {
	return observeClaims{PersistentVolumeClaimInterface: c.CoreV1Interface.PersistentVolumeClaims(namespace), index: c.claims}
}

func (c observeCore) Pods(namespace string) corev1client.PodInterface {
	return observePods{
		PodInterface: c.CoreV1Interface.Pods(namespace),
		namespace:    namespace,
		pods:         c.pods,
		rcs:          c.rcs,
		rcStatus:     c.CoreV1Interface.ReplicationControllers(namespace),
	}
}

func (c observeCore) ReplicationControllers(namespace string) corev1client.ReplicationControllerInterface {
	return observeReplicationControllers{
		ReplicationControllerInterface: c.CoreV1Interface.ReplicationControllers(namespace),
		rcs:                            c.rcs,
		pods:                           c.pods,
		listPods: func(ctx context.Context, ns string) ([]corev1.Pod, error) {
			listed, err := listPods(ctx, c.CoreV1Interface.Pods(ns))
			if err != nil {
				return nil, err
			}
			pods := make([]corev1.Pod, 0, len(listed))
			for _, item := range listed {
				pod, ok := item.(*corev1.Pod)
				if ok {
					pods = append(pods, *pod)
				}
			}
			return pods, nil
		},
	}
}

type observePods struct {
	corev1client.PodInterface
	namespace string
	pods      *snapshotInformer
	rcs       *snapshotInformer
	rcStatus  corev1client.ReplicationControllerInterface
}

func (p observePods) Create(ctx context.Context, pod *corev1.Pod, opts metav1.CreateOptions) (*corev1.Pod, error) {
	got, err := p.PodInterface.Create(ctx, pod, opts)
	if err != nil {
		p.pods.markStale()
		return got, err
	}
	p.pods.note(got)
	p.publishReached(ctx, got)
	return got, nil
}

func (p observePods) publishReached(ctx context.Context, pod *corev1.Pod) {
	if p.rcStatus == nil || p.pods == nil || pod == nil {
		return
	}
	ref := metav1.GetControllerOf(pod)
	if ref == nil || ref.Kind != "ReplicationController" || ref.UID == "" {
		return
	}
	n, ok := activeOwnedPods(p.pods, pod.Namespace, ref.UID)
	if !ok {
		return
	}
	rc, err := p.rcStatus.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil || rc.Spec.Replicas == nil || n < *rc.Spec.Replicas || rc.Status.Replicas >= n {
		return
	}
	rc = rc.DeepCopy()
	if rc.Status.FullyLabeledReplicas == rc.Status.Replicas {
		rc.Status.FullyLabeledReplicas = n
	}
	rc.Status.Replicas = n
	updated, err := p.rcStatus.UpdateStatus(ctx, rc, metav1.UpdateOptions{})
	if err == nil {
		p.rcs.note(updated)
	}
}

func (p observePods) Update(ctx context.Context, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	got, err := p.PodInterface.Update(ctx, pod, opts)
	if err == nil {
		p.pods.note(got)
	}
	return got, err
}

func (p observePods) UpdateStatus(ctx context.Context, pod *corev1.Pod, opts metav1.UpdateOptions) (*corev1.Pod, error) {
	got, err := p.PodInterface.UpdateStatus(ctx, pod, opts)
	if err == nil {
		p.pods.note(got)
	}
	return got, err
}

func (p observePods) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	err := p.PodInterface.Delete(ctx, name, opts)
	if err == nil {
		p.pods.forget(p.namespace, name)
	}
	return err
}

type observeReplicationControllers struct {
	corev1client.ReplicationControllerInterface
	rcs      *snapshotInformer
	pods     *snapshotInformer
	listPods func(context.Context, string) ([]corev1.Pod, error)
}

func (r observeReplicationControllers) Update(ctx context.Context, rc *corev1.ReplicationController, opts metav1.UpdateOptions) (*corev1.ReplicationController, error) {
	got, err := r.ReplicationControllerInterface.Update(ctx, rc, opts)
	if err == nil {
		r.rcs.note(got)
	}
	return got, err
}

func (r observeReplicationControllers) UpdateStatus(ctx context.Context, rc *corev1.ReplicationController, opts metav1.UpdateOptions) (*corev1.ReplicationController, error) {
	rc = r.withObservedReplicas(ctx, rc)
	got, err := r.ReplicationControllerInterface.UpdateStatus(ctx, rc, opts)
	if err == nil {
		r.rcs.note(got)
	}
	return got, err
}

func (r observeReplicationControllers) withObservedReplicas(ctx context.Context, rc *corev1.ReplicationController) *corev1.ReplicationController {
	n, ok := activeOwnedPods(r.pods, rc.Namespace, rc.UID)
	if !ok || rc.Spec.Replicas == nil {
		return rc
	}
	if n < *rc.Spec.Replicas && r.listPods != nil {
		if items, err := r.listPods(ctx, rc.Namespace); err == nil {
			for i := range items {
				r.pods.note(&items[i])
			}
			if counted, ok := activeOwnedPods(r.pods, rc.Namespace, rc.UID); ok {
				n = counted
			}
		}
	}
	if rc.Status.Replicas >= n {
		return rc
	}
	out := rc.DeepCopy()
	if out.Status.FullyLabeledReplicas == out.Status.Replicas {
		out.Status.FullyLabeledReplicas = n
	}
	out.Status.Replicas = n
	live, err := r.ReplicationControllerInterface.Get(ctx, rc.Name, metav1.GetOptions{})
	if err != nil {
		return out
	}
	live = live.DeepCopy()
	live.Status = out.Status
	return live
}

func activeOwnedPods(pods *snapshotInformer, namespace string, uid types.UID) (int32, bool) {
	if pods == nil || uid == "" {
		return 0, false
	}
	items, err := pods.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		return 0, false
	}
	var n int32
	for _, item := range items {
		pod, ok := item.(*corev1.Pod)
		if !ok || !podActive(pod) {
			continue
		}
		ref := metav1.GetControllerOf(pod)
		if ref == nil || ref.UID != uid || ref.Kind != "ReplicationController" {
			continue
		}
		n++
	}
	return n, true
}

func podActive(pod *corev1.Pod) bool {
	return pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed && pod.DeletionTimestamp == nil
}

type observeVolumes struct {
	corev1client.PersistentVolumeInterface
	index cache.Indexer
}

func (v observeVolumes) Update(ctx context.Context, volume *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	got, err := v.PersistentVolumeInterface.Update(ctx, volume, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}

func (v observeVolumes) UpdateStatus(ctx context.Context, volume *corev1.PersistentVolume, opts metav1.UpdateOptions) (*corev1.PersistentVolume, error) {
	got, err := v.PersistentVolumeInterface.UpdateStatus(ctx, volume, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}

type observeClaims struct {
	corev1client.PersistentVolumeClaimInterface
	index cache.Indexer
}

func (v observeClaims) Update(ctx context.Context, claim *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	got, err := v.PersistentVolumeClaimInterface.Update(ctx, claim, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}

func (v observeClaims) UpdateStatus(ctx context.Context, claim *corev1.PersistentVolumeClaim, opts metav1.UpdateOptions) (*corev1.PersistentVolumeClaim, error) {
	got, err := v.PersistentVolumeClaimInterface.UpdateStatus(ctx, claim, opts)
	if err == nil {
		remember(v.index, got)
	}
	return got, err
}
