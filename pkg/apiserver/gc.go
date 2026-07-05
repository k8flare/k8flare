package apiserver

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// CascadeDeleteDependents is this apiserver's stand-in for upstream
// kube-controller-manager's garbagecollector controller: it makes
// `kubectl delete deployment` also remove the ReplicaSets/Pods it owns.
//
// A real garbagecollector needs a PartialObjectMetadata client (arbitrary
// GVR Get/List/Watch/Delete/Patch) and a RESTMapper, neither of which
// pkg/leanclient provides today -- building those is a new subsystem on the
// scale of a second client generator, not a small addition (see
// docs/general-purpose-k8s-plan.md's "ownerReferences GC" entry for the
// investigation this is based on). This function is a deliberately narrower
// substitute: it only handles namespace-scoped ownerReferences (this
// project's API surface has no cluster-scoped resource that's ever owned by
// another object), walks every namespaced ResourceStore synchronously
// within the same request that deletes the owner (no finalizer, no
// background worker -- same simplification namespacedelete.go's
// DeleteNamespaceDependents makes for namespace deletion), and does not
// consult a RESTMapper or discovery: it just scans the fixed, known set of
// namespacedStores this apiserver already has in memory.
//
// policy selects the behavior:
//   - metav1.DeletePropagationOrphan: strip the matching ownerReference from
//     each dependent, leaving the dependent itself in place.
//   - anything else (including "", nil-equivalent, Background, Foreground):
//     delete each dependent owner-first, then its dependents in later
//     sweep passes (so a Deployment's ReplicaSet is gone before that
//     ReplicaSet's Pods are -- see deleteDependents for why this order).
//
// Foreground and Background are not distinguished. Real Foreground delete
// blocks the owner's own removal (via a finalizer) until dependents are
// gone; here the owner is already deleted by the time this is called (see
// handler.go), so a client can never observe that difference.
func CascadeDeleteDependents(ctx context.Context, namespacedStores []*ResourceStore, namespace string, ownerUID types.UID, policy metav1.DeletionPropagation) error {
	if ownerUID == "" || namespace == "" {
		return nil
	}
	if policy == metav1.DeletePropagationOrphan {
		return orphanDependents(ctx, namespacedStores, namespace, ownerUID)
	}
	return deleteDependents(ctx, namespacedStores, namespace, ownerUID)
}

func orphanDependents(ctx context.Context, stores []*ResourceStore, namespace string, ownerUID types.UID) error {
	return forEachDependent(ctx, stores, namespace, ownerUID, func(rs *ResourceStore, item runtime.Object, m *metav1.ObjectMeta) error {
		m.OwnerReferences = removeOwnerRef(m.OwnerReferences, ownerUID)
		_, err := rs.Update(ctx, namespace, m.Name, item)
		return err
	})
}

func deleteDependents(ctx context.Context, stores []*ResourceStore, namespace string, ownerUID types.UID) error {
	// Top-down sweep with a quiescence loop, not children-first recursion.
	// The order matters against the live kube-controller-manager: an
	// earlier children-first version deleted a ReplicaSet's Pods while the
	// ReplicaSet still existed, so the real replicaset controller (still
	// holding the RS in its informer cache) raced replacement Pods into
	// existence mid-cascade -- and with no background garbagecollector to
	// reap dangling dependents later, those replacements survived as
	// permanent orphans (observed live 2026-07-05: DELETE of a Deployment
	// removed the Deployment and its ReplicaSet but left one
	// freshly-created Pod behind). Deleting each owner before touching its
	// children lets watching controllers observe the owner's deletion
	// first; the repeat-until-quiet passes catch anything still created in
	// the remaining, much smaller window.
	owners := map[types.UID]bool{ownerUID: true}
	const maxPasses = 5
	for pass := 0; pass < maxPasses; pass++ {
		deletedAny := false
		for uid := range owners {
			err := forEachDependent(ctx, stores, namespace, uid, func(rs *ResourceStore, _ runtime.Object, m *metav1.ObjectMeta) error {
				if _, err := rs.Delete(ctx, namespace, m.Name); err != nil {
					var se *StatusError
					if errors.As(err, &se) && se.Status.Reason == metav1.StatusReasonNotFound {
						// Already gone -- e.g. it had a second owner also
						// being deleted in this same sweep.
						return nil
					}
					return err
				}
				owners[m.UID] = true // its dependents are a later pass's work
				deletedAny = true
				return nil
			})
			if err != nil {
				return err
			}
		}
		if !deletedAny {
			return nil
		}
	}
	return nil
}

// forEachDependent lists every object of every store in stores within
// namespace, and calls fn for each whose ownerReferences names ownerUID.
func forEachDependent(ctx context.Context, stores []*ResourceStore, namespace string, ownerUID types.UID, fn func(rs *ResourceStore, item runtime.Object, m *metav1.ObjectMeta) error) error {
	for _, rs := range stores {
		listObj, err := rs.List(ctx, namespace, "", "")
		if err != nil {
			return fmt.Errorf("list %s for owner %s cascade: %w", rs.resource, ownerUID, err)
		}
		items, err := meta.ExtractList(listObj)
		if err != nil {
			return fmt.Errorf("extract %s list: %w", rs.resource, err)
		}
		for _, item := range items {
			m := getObjectMeta(item)
			if m == nil || !hasOwnerRef(m.OwnerReferences, ownerUID) {
				continue
			}
			if err := fn(rs, item, m); err != nil {
				return fmt.Errorf("%s %s/%s: %w", rs.resource, namespace, m.Name, err)
			}
		}
	}
	return nil
}

func hasOwnerRef(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

func removeOwnerRef(refs []metav1.OwnerReference, uid types.UID) []metav1.OwnerReference {
	out := refs[:0]
	for _, ref := range refs {
		if ref.UID != uid {
			out = append(out, ref)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
