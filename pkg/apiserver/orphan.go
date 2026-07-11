package apiserver

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// orphanUpdateRetries bounds orphanOne's conflict-retry loop. Conflicts
// here are momentary (a controller touching the same dependent between
// the re-Get and the Update), so a handful of immediate retries is
// enough; anything still conflicting after that is a real problem worth
// surfacing.
const orphanUpdateRetries = 5

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
//
// Because it runs against a LIVE cluster (the real RC/RS controllers
// churn the very dependents being orphaned), a dependent listed here
// can be gone, or already modified, by the time its update lands.
// Upstream's async orphaning tolerates exactly this (NotFound is
// success, Conflict is retried); before orphanOne below did the same,
// the [sig-api-machinery] "should orphan pods created by rc" e2e failed
// the owner's whole DELETE with `pods "simpletest.rc-f2gbs" not found`
// (run 29118462901, 2026-07-11) when one of ~100 pods vanished between
// the List and its Update.
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
			if err := orphanOne(ctx, rs, namespace, m.Name, ownerUID); err != nil {
				return fmt.Errorf("%s %s/%s: %w", rs.resource, namespace, m.Name, err)
			}
		}
	}
	return nil
}

// orphanOne strips ownerUID from a single named object, re-reading it
// fresh each attempt (the listed copy's resourceVersion is already
// stale the moment a controller touches the object). A dependent that
// disappeared is already "orphaned" as far as the deleted owner is
// concerned -- NotFound at any point is success, matching upstream's
// GraphBuilder orphaning semantics.
func orphanOne(ctx context.Context, rs *ResourceStore, namespace, name string, ownerUID types.UID) error {
	var lastErr error
	for attempt := 0; attempt < orphanUpdateRetries; attempt++ {
		obj, err := rs.Get(ctx, namespace, name)
		if isStatusReason(err, metav1.StatusReasonNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		m := getObjectMeta(obj)
		if m == nil || !hasOwnerRef(m.OwnerReferences, ownerUID) {
			return nil // already orphaned (or never owned) by the time we re-read it
		}
		m.OwnerReferences = removeOwnerRef(m.OwnerReferences, ownerUID)
		_, err = rs.Update(ctx, namespace, name, obj)
		switch {
		case err == nil:
			return nil
		case isStatusReason(err, metav1.StatusReasonNotFound):
			return nil
		case isStatusReason(err, metav1.StatusReasonConflict):
			lastErr = err
			continue
		default:
			return err
		}
	}
	return fmt.Errorf("orphan update conflicted %d times: %w", orphanUpdateRetries, lastErr)
}

// isStatusReason reports whether err is a StatusError carrying reason.
func isStatusReason(err error, reason metav1.StatusReason) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status.Reason == reason
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
