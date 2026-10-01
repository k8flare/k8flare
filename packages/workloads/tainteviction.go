package workloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	apipod "k8s.io/kubernetes/pkg/api/v1/pod"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	utilpod "k8s.io/kubernetes/pkg/util/pod"
)

var Store *kine.Client

const taintEvictionPrefix = "/k8flare/tainteviction/"

type evictionRecord struct {
	UID       types.UID   `json:"uid"`
	StartedAt metav1.Time `json:"startedAt"`
}

type storedEviction struct {
	revision int64
	record   evictionRecord
}

type evictionRecords interface {
	list(ctx context.Context) (map[string]storedEviction, error)
	put(ctx context.Context, key string, rec evictionRecord, revision int64) error
	delete(ctx context.Context, key string, revision int64) error
}

type kineEvictionRecords struct{ client *kine.Client }

func (k kineEvictionRecords) list(ctx context.Context) (map[string]storedEviction, error) {
	kvs, _, _, err := k.client.List(ctx, taintEvictionPrefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := make(map[string]storedEviction, len(kvs))
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		var rec evictionRecord
		if json.Unmarshal(data, &rec) != nil {
			continue
		}
		out[kv.Key] = storedEviction{revision: kv.ModRevision, record: rec}
	}
	return out, nil
}

func (k kineEvictionRecords) put(ctx context.Context, key string, rec evictionRecord, revision int64) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = k.client.Put(ctx, key, data, revision)
	return err
}

func (k kineEvictionRecords) delete(ctx context.Context, key string, revision int64) error {
	_, err := k.client.Delete(ctx, key, revision)
	return err
}

type memoryEvictionRecords struct {
	mu   sync.Mutex
	recs map[string]storedEviction
}

var localEvictionRecords = &memoryEvictionRecords{recs: map[string]storedEviction{}}

func (m *memoryEvictionRecords) list(context.Context) (map[string]storedEviction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]storedEviction, len(m.recs))
	for k, v := range m.recs {
		out[k] = v
	}
	return out, nil
}

func (m *memoryEvictionRecords) put(_ context.Context, key string, rec evictionRecord, revision int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs[key] = storedEviction{revision: revision + 1, record: rec}
	return nil
}

func (m *memoryEvictionRecords) delete(_ context.Context, key string, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.recs, key)
	return nil
}

func taintEvictionRecords() evictionRecords {
	if Store == nil {
		return localEvictionRecords
	}
	return kineEvictionRecords{client: Store}
}

func evictTaintedPods(ctx context.Context, client kubernetes.Interface, nodes []*v1.Node, pods []*v1.Pod, now time.Time) error {
	tainted := map[string][]v1.Taint{}
	for _, node := range nodes {
		if taints := noExecuteTaints(node.Spec.Taints); len(taints) > 0 {
			tainted[node.Name] = taints
		}
	}
	records := taintEvictionRecords()
	stored, err := records.list(ctx)
	if err != nil {
		return err
	}
	logger := klog.FromContext(ctx)
	broadcaster := record.NewBroadcaster(record.WithContext(ctx))
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: client.CoreV1().Events("")})
	defer broadcaster.Shutdown()
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "taint-eviction-controller"})
	kept := map[string]bool{}
	var errs []error
	for _, pod := range pods {
		taints, ok := tainted[pod.Spec.NodeName]
		if !ok || pod.DeletionTimestamp != nil {
			continue
		}
		key := taintEvictionPrefix + pod.Namespace + "/" + pod.Name
		fireAt := now
		if allTolerated, used := v1helper.GetMatchingTolerations(logger, taints, pod.Spec.Tolerations); allTolerated {
			minToleration := minTolerationTime(used)
			if minToleration < 0 {
				continue
			}
			start := now
			if rec, ok := stored[key]; ok && rec.record.UID == pod.UID {
				start = rec.record.StartedAt.Time
			} else {
				if err := records.put(ctx, key, evictionRecord{UID: pod.UID, StartedAt: metav1.NewTime(now)}, rec.revision); err != nil && err != kine.ErrConflict {
					errs = append(errs, err)
				}
			}
			fireAt = start.Add(minToleration)
		}
		if fireAt.After(now) {
			kept[key] = true
			pending.add(fireAt.Sub(now))
			continue
		}
		recorder.Eventf(&v1.ObjectReference{APIVersion: "v1", Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
			v1.EventTypeNormal, "TaintManagerEviction", "Marking for deletion Pod %s/%s", pod.Namespace, pod.Name)
		if err := addConditionAndDeletePod(ctx, client, pod); err != nil {
			errs = append(errs, err)
			kept[key] = true
			pending.add(evictionRetry)
		}
	}
	for key, rec := range stored {
		if kept[key] {
			continue
		}
		if err := records.delete(ctx, key, rec.revision); err != nil && err != kine.ErrNotFound && err != kine.ErrConflict {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func noExecuteTaints(taints []v1.Taint) []v1.Taint {
	var out []v1.Taint
	for _, taint := range taints {
		if taint.Effect == v1.TaintEffectNoExecute {
			out = append(out, taint)
		}
	}
	return out
}

func minTolerationTime(tolerations []v1.Toleration) time.Duration {
	minSeconds := int64(math.MaxInt64)
	if len(tolerations) == 0 {
		return 0
	}
	for i := range tolerations {
		if tolerations[i].TolerationSeconds == nil {
			continue
		}
		seconds := *tolerations[i].TolerationSeconds
		if seconds <= 0 {
			return 0
		}
		if seconds < minSeconds {
			minSeconds = seconds
		}
	}
	if minSeconds == int64(math.MaxInt64) {
		return -1
	}
	return time.Duration(minSeconds) * time.Second
}

func addConditionAndDeletePod(ctx context.Context, client kubernetes.Interface, pod *v1.Pod) error {
	newStatus := pod.Status.DeepCopy()
	updated := apipod.UpdatePodCondition(newStatus, &v1.PodCondition{
		Type:               v1.DisruptionTarget,
		ObservedGeneration: apipod.CalculatePodConditionObservedGeneration(&pod.Status, pod.Generation, v1.DisruptionTarget),
		Status:             v1.ConditionTrue,
		Reason:             "DeletionByTaintManager",
		Message:            "Taint manager: deleting due to NoExecute taint",
	})
	if updated {
		if _, _, _, err := utilpod.PatchPodStatus(ctx, client, pod.Namespace, pod.Name, pod.UID, pod.Status, *newStatus); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	err := client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func podsOf(all []loadedSource) []*v1.Pod {
	for _, l := range all {
		if _, ok := l.informer.example.(*v1.Pod); !ok {
			continue
		}
		items := l.informer.GetIndexer().List()
		pods := make([]*v1.Pod, 0, len(items))
		for _, item := range items {
			if pod, ok := item.(*v1.Pod); ok {
				pods = append(pods, pod)
			}
		}
		return pods
	}
	return nil
}
