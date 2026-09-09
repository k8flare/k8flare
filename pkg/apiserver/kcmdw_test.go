package apiserver_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

// TestKCMDynamicWorkerControlPlane is the LOCAL stand-in for the parts of
// e2e-conformance.yml's dw variants that don't need a Linux kubelet: it
// runs wrangler dev with the real KCM/GC/sched dynamic workers ENABLED
// (no KCM_DISABLED kill switch, unlike every other test in this package)
// and drives the poke-pump control plane end to end with real client-go.
//
// This is the regression gate for the 2026-07-25 prod-only bug class --
// WatchHub's reserved-close-code exception storm, the namespace-lifecycle
// admission gap, and the events sweep race were all invisible to `make
// test` because it runs with controllers disabled. What a Mac cannot
// check remains out of scope here: actual Pod scheduling/binding needs a
// Node (Linux kubelet), which stays the conformance CI's job.
//
// Opt-in via K8FLARE_KCM_TEST=1 (`make test-kcm`): it boots its own
// wrangler dev on a fresh port with a throwaway state dir, and the first
// poke compiles three ~40MB WASM modules inside workerd, so it is far
// slower than the rest of the suite.
func TestKCMDynamicWorkerControlPlane(t *testing.T) {
	if os.Getenv("K8FLARE_KCM_TEST") != "1" {
		t.Skip("KCM dynamic-worker smoke is opt-in: set K8FLARE_KCM_TEST=1 (make test-kcm)")
	}

	port := findFreePort(t)
	projectRoot := findProjectRoot(t)

	// Same flag rationale as setupWranglerDev (apiserver_test.go), minus
	// KCM_DISABLED, plus an isolated state dir so this run can't inherit
	// or clobber the main suite's persisted DO state.
	cmd := exec.Command("npx", "wrangler", "dev",
		"-c", "packages/k8flare-worker/wrangler.jsonc",
		"--enable-containers=false",
		"--local",
		"--port", fmt.Sprintf("%d", port),
		"--persist-to", t.TempDir(),
		"--log-level", "error",
		// Every third resident dispatch loses its pump-window close, the
		// way production loses one whose IoContext is torn down before
		// ctx.waitUntil's timer runs. `wrangler dev` never does that on
		// its own (S31 E1), and one lost close used to wedge the whole
		// resident instance until a fresh Loader id -- measured
		// 2026-09-09 as "no podCIDR after 3 minutes" with this set and
		// the window expiry backstop reverted (S31 addendum). So this
		// whole lane doubles as that regression's gate.
		"--var", "PUMP_WINDOW_DROP_CLOSE:3",
	)
	cmd.Dir = projectRoot
	devNull, _ := os.Open(os.DevNull)
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start wrangler dev: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	})

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			time.Sleep(2 * time.Second)
			break
		}
		time.Sleep(time.Second)
	}

	client, err := kubernetes.NewForConfig(&rest.Config{
		Host:        fmt.Sprintf("http://127.0.0.1:%d", port),
		BearerToken: "k8flare-dev-token",
		// Bounds a single request so a stuck connection fails fast
		// instead of blocking waitFor's deadline check indefinitely
		// (found 2026-08-09: an unbounded request hung the whole
		// process past its outer `go test -timeout`, in
		// clusterop_test.go which shares this same waitFor helper).
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	ctx := context.Background()

	const ns = "kcmdw-smoke"
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	replicas := int32(2)
	kcmLoadedAt := time.Now()
	if _, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.27"}},
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	// The real KCM (deployment + replicaset controllers, loaded as a
	// dynamic worker by this write's poke) must produce 2 Pods. First
	// poke includes the in-workerd WASM compile, so this phase gets the
	// largest budget of the test; a green run typically finishes in well
	// under a minute. Every budget in this test is sized so that their
	// SUM plus the fixed sleep below stays comfortably inside the
	// harness `-timeout 15m` (Makefile's test-kcm, ci.yml): overrunning
	// that is a `panic: test timed out` in which t.Cleanup never runs and
	// the wrangler dev process group is leaked.
	waitFor(t, 3*time.Minute, "KCM created 2 pods", func() bool {
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		return err == nil && len(pods.Items) == 2
	})

	// Two registered Nodes, each with a Ready status and a heartbeat
	// Lease its "kubelet" keeps renewing -- the input the real nodeipam
	// and nodelifecycle controllers work from. Both replaced
	// pkg/apiserver's own AssignPodCIDR/ReconcileNodeLifecycle on
	// 2026-09-09 (docs/platform-verification.md S28), so the assertions
	// below port the load-bearing part of what those deleted unit tests
	// covered, now driven through the real controllers. What was dropped
	// rather than ported, and why, is listed in S28.
	const nodeA, nodeB = "kcmdw-node-a", "kcmdw-node-b"
	for _, name := range []string{nodeA, nodeB} {
		if _, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{corev1.LabelHostname: name},
			},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create node %s: %v", name, err)
		}
		t.Cleanup(func() {
			_ = client.CoreV1().Nodes().Delete(context.Background(), name, metav1.DeleteOptions{})
			_ = client.CoordinationV1().Leases(corev1.NamespaceNodeLease).
				Delete(context.Background(), name, metav1.DeleteOptions{})
		})
		markNodeReady(t, ctx, client, name)
	}
	stopLeaseA := keepNodeLeaseFresh(ctx, client, nodeA)
	defer stopLeaseA()
	stopLeaseB := keepNodeLeaseFresh(ctx, client, nodeB)
	defer stopLeaseB()

	// The real nodeipam controller must hand each Node a distinct /24 out
	// of pkg/controllers/controllermanager.go's clusterCIDR (10.42.0.0/16)
	// -- the assertions the deleted nodecidr_allocator_test.go made
	// against the hand-written allocator.
	_, clusterCIDR, err := net.ParseCIDR("10.42.0.0/16")
	if err != nil {
		t.Fatalf("parse cluster CIDR: %v", err)
	}
	podCIDRs := map[string]string{}
	for _, name := range []string{nodeA, nodeB} {
		waitFor(t, time.Minute, "nodeipam assigned "+name+" a podCIDR", func() bool {
			node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
			if err != nil || node.Spec.PodCIDR == "" {
				return false
			}
			podCIDRs[name] = node.Spec.PodCIDR
			return true
		})
		node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get node %s: %v", name, err)
		}
		ip, block, err := net.ParseCIDR(node.Spec.PodCIDR)
		if err != nil {
			t.Fatalf("node %s podCIDR %q is not a CIDR: %v", name, node.Spec.PodCIDR, err)
		}
		if !clusterCIDR.Contains(ip) {
			t.Errorf("node %s podCIDR %s is outside the cluster CIDR %s", name, node.Spec.PodCIDR, clusterCIDR)
		}
		if size, _ := block.Mask.Size(); size != 24 {
			t.Errorf("node %s podCIDR %s is a /%d, want a /24", name, node.Spec.PodCIDR, size)
		}
		if len(node.Spec.PodCIDRs) != 1 || node.Spec.PodCIDRs[0] != node.Spec.PodCIDR {
			t.Errorf("node %s podCIDRs = %v, want [%s]", name, node.Spec.PodCIDRs, node.Spec.PodCIDR)
		}
	}
	if podCIDRs[nodeA] == podCIDRs[nodeB] {
		t.Errorf("expected distinct podCIDRs, both nodes got %s", podCIDRs[nodeA])
	}

	// A Service with a selector plus a matching, ready Pod must get both
	// a legacy Endpoints and an EndpointSlice from the real
	// endpoint/endpointslice controllers -- what the deleted
	// endpoints_test.go asserted against ReconcileNamespaceEndpoints.
	// The Pod is pre-bound to nodeA: the real endpointslice reconciler
	// skips any Pod whose spec.nodeName does not resolve to a Node in its
	// informer cache (k8s.io/endpointslice's reconciler.go), so an
	// unscheduled Pod would never be published.
	if _, err := client.CoreV1().Services(ns).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ep-web"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "ep-web"},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromString("http"),
			}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create service: %v", err)
	}
	// A Service with no selector is the endpoint controller's explicit
	// no-op case: those Endpoints belong to whoever manages them by hand,
	// and the controller must never create or touch one. What the deleted
	// endpoints_test.go's selector-less case covered.
	if _, err := client.CoreV1().Services(ns).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ep-noselector"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create selector-less service: %v", err)
	}
	if _, err := client.CoreV1().Pods(ns).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ep-web-1", Labels: map[string]string{"app": "ep-web"}},
		Spec: corev1.PodSpec{
			NodeName: nodeA,
			Containers: []corev1.Container{{
				Name:  "c",
				Image: "nginx:1.27",
				Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP}},
			}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create endpoints pod: %v", err)
	}
	// Get-modify-update: the live controllers write to this Pod too, so a
	// single-shot UpdateStatus can lose a resourceVersion race and 409.
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		epPod, err := client.CoreV1().Pods(ns).Get(ctx, "ep-web-1", metav1.GetOptions{})
		if err != nil {
			return err
		}
		epPod.Status = corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      "10.42.0.5",
			PodIPs:     []corev1.PodIP{{IP: "10.42.0.5"}},
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		}
		_, err = client.CoreV1().Pods(ns).UpdateStatus(ctx, epPod, metav1.UpdateOptions{})
		return err
	}); err != nil {
		t.Fatalf("update endpoints pod status: %v", err)
	}

	waitFor(t, time.Minute, "endpoint controller published the ready pod", func() bool {
		eps, err := client.CoreV1().Endpoints(ns).Get(ctx, "ep-web", metav1.GetOptions{})
		if err != nil || len(eps.Subsets) != 1 {
			return false
		}
		sub := eps.Subsets[0]
		return len(sub.Addresses) == 1 && sub.Addresses[0].IP == "10.42.0.5" &&
			len(sub.NotReadyAddresses) == 0 &&
			len(sub.Ports) == 1 && sub.Ports[0].Port == 8080
	})
	eps, err := client.CoreV1().Endpoints(ns).Get(ctx, "ep-web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get endpoints: %v", err)
	}
	if got := eps.Labels["endpoints.kubernetes.io/managed-by"]; got != "endpoint-controller" {
		t.Errorf("Endpoints managed-by = %q, want endpoint-controller", got)
	}
	if ref := eps.Subsets[0].Addresses[0].TargetRef; ref == nil || ref.Name != "ep-web-1" {
		t.Errorf("Endpoints targetRef = %+v, want the ep-web-1 Pod", ref)
	}
	// The controller has demonstrably processed this namespace's Services
	// by now, so the selector-less one still having no Endpoints is a
	// real assertion rather than a race with a pending sync.
	if _, err := client.CoreV1().Endpoints(ns).Get(ctx, "ep-noselector", metav1.GetOptions{}); !errors.IsNotFound(err) {
		t.Errorf("endpoint controller created Endpoints for a selector-less Service (get err = %v)", err)
	}

	waitFor(t, time.Minute, "endpointslice controller published the ready pod", func() bool {
		slices, err := client.DiscoveryV1().EndpointSlices(ns).List(ctx, metav1.ListOptions{
			LabelSelector: discoveryv1.LabelServiceName + "=ep-web",
		})
		if err != nil || len(slices.Items) != 1 {
			return false
		}
		slice := slices.Items[0]
		return len(slice.Endpoints) == 1 && len(slice.Endpoints[0].Addresses) == 1 &&
			slice.Endpoints[0].Addresses[0] == "10.42.0.5"
	})
	slices, err := client.DiscoveryV1().EndpointSlices(ns).List(ctx, metav1.ListOptions{
		LabelSelector: discoveryv1.LabelServiceName + "=ep-web",
	})
	if err != nil {
		t.Fatalf("list endpointslices: %v", err)
	}
	slice := slices.Items[0]
	if got := slice.Labels[discoveryv1.LabelManagedBy]; got != "endpointslice-controller.k8s.io" {
		t.Errorf("EndpointSlice managed-by = %q, want endpointslice-controller.k8s.io", got)
	}
	if slice.AddressType != discoveryv1.AddressTypeIPv4 {
		t.Errorf("EndpointSlice addressType = %q, want IPv4", slice.AddressType)
	}
	if ready := slice.Endpoints[0].Conditions.Ready; ready == nil || !*ready {
		t.Errorf("EndpointSlice endpoint ready = %v, want true", ready)
	}
	if len(slice.Ports) != 1 || slice.Ports[0].Port == nil || *slice.Ports[0].Port != 8080 {
		t.Errorf("EndpointSlice ports = %+v, want the resolved container port 8080", slice.Ports)
	}

	// A Node registered LONG after the kcm dynamic worker was loaded must
	// be served just as fast as one registered right after it. Everything
	// above happens within the first pump window or two, so it cannot
	// tell a reflector that is still watching from one whose watch died
	// at a pump-window boundary and was never re-established -- the S31
	// production defect, where a Node registered 13 minutes in went
	// 6+ minutes without a podCIDR while the Controllers DO kept pumping
	// every 15s, and only a fresh Loader id fixed it.
	//
	// The budget is the elapsed test so far, topped up only if the
	// assertions above ran unusually fast, so this phase normally costs
	// nothing beyond the bounded wait at the end of it.
	if remaining := 3*time.Minute - time.Since(kcmLoadedAt); remaining > 0 {
		t.Logf("waiting %s more so the kcm has been live for 3 minutes before the late Node registers", remaining.Round(time.Second))
		time.Sleep(remaining)
	}
	const nodeLate = "kcmdw-node-late"
	if _, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeLate},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create late node: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Nodes().Delete(context.Background(), nodeLate, metav1.DeleteOptions{})
		_ = client.CoordinationV1().Leases(corev1.NamespaceNodeLease).
			Delete(context.Background(), nodeLate, metav1.DeleteOptions{})
	})
	markNodeReady(t, ctx, client, nodeLate)
	stopLeaseLate := keepNodeLeaseFresh(ctx, client, nodeLate)
	defer stopLeaseLate()
	waitFor(t, time.Minute, "nodeipam assigned a podCIDR to a Node registered 3 minutes after kcm loaded", func() bool {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeLate, metav1.GetOptions{})
		return err == nil && node.Spec.PodCIDR != ""
	})

	// Namespace deletion must leave nothing behind, with the LIVE
	// controllers racing the sweep (admission + post-delete events
	// sweep regression: commits 9dfbb35 / a012a01).
	if err := client.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}
	waitFor(t, time.Minute, "namespace fully deleted", func() bool {
		_, err := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		return err != nil
	})
	// Two different waits, and conflating them is what made this flaky.
	//
	// Reaching zero Pods is CONVERGENCE: it takes as long as the garbage
	// collector takes, so it has to be polled. It used to be a flat 30s
	// sleep followed by a hard assertion, which fails whenever GC is
	// merely slow -- observed once in 8 runs, at full test duration
	// rather than at startup. Suppressing no-op writes plausibly made
	// that more likely: fewer writes mean fewer pokes, so the controllers
	// get fewer pump windows and the backoff stretches sooner (the
	// tradeoff recorded as S26b in docs/platform-verification.md).
	waitFor(t, 90*time.Second, "all pods gone cluster-wide", func() bool {
		pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		return err == nil && len(pods.Items) == 0
	})

	// Proving ABSENCE is the other kind, and that one does need a fixed
	// window: a straggler write lands after the state first looks clean,
	// so polling until it looks clean would just race it. Hold still and
	// then re-check -- including the Pods, since a straggler could have
	// recreated one after the wait above passed.
	time.Sleep(30 * time.Second)
	pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Errorf("a straggler write recreated %d pod(s) after the namespace was deleted", len(pods.Items))
	}
	events, err := client.CoreV1().Events(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, ev := range events.Items {
		if ev.Namespace == ns {
			t.Errorf("orphan event survived namespace delete: %s/%s", ev.Namespace, ev.Name)
		}
	}

	// Lease staleness, last: this is the assertion the deleted
	// nodelifecycle_test.go used to make against pkg/apiserver's own
	// stand-in, now driven through the real controller. It does NOT
	// isolate which of the two alarms opened the pump window it needed
	// (the Cluster DO's node-lifecycle safety net or the Controllers
	// DO's own) -- measured 2026-09-09, docs/platform-verification.md
	// S28.
	//
	// nodeMonitorGracePeriod is 50s and the controller anchors its grace
	// window to the last heartbeat it OBSERVED, so the marking lands
	// relative to the last renewal, not 50s after the backdated
	// renewTime.
	stopLeaseB()
	staleLease, err := client.CoordinationV1().Leases(corev1.NamespaceNodeLease).
		Get(ctx, nodeB, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get node lease: %v", err)
	}
	backdated := metav1.NewMicroTime(time.Now().Add(-5 * time.Minute))
	staleLease.Spec.RenewTime = &backdated
	if _, err := client.CoordinationV1().Leases(corev1.NamespaceNodeLease).
		Update(ctx, staleLease, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("backdate node lease: %v", err)
	}

	waitFor(t, 2*time.Minute, "nodelifecycle marked the stale node Ready=Unknown and unreachable", func() bool {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeB, metav1.GetOptions{})
		if err != nil {
			return false
		}
		unknown := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				unknown = cond.Status == corev1.ConditionUnknown && cond.Reason == "NodeStatusUnknown"
			}
		}
		return unknown && hasTaint(node, corev1.TaintNodeUnreachable)
	})
	staleNode, err := client.CoreV1().Nodes().Get(ctx, nodeB, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get stale node: %v", err)
	}
	if !hasTaint(staleNode, corev1.TaintNodeUnreachable) {
		t.Errorf("stale node taints = %+v, want one keyed %s", staleNode.Spec.Taints, corev1.TaintNodeUnreachable)
	}
	// The other three conditions monitorNodeHealth transitions alongside
	// Ready (NodeNetworkUnavailable is deliberately excluded upstream).
	for _, want := range []corev1.NodeConditionType{
		corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure,
	} {
		found := false
		for _, cond := range staleNode.Status.Conditions {
			if cond.Type == want {
				found = true
				if cond.Status != corev1.ConditionUnknown {
					t.Errorf("condition %s = %s, want Unknown", want, cond.Status)
				}
			}
		}
		if !found {
			t.Errorf("condition %s was never synthesized on the stale node", want)
		}
	}
	// A Node whose Lease is fresh must be left alone -- the common case
	// the deleted TestReconcileNodeLifecycle_FreshLeaseIsUntouched covered.
	healthyNode, err := client.CoreV1().Nodes().Get(ctx, nodeA, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get healthy node: %v", err)
	}
	for _, cond := range healthyNode.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
			t.Errorf("node with a fresh Lease was marked %s, want it left Ready=True", cond.Status)
		}
	}

	// Coming BACK is its own assertion, and the one production actually
	// failed: a node paused for 9.5 minutes went Ready=True the moment
	// its kubelet resumed, yet the unreachable taints stayed on it and
	// its replacement Pods stayed Pending for 5+ minutes, until a fresh
	// Loader id (docs/platform-verification.md, S31 addendum). Marking a
	// node unreachable exercises the informers up to the point of
	// failure; only un-marking it proves they are still live afterwards.
	//
	// The Pod pinned to the recovered node makes the consequence visible
	// rather than just the taint: while the NoSchedule taint is there it
	// cannot be placed anywhere (nodeA is excluded by the selector), so
	// its binding is exactly what the production report was missing.
	const recoveryNS = "kcmdw-recovery"
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: recoveryNS},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create recovery namespace: %v", err)
	}
	if _, err := client.CoreV1().Pods(recoveryNS).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pinned"},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{corev1.LabelHostname: nodeB},
			Containers:   []corev1.Container{{Name: "c", Image: "nginx:1.27"}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pinned pod: %v", err)
	}

	markNodeReady(t, ctx, client, nodeB)
	stopLeaseB2 := keepNodeLeaseFresh(ctx, client, nodeB)
	defer stopLeaseB2()

	waitFor(t, 90*time.Second, "nodelifecycle un-tainted the recovered node", func() bool {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeB, metav1.GetOptions{})
		if err != nil {
			return false
		}
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				ready = cond.Status == corev1.ConditionTrue
			}
		}
		return ready && !hasTaint(node, corev1.TaintNodeUnreachable) &&
			!hasTaint(node, corev1.TaintNodeNotReady)
	})
	waitFor(t, 90*time.Second, "scheduler placed a Pod on the recovered node", func() bool {
		pod, err := client.CoreV1().Pods(recoveryNS).Get(ctx, "pinned", metav1.GetOptions{})
		return err == nil && pod.Spec.NodeName == nodeB
	})
}

// hasTaint reports whether node carries a taint with this key, whatever
// its effect: nodelifecycle applies the unreachable taint as NoSchedule
// and NoExecute in two separate passes, and this test only cares that
// the Node was tainted at all.
func hasTaint(node *corev1.Node, key string) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == key {
			return true
		}
	}
	return false
}

// markNodeReady posts the Ready=True status a real kubelet posts at
// registration -- and re-posts it when a recovered "kubelet" starts
// heartbeating again. Without it the real nodelifecycle controller treats
// the Node as one whose kubelet never reported and uses
// nodeStartupGracePeriod instead of nodeMonitorGracePeriod. The capacity
// is what a real kubelet reports too, and the scheduler needs it: with
// allocatable.pods absent, NodeResourcesFit rejects every Pod for
// exceeding an allowed pod count of zero.
//
// Get-modify-update: the live nodelifecycle controller writes this same
// status, so a single-shot UpdateStatus can lose a resourceVersion race.
func markNodeReady(t *testing.T, ctx context.Context, client kubernetes.Interface, name string) {
	t.Helper()
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		now := metav1.NewTime(time.Now())
		node.Status.Capacity = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
			corev1.ResourcePods:   resource.MustParse("110"),
		}
		node.Status.Allocatable = node.Status.Capacity
		node.Status.Conditions = []corev1.NodeCondition{{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionTrue,
			Reason:             "KubeletReady",
			LastHeartbeatTime:  now,
			LastTransitionTime: now,
		}}
		_, err = client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{})
		return err
	}); err != nil {
		t.Fatalf("mark node %s ready: %v", name, err)
	}
}

// keepNodeLeaseFresh renews name's kube-node-lease Lease every 10s until
// the returned stop function is called -- the kubelet heartbeat the real
// nodelifecycle controller reads to decide a Node is still healthy.
// Without it every Node in this test would be marked Ready=Unknown
// 50s (nodeMonitorGracePeriod) after the controller first saw it, and its
// Pods would stop being published as ready endpoints mid-assertion.
func keepNodeLeaseFresh(ctx context.Context, client kubernetes.Interface, name string) func() {
	renew := func() {
		now := metav1.NewMicroTime(time.Now())
		leases := client.CoordinationV1().Leases(corev1.NamespaceNodeLease)
		if lease, err := leases.Get(ctx, name, metav1.GetOptions{}); err == nil {
			lease.Spec.RenewTime = &now
			_, _ = leases.Update(ctx, lease, metav1.UpdateOptions{})
			return
		}
		durationSeconds := int32(40)
		_, _ = leases.Create(ctx, &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &name,
				LeaseDurationSeconds: &durationSeconds,
				RenewTime:            &now,
			},
		}, metav1.CreateOptions{})
	}
	renew()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				renew()
			}
		}
	}()
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		close(stop)
		<-done
	}
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, what)
}
