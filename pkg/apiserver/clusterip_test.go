package apiserver_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestClusterIPAllocation_SynchronousAndNoDoubleAllocation verifies the
// Phase 3 ClusterIP boundary condition end-to-end against a real wrangler
// dev stack (not just read from source, per CLAUDE.md's "actually run it"
// rule): the Go apiserver now allocates a Service's ClusterIP synchronously
// in the create path (clusterip.go), and
// workers/storage/src/serviceip.ts's alarm-driven allocateClusterIPs (kept
// as-is -- this project's storage boundary for this change) must not
// double-allocate or clobber it on its next pass.
func TestClusterIPAllocation_SynchronousAndNoDoubleAllocation(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	for _, name := range []string{"svc-a", "svc-b", "svc-headless"} {
		_ = client.CoreV1().Services(ns).Delete(ctx, name, metav1.DeleteOptions{})
	}
	t.Cleanup(func() {
		for _, name := range []string{"svc-a", "svc-b", "svc-headless"} {
			_ = client.CoreV1().Services(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
		}
	})

	var svcA, svcB *corev1.Service

	t.Run("AllocatesSynchronouslyOnCreate", func(t *testing.T) {
		var err error
		svcA, err = client.CoreV1().Services(ns).Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-a"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create svc-a: %v", err)
		}
		// If allocation were still async (TS alarm-driven only), the
		// Create response itself would come back with an empty ClusterIP.
		if svcA.Spec.ClusterIP == "" {
			t.Fatal("svc-a: ClusterIP is empty in the Create response -- allocation is not synchronous")
		}
		if svcA.Spec.ClusterIP == "None" {
			t.Fatal("svc-a: ClusterIP is \"None\", want a real allocated address")
		}
		if len(svcA.Spec.ClusterIPs) == 0 || svcA.Spec.ClusterIPs[0] != svcA.Spec.ClusterIP {
			t.Errorf("svc-a: ClusterIPs = %v, want [%s]", svcA.Spec.ClusterIPs, svcA.Spec.ClusterIP)
		}
	})

	t.Run("DistinctAddressesForDistinctServices", func(t *testing.T) {
		var err error
		svcB, err = client.CoreV1().Services(ns).Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-b"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create svc-b: %v", err)
		}
		if svcB.Spec.ClusterIP == "" || svcB.Spec.ClusterIP == "None" {
			t.Fatalf("svc-b: ClusterIP = %q, want a real allocated address", svcB.Spec.ClusterIP)
		}
		if svcB.Spec.ClusterIP == svcA.Spec.ClusterIP {
			t.Errorf("svc-a and svc-b both got ClusterIP %s -- collision", svcA.Spec.ClusterIP)
		}
	})

	t.Run("HeadlessServiceUnaffected", func(t *testing.T) {
		svc, err := client.CoreV1().Services(ns).Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-headless"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "None",
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create svc-headless: %v", err)
		}
		if svc.Spec.ClusterIP != "None" {
			t.Errorf("svc-headless: ClusterIP = %q, want \"None\" (unchanged)", svc.Spec.ClusterIP)
		}
	})

	t.Run("SurvivesAlarmPass", func(t *testing.T) {
		// Force the Cluster DO's alarm to fire: a Pod write reliably wakes
		// it (needsSchedulerAttention in workers/storage/src/scheduler.ts).
		// The DO's alarm() handler unconditionally runs
		// allocateClusterIPs on every firing (not just when
		// needsServiceIPAttention triggered the wake) -- see
		// workers/storage/src/index.ts -- so this exercises the exact
		// "TS pass runs again after Go already allocated" race the
		// boundary condition is about, whether or not the Service write
		// itself was what triggered this particular firing.
		_, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "clusterip-test-alarm-wake"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Create wake pod: %v", err)
		}
		t.Cleanup(func() {
			_ = client.CoreV1().Pods(ns).Delete(ctx, "clusterip-test-alarm-wake", metav1.DeleteOptions{})
		})

		// DEBOUNCE_MS is 1s; give the alarm emulation comfortable margin
		// beyond that to actually fire (see CLAUDE.md's wrangler dev alarm
		// footgun note).
		time.Sleep(4 * time.Second)

		gotA, err := client.CoreV1().Services(ns).Get(ctx, "svc-a", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get svc-a: %v", err)
		}
		if gotA.Spec.ClusterIP != svcA.Spec.ClusterIP {
			t.Errorf("svc-a ClusterIP changed after an alarm pass: got %s, want %s (double-allocation / clobber)", gotA.Spec.ClusterIP, svcA.Spec.ClusterIP)
		}

		gotB, err := client.CoreV1().Services(ns).Get(ctx, "svc-b", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Get svc-b: %v", err)
		}
		if gotB.Spec.ClusterIP != svcB.Spec.ClusterIP {
			t.Errorf("svc-b ClusterIP changed after an alarm pass: got %s, want %s (double-allocation / clobber)", gotB.Spec.ClusterIP, svcB.Spec.ClusterIP)
		}
	})
}

// TestClusterIPAllocation_ConcurrentCreatesGetDistinctAddresses exercises
// ClusterIPAllocator's actual reason for existing: concurrent Create
// requests race to CAS-update the same persisted bitmap
// (clusterip.go's save, keyed on clusterIPRangeKey), and AllocateNext must
// retry-from-reload on a losing race rather than silently handing out a
// duplicate or corrupting the bitmap. The sequential creates in
// TestClusterIPAllocation_SynchronousAndNoDoubleAllocation can't exercise
// this path -- by the time the second Create runs, the first's write has
// already landed, so there's never an actual conflict to retry from. This
// test fires N Creates from concurrent goroutines instead.
func TestClusterIPAllocation_ConcurrentCreatesGetDistinctAddresses(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})

	const n = 8
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("svc-concurrent-%d", i)
		_ = client.CoreV1().Services(ns).Delete(ctx, names[i], metav1.DeleteOptions{})
	}
	t.Cleanup(func() {
		for _, name := range names {
			_ = client.CoreV1().Services(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
		}
	})

	var wg sync.WaitGroup
	ips := make([]string, n)
	errs := make([]error, n)
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			svc, err := client.CoreV1().Services(ns).Create(ctx, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
			}, metav1.CreateOptions{})
			if err != nil {
				errs[i] = err
				return
			}
			ips[i] = svc.Spec.ClusterIP
		}(i, name)
	}
	wg.Wait()

	seen := make(map[string]string, n) // ip -> first service name that got it
	for i, name := range names {
		if errs[i] != nil {
			t.Errorf("Create %s: %v", name, errs[i])
			continue
		}
		if ips[i] == "" || ips[i] == "None" {
			t.Errorf("%s: ClusterIP = %q, want a real allocated address", name, ips[i])
			continue
		}
		if owner, dup := seen[ips[i]]; dup {
			t.Errorf("%s and %s both got ClusterIP %s -- concurrent allocation collided", name, owner, ips[i])
			continue
		}
		seen[ips[i]] = name
	}
}
