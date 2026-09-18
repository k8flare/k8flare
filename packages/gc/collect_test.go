package gc

import (
	"testing"

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
