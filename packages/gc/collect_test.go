package gc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func owned(uid string, owners ...metav1.OwnerReference) *item {
	return &item{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: uid, Namespace: "default", UID: types.UID(uid), OwnerReferences: owners},
		key:        "/registry/pods/default/" + uid,
	}
}

func ref(uid string, blocking bool) metav1.OwnerReference {
	return metav1.OwnerReference{Kind: "ReplicationController", APIVersion: "v1", Name: uid, UID: types.UID(uid), BlockOwnerDeletion: ptr.To(blocking)}
}

func graphOf(items ...*item) *graph {
	g := &graph{byUID: map[types.UID]*item{}, dependents: map[types.UID][]*item{}}
	for _, it := range items {
		g.add(it)
	}
	return g
}

func TestOwnerClassification(t *testing.T) {
	live := owned("rc-live")
	gone := owned("pod-dangling", ref("rc-missing", false))
	kept := owned("pod-kept", ref("rc-live", false), ref("rc-missing", false))
	foreground := owned("rc-foreground")
	foreground.DeletionTimestamp = &metav1.Time{}
	foreground.Finalizers = []string{foregroundFinalizer}
	waiting := owned("pod-waiting", ref("rc-foreground", true))
	g := graphOf(live, gone, kept, foreground, waiting)

	if solid, waitingRefs, stale := classify(g, gone); solid != 0 || waitingRefs != 0 || len(stale) != 1 {
		t.Fatalf("dangling: solid=%d waiting=%d stale=%d", solid, waitingRefs, len(stale))
	}
	if solid, waitingRefs, stale := classify(g, kept); solid != 1 || waitingRefs != 0 || len(stale) != 1 {
		t.Fatalf("mixed: solid=%d waiting=%d stale=%d", solid, waitingRefs, len(stale))
	}
	if solid, waitingRefs, _ := classify(g, waiting); solid != 0 || waitingRefs != 1 {
		t.Fatalf("waiting: solid=%d waiting=%d", solid, waitingRefs)
	}
	if !blocks(waiting, "rc-foreground") {
		t.Fatal("expected the dependent to block its owner")
	}
}

func TestCircleBlockersAreAlreadyDeleting(t *testing.T) {
	p1 := owned("p1", ref("p3", true))
	p2 := owned("p2", ref("p1", true))
	p3 := owned("p3", ref("p2", true))
	now := metav1.Now()
	for _, p := range []*item{p1, p2, p3} {
		p.DeletionTimestamp = &now
		p.Finalizers = []string{foregroundFinalizer}
	}
	g := graphOf(p1, p2, p3)
	for _, p := range []*item{p1, p2, p3} {
		live, deleting := splitBlockers(g, p)
		if len(live) != 0 || len(deleting) != 1 {
			t.Fatalf("%s: live=%d deleting=%d", p.Name, len(live), len(deleting))
		}
	}
}

func TestOrphanEventKeys(t *testing.T) {
	ns := &item{TypeMeta: metav1.TypeMeta{Kind: "Namespace"}, ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	nodeLease := &item{TypeMeta: metav1.TypeMeta{Kind: "Namespace"}, ObjectMeta: metav1.ObjectMeta{Name: "kube-node-lease"}}
	g := graphOf(ns, nodeLease)
	g.eventKeys = []string{
		"/registry/events/default/keep",
		"/registry/events/gone/drop",
		"/registry/events.k8s.io/events/gone/other",
	}
	g.leaseKeys = []string{
		"/registry/leases/kube-node-lease/k8flare-agent",
		"/registry/leases/gone/agent",
	}
	got := orphanEventKeys(g)
	if len(got) != 2 || got[0] != "/registry/events/gone/drop" || got[1] != "/registry/events.k8s.io/events/gone/other" {
		t.Fatalf("orphan events: %#v", got)
	}
	leases := orphanLeaseKeys(g)
	if len(leases) != 1 || leases[0] != "/registry/leases/gone/agent" {
		t.Fatalf("orphan leases: %#v", leases)
	}
}

func TestExpiredEventKeys(t *testing.T) {
	now := time.Now()
	g := &graph{
		eventKeys: []string{"/registry/events/default/old", "/registry/events/default/new"},
		eventAt: map[string]time.Time{
			"/registry/events/default/old": now.Add(-2 * time.Hour),
			"/registry/events/default/new": now.Add(-time.Minute),
		},
	}
	got := expiredEventKeys(g, now)
	if len(got) != 1 || got[0] != "/registry/events/default/old" {
		t.Fatalf("expired: %#v", got)
	}
}

func TestAbsentNamespace(t *testing.T) {
	ns := &item{TypeMeta: metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"}, ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "ns"}}
	kept := owned("kept")
	orphan := owned("orphan")
	orphan.Namespace = "gone"
	cluster := owned("node")
	cluster.Namespace = ""
	cluster.Kind = "Node"
	got := absentNamespace([]*item{ns, kept, orphan, cluster})
	if len(got) != 1 || got[0].Name != "orphan" {
		t.Fatalf("absent namespace: %#v", got)
	}
}

func TestPropagationFromFinalizers(t *testing.T) {
	c := &collector{}
	orphaned := owned("rc-orphan")
	orphaned.Finalizers = []string{orphanFinalizer}
	if got := c.propagationFor(orphaned); got != metav1.DeletePropagationOrphan {
		t.Fatalf("orphan finalizer gave %s", got)
	}
	plain := owned("rc-plain")
	if got := c.propagationFor(plain); got != metav1.DeletePropagationBackground {
		t.Fatalf("plain gave %s", got)
	}
}

type fakeKineStore struct {
	mu   sync.Mutex
	data map[string]string
}

func (s *fakeKineStore) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	if req.URL.Path == "/list" {
		prefix := req.URL.Query().Get("prefix")
		from := req.URL.Query().Get("from")
		var kvs []kine.KV
		for k, v := range s.data {
			if strings.HasPrefix(k, prefix) && (from == "" || k > from) {
				kvs = append(kvs, kine.KV{Key: k, Value: v, ModRevision: 1})
			}
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
		out["kvs"] = kvs
	}
	body, _ := json.Marshal(out)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
}

type blockingDynamicClient struct {
	onDelete func(name string)
}

func (c *blockingDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &blockingResourceClient{onDelete: c.onDelete}
}

type blockingResourceClient struct {
	onDelete func(name string)
}

func (b *blockingResourceClient) Namespace(ns string) dynamic.ResourceInterface {
	return b
}

func (b *blockingResourceClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
	b.onDelete(name)
	return nil
}

func (b *blockingResourceClient) Create(ctx context.Context, obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (b *blockingResourceClient) Update(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (b *blockingResourceClient) UpdateStatus(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (b *blockingResourceClient) DeleteCollection(ctx context.Context, options metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}
func (b *blockingResourceClient) Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (b *blockingResourceClient) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, nil
}
func (b *blockingResourceClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}
func (b *blockingResourceClient) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (b *blockingResourceClient) Apply(ctx context.Context, name string, obj *unstructured.Unstructured, options metav1.ApplyOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (b *blockingResourceClient) ApplyStatus(ctx context.Context, name string, obj *unstructured.Unstructured, options metav1.ApplyOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func TestConcurrentGCSyncs(t *testing.T) {
	const total = 10
	kineData := map[string]string{}
	ns := &item{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "ns-default"},
		key:        "/registry/namespaces/default",
	}
	rawNs, _ := json.Marshal(ns)
	kineData[ns.key] = base64.StdEncoding.EncodeToString(rawNs)

	for i := 0; i < total; i++ {
		name := fmt.Sprintf("pod-%d", i)
		it := owned(name, ref("missing-rc", false))
		raw, _ := json.Marshal(it)
		kineData[it.key] = base64.StdEncoding.EncodeToString(raw)
	}

	store := &kine.Client{HTTP: &http.Client{Transport: &fakeKineStore{data: kineData}}}

	kubeClient := fakekube.NewSimpleClientset()
	fakeDisco := kubeClient.Discovery().(*fake.FakeDiscovery)
	fakeDisco.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod"},
				{Name: "namespaces", SingularName: "namespace", Namespaced: false, Kind: "Namespace"},
			},
		},
	}

	var (
		mu           sync.Mutex
		inFlight     int
		maxInFlight  int
		reachedMulti = make(chan struct{})
		once         sync.Once
	)

	dyn := &blockingDynamicClient{
		onDelete: func(name string) {
			mu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			if inFlight >= 3 {
				once.Do(func() { close(reachedMulti) })
			}
			cur := inFlight
			mu.Unlock()

			if cur < 3 {
				select {
				case <-reachedMulti:
				case <-time.After(200 * time.Millisecond):
				}
			}

			mu.Lock()
			inFlight--
			mu.Unlock()
		},
	}

	res, err := Collect(context.Background(), kubeClient, dyn, store)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	mu.Lock()
	maxSeen := maxInFlight
	mu.Unlock()

	if maxSeen < 3 {
		t.Fatalf("expected concurrent deletes (max in flight >= 3), but max was %d", maxSeen)
	}
	if res.Deleted != total {
		t.Fatalf("expected %d deleted, got %d", total, res.Deleted)
	}
}

func TestRaceOnOwnerReferences(t *testing.T) {
	const totalDeps = 100
	kineData := map[string]string{}
	ns := &item{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "ns-default"},
		key:        "/registry/namespaces/default",
	}
	rawNs, _ := json.Marshal(ns)
	kineData[ns.key] = base64.StdEncoding.EncodeToString(rawNs)

	now := metav1.Now()
	for o := 0; o < 10; o++ {
		ownerName := fmt.Sprintf("owner-%d", o)
		owner := owned(ownerName)
		owner.Kind = "ReplicationController"
		owner.DeletionTimestamp = &now
		owner.Finalizers = []string{foregroundFinalizer}
		rawOwner, _ := json.Marshal(owner)
		kineData[owner.key] = base64.StdEncoding.EncodeToString(rawOwner)
	}

	for i := 0; i < totalDeps; i++ {
		name := fmt.Sprintf("dep-%d", i)
		o1 := fmt.Sprintf("owner-%d", i%10)
		o2 := fmt.Sprintf("owner-%d", (i+1)%10)
		dep := owned(name, ref(o1, true), ref(o2, true))
		dep.DeletionTimestamp = &now
		raw, _ := json.Marshal(dep)
		kineData[dep.key] = base64.StdEncoding.EncodeToString(raw)
	}

	store := &kine.Client{HTTP: &http.Client{Transport: &fakeKineStore{data: kineData}}}

	kubeClient := fakekube.NewSimpleClientset()
	fakeDisco := kubeClient.Discovery().(*fake.FakeDiscovery)
	fakeDisco.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod"},
				{Name: "replicationcontrollers", SingularName: "replicationcontroller", Namespaced: true, Kind: "ReplicationController"},
				{Name: "namespaces", SingularName: "namespace", Namespaced: false, Kind: "Namespace"},
			},
		},
	}

	dyn := &blockingDynamicClient{onDelete: func(name string) {}}
	_, err := Collect(context.Background(), kubeClient, dyn, store)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
}
