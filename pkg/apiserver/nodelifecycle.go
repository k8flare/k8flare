package apiserver

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	kubectrlmgrconfigv1alpha1 "k8s.io/kube-controller-manager/config/v1alpha1"
	nodelifecycleconfigv1alpha1 "k8s.io/kubernetes/pkg/controller/nodelifecycle/config/v1alpha1"
	taintutils "k8s.io/kubernetes/pkg/util/taints"
)

// nodeMonitorGracePeriod and podEvictionTimeout are the real upstream
// defaults (RecommendedDefaultNodeLifecycleControllerConfiguration, called
// in init() below), not hand-copied literals -- the deleted
// nodelifecycle.ts hardcoded 40s/5m and its own comment claimed 40s
// "matches upstream's default", which docs/platform-verification.md's
// Phase 5 findings later measured as wrong (the real default is 50s,
// verified directly off a running cmd/controller-manager -v=2). Reading
// them from the real upstream defaulter here means a future k8s version
// bump can change these values without this file drifting out of sync
// silently, the same property zz_generated_version.go has for the
// Major/Minor/GitVersion this project reports.
var (
	nodeMonitorGracePeriod time.Duration
	podEvictionTimeout     time.Duration
)

func init() {
	var cfg kubectrlmgrconfigv1alpha1.NodeLifecycleControllerConfiguration
	nodelifecycleconfigv1alpha1.RecommendedDefaultNodeLifecycleControllerConfiguration(&cfg)
	nodeMonitorGracePeriod = cfg.NodeMonitorGracePeriod.Duration
	podEvictionTimeout = cfg.PodEvictionTimeout.Duration
}

// unreachableTaint mirrors
// k8s.io/kubernetes/pkg/controller/nodelifecycle.UnreachableTaintTemplate
// (node_lifecycle_controller.go) -- hand-copied, not imported, because that
// package is one of the five real controllers docs/platform-verification.md's
// Phase 5 findings measured at +6MiB+ gzip each (client-go informers/
// listers/workqueue-entangled at the package level, same as every other
// real KCM controller) -- importing it just for this one *v1.Taint literal
// would pay that whole cost. taintutils (k8s.io/kubernetes/pkg/util/taints,
// imported below for AddOrUpdateTaint) is a separate, genuinely lightweight
// package (measured: cidrset + this + the config/v1alpha1 defaulter above,
// combined, cost +59,562 bytes gzip -- see docs/cost-model.md) that the
// heavy controller package also happens to depend on, so reusing it
// directly here is real upstream code, not a re-implementation.
var unreachableTaint = &corev1.Taint{
	Key:    corev1.TaintNodeUnreachable,
	Effect: corev1.TaintEffectNoExecute,
}

// nodeUnknownConditionTypes mirrors node_lifecycle_controller.go's
// nodeConditionTypes list (the exact 4 conditions transitioned to Unknown on
// Lease staleness; NodeNetworkUnavailable is deliberately excluded there --
// "it's managed on a control plane level" -- and here too).
var nodeUnknownConditionTypes = []corev1.NodeConditionType{
	corev1.NodeReady,
	corev1.NodeMemoryPressure,
	corev1.NodeDiskPressure,
	corev1.NodePIDPressure,
}

// ReconcileNodeLifecycle detects Nodes whose Lease has gone stale, marks
// them Unknown + taints them unreachable, and evicts their Pods once stale
// for long enough -- restores workers/storage/src/nodelifecycle.ts (deleted
// in 0061a26 on the same mistaken premise as endpoints.ts/scheduler.ts, see
// docs/platform-verification.md's Phase 5 findings), now backed by real
// upstream grace-period defaults and taint machinery instead of hand-rolled
// constants.
//
// Unlike AssignClusterIP/AssignPodCIDR/ReconcileNamespaceEndpoints (all
// triggered by a specific write), staleness is detected by the ABSENCE of
// an expected Lease renewal -- there is no write to hook this to. It must
// run periodically; see packages/k8flare-worker/src/storage/index.ts's alarm loop, which
// calls this via a service-binding fetch to apiserver's
// /internal/reconcile-node-lifecycle route (main.go) on its existing
// event-armed safety-net alarm, extended to also re-arm while any Node is
// live (previously only Services did).
//
// Deliberately not implemented, matching nodelifecycle.ts's own documented
// gap: recovery (a node coming back healthy has its Unknown status cleared
// automatically by kubelet's own next heartbeat overwriting status, but
// nothing here proactively removes a taint on its own), and
// TolerationSeconds-aware deferred eviction (real taint-eviction-controller's
// per-pod timed-worker-queue design -- a materially larger feature; this
// evicts immediately once podEvictionTimeout has elapsed, except for Pods
// that explicitly tolerate the unreachable taint forever, e.g. DaemonSet
// pods -- see podToleratesUnreachableForever below).
func ReconcileNodeLifecycle(ctx context.Context, storage *Storage) error {
	nodeStore := NewResourceStore(storage, corev1.SchemeGroupVersion, "nodes", "node", false,
		func() runtime.Object { return &corev1.Node{} },
		func() runtime.Object { return &corev1.NodeList{} },
	)
	leaseStore := NewResourceStore(storage, coordinationv1.SchemeGroupVersion, "leases", "lease", true,
		func() runtime.Object { return &coordinationv1.Lease{} },
		func() runtime.Object { return &coordinationv1.LeaseList{} },
	)

	nodeListObj, err := nodeStore.List(ctx, "", "", "")
	if err != nil {
		return fmt.Errorf("reconcile node lifecycle: list nodes: %w", err)
	}
	nodeList, ok := nodeListObj.(*corev1.NodeList)
	if !ok {
		return fmt.Errorf("reconcile node lifecycle: unexpected list type %T for nodes", nodeListObj)
	}

	now := time.Now()
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if err := reconcileOneNode(ctx, storage, nodeStore, leaseStore, node, now); err != nil {
			log.Printf("node lifecycle reconciliation error for node %s: %v", node.Name, err)
		}
	}
	return nil
}

func reconcileOneNode(ctx context.Context, storage *Storage, nodeStore, leaseStore *ResourceStore, node *corev1.Node, now time.Time) error {
	// Node lease objects live in the reserved "kube-node-lease" namespace,
	// named after the Node -- standard upstream convention
	// (k8s.io/api/core/v1.NamespaceNodeLease), same key nodelifecycle.ts read.
	leaseObj, err := leaseStore.Get(ctx, corev1.NamespaceNodeLease, node.Name)
	if err != nil {
		if isNotFoundErr(err) {
			return nil // no lease yet -- node hasn't finished registering
		}
		return fmt.Errorf("get lease: %w", err)
	}
	lease := leaseObj.(*coordinationv1.Lease)
	if lease.Spec.RenewTime == nil {
		return nil
	}

	stale := now.Sub(lease.Spec.RenewTime.Time)
	if stale <= nodeMonitorGracePeriod {
		return nil
	}

	changed := markNodeUnknown(node, now)
	if _, err := ensureUnreachableTaint(ctx, nodeStore, node, changed); err != nil {
		return fmt.Errorf("taint node: %w", err)
	}

	if stale > podEvictionTimeout {
		if err := evictPodsOnNode(ctx, storage, node.Name); err != nil {
			return fmt.Errorf("evict pods: %w", err)
		}
	}
	return nil
}

// markNodeUnknown transitions node's Ready/MemoryPressure/DiskPressure/
// PIDPressure conditions to Unknown, mirroring
// node_lifecycle_controller.go's monitorNodeHealth (lines ~921-957 as of
// k8s.io/kubernetes@v1.36.2-k3s1): a condition that already exists and
// isn't already Unknown gets Reason="NodeStatusUnknown",
// Message="Kubelet stopped posting node status." (both literal upstream
// strings); a condition the kubelet never reported at all gets synthesized
// with Reason="NodeStatusNeverUpdated", Message="Kubelet never posted node
// status." -- a case nodelifecycle.ts never handled (it only ever mutated
// conditions already present in the list). Returns whether anything
// changed.
func markNodeUnknown(node *corev1.Node, now time.Time) bool {
	changed := false
	nowMeta := metav1.NewTime(now)

	for _, condType := range nodeUnknownConditionTypes {
		found := false
		for i := range node.Status.Conditions {
			cond := &node.Status.Conditions[i]
			if cond.Type != condType {
				continue
			}
			found = true
			if cond.Status != corev1.ConditionUnknown {
				cond.Status = corev1.ConditionUnknown
				cond.LastTransitionTime = nowMeta
				cond.Reason = "NodeStatusUnknown"
				cond.Message = "Kubelet stopped posting node status."
				changed = true
			}
			break
		}
		if !found {
			node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
				Type:               condType,
				Status:             corev1.ConditionUnknown,
				Reason:             "NodeStatusNeverUpdated",
				Message:            "Kubelet never posted node status.",
				LastHeartbeatTime:  node.CreationTimestamp,
				LastTransitionTime: nowMeta,
			})
			changed = true
		}
	}
	return changed
}

// ensureUnreachableTaint applies unreachableTaint to node via the real
// upstream taintutils.AddOrUpdateTaint (idempotent: a no-op if the taint is
// already present with an identical value), then persists node if either
// the taint or conditionsChanged (from markNodeUnknown) actually changed
// anything -- a single combined write, matching nodelifecycle.ts's
// changed-flag-gated upsertNode.
func ensureUnreachableTaint(ctx context.Context, nodeStore *ResourceStore, node *corev1.Node, conditionsChanged bool) (bool, error) {
	newNode, taintChanged, err := taintutils.AddOrUpdateTaint(node, unreachableTaint)
	if err != nil {
		return false, err
	}
	if !taintChanged && !conditionsChanged {
		return false, nil
	}
	// AddOrUpdateTaint deep-copies node in full (not just Spec.Taints), so
	// newNode already carries markNodeUnknown's Status edits (node was
	// mutated before this function was called).
	newNode.ResourceVersion = "" // unconditional update -- store.Update fetches the current revision itself
	if _, err := nodeStore.Update(ctx, "", node.Name, newNode); err != nil {
		return false, err
	}
	return true, nil
}

// evictPodsOnNode deletes every Pod bound to nodeName across every
// namespace, except ones that explicitly tolerate the unreachable taint
// forever (see podToleratesUnreachableForever) -- best-effort per-Pod,
// matching nodelifecycle.ts's evictPodsOnNode (which also never checked
// tolerations; this is a deliberate, small correctness addition, not
// present in the deleted TS, protecting e.g. DaemonSet-style pods that
// declare they tolerate node unavailability indefinitely).
func evictPodsOnNode(ctx context.Context, storage *Storage, nodeName string) error {
	podStore := podsResourceStore(storage)
	podListObj, err := podStore.List(ctx, "", "", "")
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}
	podList, ok := podListObj.(*corev1.PodList)
	if !ok {
		return fmt.Errorf("unexpected list type %T for pods", podListObj)
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Spec.NodeName != nodeName {
			continue
		}
		if podToleratesUnreachableForever(pod) {
			continue
		}
		if _, err := podStore.Delete(ctx, pod.Namespace, pod.Name); err != nil && !isNotFoundErr(err) {
			log.Printf("failed to evict pod %s/%s from unreachable node %s: %v", pod.Namespace, pod.Name, nodeName, err)
		}
	}
	return nil
}

// podToleratesUnreachableForever reports whether pod has an explicit
// toleration for unreachableTaint with no TolerationSeconds (tolerate
// indefinitely) -- the one piece of real taint-eviction-controller's
// toleration handling this simplified, immediate-eviction design respects.
// Uses Toleration.ToleratesTaint (k8s.io/api/core/v1, already a base
// dependency of this whole package) directly rather than hand-rolling
// key/effect matching. enableComparisonOperators=false: this project has no
// evidence of the alpha Lt/Gt toleration-operator feature being used
// anywhere, matching stable-only behavior.
func podToleratesUnreachableForever(pod *corev1.Pod) bool {
	logger := klog.Background()
	for i := range pod.Spec.Tolerations {
		t := &pod.Spec.Tolerations[i]
		if t.TolerationSeconds == nil && t.ToleratesTaint(logger, unreachableTaint, false) {
			return true
		}
	}
	return false
}

// RegisterInternalHandlers registers routes meant only for other Workers in
// this project to call via a Cloudflare service binding (not for kubectl or
// any other public client) -- mirrors RegisterSupervisorHandlers'
// registration style (supervisor.go) and its lack of AuthMiddleware: a
// service binding is only reachable by a Worker this account explicitly
// wired a "services" binding to (see workers/storage/wrangler.jsonc's
// APISERVER binding), which is the actual trust boundary here, the same way
// it already is for workers/storage's existing CONTROLLERS binding
// (index.ts's pingControllers).
//
// POST /internal/reconcile-node-lifecycle runs ReconcileNodeLifecycle once.
// Called from packages/k8flare-worker/src/storage/index.ts's Cluster DO alarm loop, since
// Lease staleness has no write to hook a synchronous call to (see
// ReconcileNodeLifecycle's doc comment).
func RegisterInternalHandlers(mux *http.ServeMux, storage *Storage) {
	mux.HandleFunc("POST /internal/reconcile-node-lifecycle", func(w http.ResponseWriter, r *http.Request) {
		if err := ReconcileNodeLifecycle(r.Context(), storage); err != nil {
			writeInternalError(w, fmt.Errorf("reconcile node lifecycle: %w", err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /internal/vkubeproxy-resolve?ip=&port=: the ClusterIP->(Pod
	// UID, container port) resolution half of nodes/podproxy.ts's
	// handleVKubeProxy (vkubeproxy.go).
	mux.HandleFunc("GET /internal/vkubeproxy-resolve", handleVKubeProxyResolve(storage))
}
