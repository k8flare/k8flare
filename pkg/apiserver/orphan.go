package apiserver

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// OrphanDependents strips ownerUID from every namespaced object's
// ownerReferences, leaving the objects themselves in place --
// `kubectl delete deployment --cascade=orphan`'s effect on its
// ReplicaSets. This is the one propagationPolicy this apiserver still
// handles synchronously, inline in the same DELETE request that removes
// the owner (see handler.go's call sites): real upstream Kubernetes
// gets this behavior from the deleted owner's own deletionTimestamp +
// an "orphan" finalizer that the real garbagecollector controller
// (pkg/controllers/gc, this apiserver's Background/foreground cascade
// mechanism as of 2026-07-09 -- see docs/general-purpose-k8s-plan.md's
// "OwnerReference GC" entry) observes and clears -- this apiserver has
// no finalizer/graceful-deletion lifecycle at all (an object is deleted
// outright, not marked for deletion first), so there is no hook for the
// async controller to react to. Reimplementing that whole lifecycle
// just to cover this one policy value was judged out of scope for the
// real-GC migration; unlike cascade delete (an unbounded, multi-level
// walk down the owner tree -- exactly what the real controller now
// does correctly and asynchronously), orphan only ever touches this
// owner's DIRECT dependents, a small, bounded, synchronous operation
// that doesn't need a background worker or cross-level traversal.
func OrphanDependents(ctx context.Context, namespacedStores []*ResourceStore, namespace string, ownerUID types.UID) error {
	if ownerUID == "" || namespace == "" {
		return nil
	}
	for _, rs := range namespacedStores {
		listObj, err := rs.List(ctx, namespace, "", "")
		if err != nil {
			return fmt.Errorf("list %s for owner %s orphan: %w", rs.resource, ownerUID, err)
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
			m.OwnerReferences = removeOwnerRef(m.OwnerReferences, ownerUID)
			if _, err := rs.Update(ctx, namespace, m.Name, item); err != nil {
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
