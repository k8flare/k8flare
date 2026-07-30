package apiserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
)

// fakeDO is an in-memory stand-in for the Cluster Durable Object's key
// endpoint, speaking just enough of the kine wire protocol for Storage.
// It counts writes, which is the whole point: the assertion below is about
// whether a write happens at all, not about what it contains.
type fakeDO struct {
	value    []byte
	revision int64
	puts     int
}

func (f *fakeDO) fetch(req *http.Request) (*http.Response, error) {
	reply := func(status int, body any) (*http.Response, error) {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewReader(b)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}

	switch req.Method {
	case http.MethodGet:
		if f.value == nil {
			return reply(http.StatusOK, map[string]any{"kv": nil})
		}
		return reply(http.StatusOK, map[string]any{"kv": map[string]any{
			"key":            strings.TrimPrefix(req.URL.Path, "/key"),
			"createRevision": int64(1),
			"modRevision":    f.revision,
			"value":          base64.StdEncoding.EncodeToString(f.value),
		}})

	case http.MethodPut:
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var put struct {
			Value    string `json:"value"`
			Revision int64  `json:"revision"`
		}
		if err := json.Unmarshal(body, &put); err != nil {
			return nil, err
		}
		data, err := base64.StdEncoding.DecodeString(put.Value)
		if err != nil {
			return nil, err
		}
		if put.Revision != 0 && put.Revision != f.revision {
			return reply(http.StatusConflict, map[string]any{"revision": f.revision})
		}
		f.puts++
		f.value = data
		f.revision++
		status := http.StatusOK
		if put.Revision == 0 {
			status = http.StatusCreated
		}
		return reply(status, map[string]any{"revision": f.revision, "updated": true})
	}
	return reply(http.StatusMethodNotAllowed, map[string]any{"error": req.Method})
}

// TestGuaranteedUpdateSuppressesNoOpWrites pins the no-op suppression in
// KineStorage.GuaranteedUpdate, which upstream's etcd3 store performs before
// its Txn and this one lacked until 2026-07-30.
//
// Why it matters, and why the bar is "zero writes" rather than "not too
// many": a write is not just a wasted row. It pokes the controllers via the
// Cluster DO's afterWrite, which makes them re-sync and write again. Measured
// on a node-less cluster, a single Deployment's status rewrite ran at ~20
// writes/sec indefinitely -- ~1,130 kine rows a minute, unbounded, for an
// object nobody had touched since creating it (docs/platform-verification.md
// "S26 訂正", docs/cost-model.md).
//
// This exercises KineStorage directly rather than going through the API.
// That is deliberate: genericregistry.Store has its own equality check on the
// decoded object, and it already absorbs a plain client-side no-op update --
// verified 2026-07-30 by disabling the guard and replaying no-op PUTs against
// a live dev server, where resourceVersion stayed put either way. An
// end-to-end test therefore cannot tell the fixed build from the broken one.
// The writes that got through in production differed at the registry's layer
// and became identical only after PrepareObjectForStorage + EncodeToStorage,
// which is exactly the representation this guard compares.
func TestGuaranteedUpdateSuppressesNoOpWrites(t *testing.T) {
	ctx := context.Background()
	newFunc := func() runtime.Object { return &corev1.ConfigMap{} }
	do := &fakeDO{}
	ks := NewKineStorage(NewStorage(do.fetch, "/registry"), newFunc)

	const key = "/configmaps/default/probe"
	in := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: "default"},
		Data:       map[string]string{"k": "v"},
	}
	out := &corev1.ConfigMap{}
	if err := ks.Create(ctx, key, in, out, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if do.puts != 1 {
		t.Fatalf("Create should write exactly once: puts = %d", do.puts)
	}

	unchanged := func(cur runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
		return cur, nil, nil
	}

	// Two consecutive no-op updates, not one: a single suppressed write
	// proves the guard fires, but not that the skip path leaves the store
	// usable for the next caller. Both iterations here must be suppressed.
	for i := 1; i <= 2; i++ {
		got := &corev1.ConfigMap{}
		if err := ks.GuaranteedUpdate(ctx, key, got, false, nil, unchanged, nil); err != nil {
			t.Fatalf("no-op GuaranteedUpdate #%d: %v", i, err)
		}
		if got.ResourceVersion != out.ResourceVersion {
			t.Errorf("no-op GuaranteedUpdate #%d returned resourceVersion %q, want the unchanged %q",
				i, got.ResourceVersion, out.ResourceVersion)
		}
		if got.Data["k"] != "v" {
			t.Errorf("no-op GuaranteedUpdate #%d lost data: %v", i, got.Data)
		}
	}
	if do.puts != 1 {
		t.Errorf("no-op updates wrote %d revision(s), want 0 beyond the create. "+
			"Each one pokes the controllers, which re-sync and write again -- "+
			"this is the S26 write storm", do.puts-1)
	}

	// The guard must not swallow a real change.
	changed := func(cur runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
		cm := cur.(*corev1.ConfigMap).DeepCopy()
		cm.Data["k"] = "v2"
		return cm, nil, nil
	}
	got := &corev1.ConfigMap{}
	if err := ks.GuaranteedUpdate(ctx, key, got, false, nil, changed, nil); err != nil {
		t.Fatalf("real GuaranteedUpdate: %v", err)
	}
	if do.puts != 2 {
		t.Errorf("a real change must write: puts = %d, want 2", do.puts)
	}
	if got.Data["k"] != "v2" {
		t.Errorf("real change not persisted: Data = %v", got.Data)
	}
	if got.ResourceVersion == out.ResourceVersion {
		t.Errorf("a real change must bump resourceVersion: stayed %q", got.ResourceVersion)
	}
}

// TestGuaranteedUpdateCreatesWhenAbsent pins the `currentRev != 0` half of the
// suppression guard. It is not a redundant "is the key there" check: on the
// absent-key path `stored` is nil (Storage.Get returns nil, ErrNotFound), so
// evaluating bytes.Equal(data, stored.Value) without it is a nil dereference
// that panics the apiserver. The path is reachable in production -- upstream's
// genericregistry.Store passes ignoreNotFound=true for any resource whose
// strategy allows create-on-update.
func TestGuaranteedUpdateCreatesWhenAbsent(t *testing.T) {
	do := &fakeDO{}
	ks := NewKineStorage(NewStorage(do.fetch, "/registry"),
		func() runtime.Object { return &corev1.ConfigMap{} })

	create := func(_ runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: "default"},
			Data:       map[string]string{"k": "v"},
		}, nil, nil
	}
	got := &corev1.ConfigMap{}
	if err := ks.GuaranteedUpdate(context.Background(), "/configmaps/default/probe",
		got, true, nil, create, nil); err != nil {
		t.Fatalf("GuaranteedUpdate on an absent key: %v", err)
	}
	if do.puts != 1 {
		t.Errorf("create-on-update must write exactly once: puts = %d", do.puts)
	}
	if got.Data["k"] != "v" {
		t.Errorf("created object not returned: Data = %v", got.Data)
	}
}

// TestGuaranteedUpdateSuppressesStatusRewrite is the shape the production
// incident actually took: a controller recomputing a status containing
// conditions and metav1.Time stamps, and storing it unchanged. ConfigMap Data
// is a flat string map and cannot exercise the one class of bug that would
// silently defeat the guard -- an encode round-trip that is not byte-stable,
// which timestamps and nested condition lists are the likely source of.
func TestGuaranteedUpdateSuppressesStatusRewrite(t *testing.T) {
	ctx := context.Background()
	do := &fakeDO{}
	ks := NewKineStorage(NewStorage(do.fetch, "/registry"),
		func() runtime.Object { return &corev1.Pod{} })

	const key = "/pods/default/probe"
	stamp := metav1.Date(2026, 7, 30, 9, 45, 51, 0, time.UTC)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: "default", CreationTimestamp: stamp},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
		Status: corev1.PodStatus{
			Phase:     corev1.PodPending,
			StartTime: &stamp,
			Conditions: []corev1.PodCondition{{
				Type:               corev1.PodScheduled,
				Status:             corev1.ConditionFalse,
				Reason:             "Unschedulable",
				Message:            "no nodes available",
				LastProbeTime:      stamp,
				LastTransitionTime: stamp,
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "c",
				Image: "nginx",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "Pending"}},
			}},
		},
	}
	out := &corev1.Pod{}
	if err := ks.Create(ctx, key, pod, out, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Recompute the identical status from the stored object, as a controller
	// sync does, and write it back. Twice.
	rewriteStatus := func(cur runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
		p := cur.(*corev1.Pod).DeepCopy()
		p.Status = *p.Status.DeepCopy()
		return p, nil, nil
	}
	for i := 1; i <= 2; i++ {
		got := &corev1.Pod{}
		if err := ks.GuaranteedUpdate(ctx, key, got, false, nil, rewriteStatus, nil); err != nil {
			t.Fatalf("status rewrite #%d: %v", i, err)
		}
		if got.ResourceVersion != out.ResourceVersion {
			t.Errorf("status rewrite #%d bumped resourceVersion: %q -> %q",
				i, out.ResourceVersion, got.ResourceVersion)
		}
	}
	if do.puts != 1 {
		t.Errorf("identical status rewrites wrote %d revision(s), want 0 beyond the create. "+
			"An encode round-trip that is not byte-stable would look exactly like this",
			do.puts-1)
	}

	// A real status transition must still land.
	transition := func(cur runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
		p := cur.(*corev1.Pod).DeepCopy()
		p.Status.Phase = corev1.PodRunning
		return p, nil, nil
	}
	got := &corev1.Pod{}
	if err := ks.GuaranteedUpdate(ctx, key, got, false, nil, transition, nil); err != nil {
		t.Fatalf("status transition: %v", err)
	}
	if do.puts != 2 {
		t.Errorf("a real status transition must write: puts = %d, want 2", do.puts)
	}
	if got.Status.Phase != corev1.PodRunning {
		t.Errorf("transition not persisted: phase = %q", got.Status.Phase)
	}
}
