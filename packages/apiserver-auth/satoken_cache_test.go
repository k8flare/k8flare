package auth

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type countingObjects struct {
	fakeObjects
	calls int
}

func (c *countingObjects) ServiceAccount(ctx context.Context, ns, name string) (*corev1.ServiceAccount, error) {
	c.calls++
	return c.fakeObjects.ServiceAccount(ctx, ns, name)
}

func (c *countingObjects) Pod(ctx context.Context, ns, name string) (*corev1.Pod, error) {
	c.calls++
	return c.fakeObjects.Pod(ctx, ns, name)
}

func (c *countingObjects) Secret(ctx context.Context, ns, name string) (*corev1.Secret, error) {
	c.calls++
	return c.fakeObjects.Secret(ctx, ns, name)
}

func (c *countingObjects) Node(ctx context.Context, name string) (*corev1.Node, error) {
	c.calls++
	return c.fakeObjects.Node(ctx, name)
}

func newTestCache(objects ServiceAccountObjects) (*CachedObjects, *time.Time) {
	now := time.Unix(1000, 0)
	c := NewCachedObjects(objects, 10*time.Second)
	c.now = func() time.Time { return now }
	return c, &now
}

func TestCachedObjectsServesFoundWithinTTL(t *testing.T) {
	inner := &countingObjects{fakeObjects: objectsWithSA("default", "sa", "u1")}
	c, now := newTestCache(inner)
	for i := 0; i < 3; i++ {
		sa, err := c.ServiceAccount(context.Background(), "default", "sa")
		if err != nil || string(sa.UID) != "u1" {
			t.Fatalf("got %v %v", sa, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("calls %d", inner.calls)
	}
	*now = now.Add(11 * time.Second)
	if _, err := c.ServiceAccount(context.Background(), "default", "sa"); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 2 {
		t.Fatalf("calls after expiry %d", inner.calls)
	}
}

func TestCachedObjectsDoesNotCacheNotFound(t *testing.T) {
	inner := &countingObjects{fakeObjects: objectsWithSA("default", "other", "u")}
	c, _ := newTestCache(inner)
	if _, err := c.ServiceAccount(context.Background(), "default", "sa"); err == nil {
		t.Fatal("expected not found")
	}
	inner.serviceAccounts["default/sa"] = &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{UID: "u1"}}
	sa, err := c.ServiceAccount(context.Background(), "default", "sa")
	if err != nil || string(sa.UID) != "u1" {
		t.Fatalf("new object not visible: %v %v", sa, err)
	}
}

func TestCachedObjectsKeepsOnlyMinimalFacts(t *testing.T) {
	deleted := metav1.NewTime(time.Unix(900, 0))
	inner := &countingObjects{fakeObjects: objectsWithSA("default", "sa", "u1")}
	inner.secrets["default/s"] = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID("s1"), DeletionTimestamp: &deleted, Labels: map[string]string{"a": "b"}},
		Data:       map[string][]byte{"token": []byte("secret")},
	}
	c, _ := newTestCache(inner)
	if _, err := c.Secret(context.Background(), "default", "s"); err != nil {
		t.Fatal(err)
	}
	got, err := c.Secret(context.Background(), "default", "s")
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 {
		t.Fatalf("calls %d", inner.calls)
	}
	if got.UID != "s1" || got.DeletionTimestamp == nil || !got.DeletionTimestamp.Equal(&deleted) {
		t.Fatalf("facts lost: %+v", got.ObjectMeta)
	}
	if len(got.Data) != 0 || len(got.Labels) != 0 {
		t.Fatalf("contents cached: %+v", got)
	}
}

func TestCachedObjectsKeysDoNotCollideAcrossKinds(t *testing.T) {
	inner := &countingObjects{fakeObjects: objectsWithSA("default", "x", "sa-uid")}
	inner.pods["default/x"] = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "pod-uid"}}
	inner.nodes["x"] = &corev1.Node{ObjectMeta: metav1.ObjectMeta{UID: "node-uid"}}
	c, _ := newTestCache(inner)
	ctx := context.Background()
	sa, _ := c.ServiceAccount(ctx, "default", "x")
	pod, _ := c.Pod(ctx, "default", "x")
	node, _ := c.Node(ctx, "x")
	if sa.UID != "sa-uid" || pod.UID != "pod-uid" || node.UID != "node-uid" {
		t.Fatalf("%s %s %s", sa.UID, pod.UID, node.UID)
	}
}

func TestCachedObjectsDeletionVisibleAfterTTL(t *testing.T) {
	inner := &countingObjects{fakeObjects: objectsWithSA("default", "sa", "u1")}
	c, now := newTestCache(inner)
	if _, err := c.ServiceAccount(context.Background(), "default", "sa"); err != nil {
		t.Fatal(err)
	}
	delete(inner.serviceAccounts, "default/sa")
	*now = now.Add(11 * time.Second)
	if _, err := c.ServiceAccount(context.Background(), "default", "sa"); err == nil {
		t.Fatal("deleted object still served")
	}
}
