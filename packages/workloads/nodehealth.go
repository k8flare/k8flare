package workloads

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
)

const (
	unknownReason  = "NodeStatusUnknown"
	unknownMessage = "Kubelet stopped posting node status."
	evictionRetry  = 30 * time.Second
	leaseExtra     = 20 * time.Second
)

var unreachableConditions = []v1.NodeConditionType{v1.NodeReady, v1.NodeMemoryPressure, v1.NodeDiskPressure, v1.NodePIDPressure}

type NodeHealthResult struct {
	Evicted int   `json:"evicted"`
	Waiting int   `json:"waiting"`
	NextMs  int64 `json:"nextMs"`
}

func NodeHealth(ctx context.Context, client kubernetes.Interface, name string) (*NodeHealthResult, error) {
	nodes := client.CoreV1().Nodes()
	node, err := nodes.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return &NodeHealthResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	node = node.DeepCopy()
	held, known, expiry := nodeLeaseState(ctx, client, name)
	if held {
		if readyStatus(node) == v1.ConditionFalse {
			result, err := taintAndEvict(ctx, client, node, v1.TaintNodeNotReady, v1.TaintNodeUnreachable, false)
			if err != nil {
				return nil, err
			}
			result.NextMs = soonest(result.NextMs, time.Duration(untilExpiry(expiry))*time.Millisecond)
			return result, nil
		}
		if removeUnreachableTaints(node) {
			if node, err = nodes.Update(ctx, node, metav1.UpdateOptions{}); err != nil {
				return nil, err
			}
		}
		if restoreReady(node) {
			if _, err := nodes.UpdateStatus(ctx, node, metav1.UpdateOptions{}); err != nil {
				return nil, err
			}
		}
		return &NodeHealthResult{NextMs: untilExpiry(expiry)}, nil
	}
	if !known {
		return &NodeHealthResult{}, nil
	}
	now := metav1.Now()
	if markUnknown(node, now) {
		if node, err = nodes.UpdateStatus(ctx, node, metav1.UpdateOptions{}); err != nil {
			return nil, err
		}
	}
	return taintAndEvict(ctx, client, node, v1.TaintNodeUnreachable, v1.TaintNodeNotReady, true)
}

func taintAndEvict(ctx context.Context, client kubernetes.Interface, node *v1.Node, key, oppositeKey string, markPodsNotReady bool) (*NodeHealthResult, error) {
	now := metav1.Now()
	taint := v1.Taint{Key: key, Effect: v1.TaintEffectNoExecute, TimeAdded: &now}
	if swapTaints(node, key, oppositeKey, now) {
		updated, err := client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		node = updated
	}
	for _, t := range node.Spec.Taints {
		if t.Key == key && t.Effect == v1.TaintEffectNoExecute && t.TimeAdded != nil {
			taint.TimeAdded = t.TimeAdded
		}
	}
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + node.Name, Limit: listPage})
	if err != nil {
		return nil, err
	}
	result := &NodeHealthResult{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if markPodsNotReady {
			if err := markPodNotReady(ctx, client, pod); err != nil {
				return nil, err
			}
		}
		if pod.DeletionTimestamp != nil || ownedByDaemonSet(pod) {
			continue
		}
		wait, tolerated := tolerationWait(klog.FromContext(ctx), pod, &taint)
		if tolerated {
			left := time.Until(taint.TimeAdded.Add(wait))
			if left > 0 {
				result.Waiting++
				result.NextMs = soonest(result.NextMs, left)
				continue
			}
		}
		if err := client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		result.Evicted++
	}
	if result.Evicted > 0 && result.NextMs == 0 {
		result.NextMs = evictionRetry.Milliseconds()
	}
	return result, nil
}

func markPodNotReady(ctx context.Context, client kubernetes.Interface, pod *v1.Pod) error {
	pod = pod.DeepCopy()
	for _, cond := range pod.Status.Conditions {
		if cond.Type != v1.PodReady {
			continue
		}
		cond.Status = v1.ConditionFalse
		if !podutil.UpdatePodCondition(&pod.Status, &cond) {
			return nil
		}
		_, err := client.CoreV1().Pods(pod.Namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func readyStatus(node *v1.Node) v1.ConditionStatus {
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady {
			return c.Status
		}
	}
	return ""
}

func nodeLeaseState(ctx context.Context, client kubernetes.Interface, name string) (held, known bool, expiry time.Time) {
	lease, err := client.CoordinationV1().Leases(v1.NamespaceNodeLease).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, false, time.Time{}
	}
	if lease.Spec.RenewTime == nil {
		return false, true, time.Time{}
	}
	dur := 40 * time.Second
	if lease.Spec.LeaseDurationSeconds != nil && *lease.Spec.LeaseDurationSeconds > 0 {
		dur = time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	}
	expiry = lease.Spec.RenewTime.Add(dur + leaseExtra)
	return time.Now().Before(expiry), true, expiry
}

// untilExpiry books the next lease check on the deadline the current lease
// already implies, so a node that stops renewing is still visited once.
func untilExpiry(expiry time.Time) int64 {
	if expiry.IsZero() {
		return 0
	}
	left := time.Until(expiry)
	if left < time.Second {
		left = time.Second
	}
	return left.Milliseconds()
}

func markUnknown(node *v1.Node, now metav1.Time) bool {
	changed := false
	for _, want := range unreachableConditions {
		found := false
		for i := range node.Status.Conditions {
			c := &node.Status.Conditions[i]
			if c.Type != want {
				continue
			}
			found = true
			if c.Status != v1.ConditionUnknown {
				c.Status = v1.ConditionUnknown
				c.Reason = unknownReason
				c.Message = unknownMessage
				c.LastTransitionTime = now
				c.LastHeartbeatTime = now
				changed = true
			}
		}
		if !found {
			node.Status.Conditions = append(node.Status.Conditions, v1.NodeCondition{
				Type: want, Status: v1.ConditionUnknown, Reason: unknownReason, Message: unknownMessage,
				LastTransitionTime: now, LastHeartbeatTime: now,
			})
			changed = true
		}
	}
	return changed
}

func swapTaints(node *v1.Node, key, oppositeKey string, now metav1.Time) bool {
	changed := false
	kept := node.Spec.Taints[:0]
	for _, t := range node.Spec.Taints {
		if t.Key == oppositeKey {
			changed = true
			continue
		}
		kept = append(kept, t)
	}
	node.Spec.Taints = kept
	for _, effect := range []v1.TaintEffect{v1.TaintEffectNoSchedule, v1.TaintEffectNoExecute} {
		present := false
		for _, t := range node.Spec.Taints {
			if t.Key == key && t.Effect == effect {
				present = true
			}
		}
		if !present {
			node.Spec.Taints = append(node.Spec.Taints, v1.Taint{Key: key, Effect: effect, TimeAdded: &now})
			changed = true
		}
	}
	return changed
}

func restoreReady(node *v1.Node) bool {
	changed := false
	now := metav1.Now()
	for i := range node.Status.Conditions {
		c := &node.Status.Conditions[i]
		if c.Type != v1.NodeReady || c.Status != v1.ConditionUnknown || c.Reason != unknownReason {
			continue
		}
		c.Status = v1.ConditionTrue
		c.Reason = "KubeletReady"
		c.Message = "kubelet is posting ready status"
		c.LastTransitionTime = now
		c.LastHeartbeatTime = now
		changed = true
	}
	return changed
}

func removeUnreachableTaints(node *v1.Node) bool {
	kept := node.Spec.Taints[:0]
	changed := false
	for _, t := range node.Spec.Taints {
		if t.Key == v1.TaintNodeUnreachable || t.Key == v1.TaintNodeNotReady {
			changed = true
			continue
		}
		kept = append(kept, t)
	}
	node.Spec.Taints = kept
	return changed
}

func tolerationWait(logger klog.Logger, pod *v1.Pod, taint *v1.Taint) (time.Duration, bool) {
	for i := range pod.Spec.Tolerations {
		toleration := &pod.Spec.Tolerations[i]
		if !toleration.ToleratesTaint(logger, taint, false) {
			continue
		}
		if toleration.TolerationSeconds == nil {
			return 0, false
		}
		return time.Duration(*toleration.TolerationSeconds) * time.Second, true
	}
	return 0, false
}

func ownedByDaemonSet(pod *v1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}
