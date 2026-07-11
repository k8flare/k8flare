package apiserver

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Minimal graceful-deletion lifecycle, exactly deep enough for the real
// garbagecollector's Orphan and Foreground propagation to work against
// this apiserver:
//
//   - DELETE with propagationPolicy Orphan/Foreground does NOT remove
//     the object. It stamps metadata.deletionTimestamp and the policy's
//     finalizer ("orphan" / "foregroundDeletion") -- markForDeletion
//     below -- and returns the still-visible, now-terminating object,
//     same as upstream.
//   - The real garbagecollector (pkg/controllers/gc) observes the
//     finalizer through its normal watches: attemptToOrphan strips the
//     owner's UID from every dependent, attemptToDelete handles the
//     foreground cascade, and both finish by patching the finalizer off
//     the owner.
//   - Any write (PUT/PATCH) that leaves an object with a non-nil
//     deletionTimestamp and zero finalizers completes the deletion
//     instead of persisting -- shouldFinalizeDelete below, checked at
//     both handler write paths. That patch from the GC is what actually
//     removes the owner.
//
// This replaced the earlier synchronous OrphanDependents sweep (see git
// history for pkg/apiserver/orphan.go): stripping dependents inline in
// the DELETE request raced the live controllers in one direction or the
// other no matter how it was ordered -- delete-owner-first let the GC's
// dangling-reference cascade eat the dependents being orphaned
// (run 29118462901), strip-first let the owner's still-running
// controller re-adopt and back-fill them (run 29134357997, "expect 50
// pods, got 53"). Upstream's answer to that whole race family is this
// lifecycle, and the real GC already implements its half; reimplementing
// less and reusing the real thing is this repo's rule 3.
//
// Background deletes are unchanged: the object is removed outright and
// the real GC cascades dependents afterwards, which matches upstream's
// observable behavior (the owner disappears immediately).

// markDeletionRetries bounds markForDeletion's conflict-retry loop.
const markDeletionRetries = 5

// finalizerForPolicy maps a propagation policy to the finalizer the real
// garbagecollector acts on. Returns "" for policies that need no
// finalizer (Background).
func finalizerForPolicy(policy metav1.DeletionPropagation) string {
	switch policy {
	case metav1.DeletePropagationOrphan:
		return metav1.FinalizerOrphanDependents
	case metav1.DeletePropagationForeground:
		return metav1.FinalizerDeleteDependents
	default:
		return ""
	}
}

// markForDeletion stamps deletionTimestamp and finalizer on the named
// object and returns the updated (terminating) object. Idempotent: an
// object already carrying both is returned as-is, so repeating a DELETE
// keeps returning 200 with the terminating object, same as upstream.
// Conflicts (a controller writing the object between the read and the
// update) are retried against a fresh read.
func markForDeletion(ctx context.Context, rs *ResourceStore, namespace, name, finalizer string) (runtime.Object, error) {
	var lastErr error
	for attempt := 0; attempt < markDeletionRetries; attempt++ {
		obj, err := rs.Get(ctx, namespace, name)
		if err != nil {
			return nil, err
		}
		m := getObjectMeta(obj)
		if m == nil {
			return nil, fmt.Errorf("mark %s %s/%s deleting: object has no metadata", rs.resource, namespace, name)
		}
		changed := false
		if m.DeletionTimestamp == nil {
			now := metav1.Now()
			m.DeletionTimestamp = &now
			changed = true
		}
		if !containsString(m.Finalizers, finalizer) {
			m.Finalizers = append(m.Finalizers, finalizer)
			changed = true
		}
		if !changed {
			return obj, nil
		}
		updated, err := rs.Update(ctx, namespace, name, obj)
		if err == nil {
			return updated, nil
		}
		if isStatusReason(err, metav1.StatusReasonConflict) {
			lastErr = err
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("mark %s %s/%s deleting: conflicted %d times: %w", rs.resource, namespace, name, markDeletionRetries, lastErr)
}

// shouldFinalizeDelete reports whether writing obj would leave a
// terminating object with no finalizers left -- the state upstream
// defines as "deletion is now allowed to complete". The write paths
// delete the object instead of persisting that state.
func shouldFinalizeDelete(obj runtime.Object) bool {
	m := getObjectMeta(obj)
	return m != nil && m.DeletionTimestamp != nil && len(m.Finalizers) == 0
}

// isStatusReason reports whether err is a StatusError carrying reason.
func isStatusReason(err error, reason metav1.StatusReason) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status.Reason == reason
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
