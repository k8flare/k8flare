package gc

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
