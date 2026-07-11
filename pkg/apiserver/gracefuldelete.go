package apiserver

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
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

// patchConflictRetries bounds the PATCH handler's re-read-and-reapply
// loop (handler.go) -- upstream's patch handler retries conflicts the
// same way (its maxRetryWhenPatchConflicts).
const patchConflictRetries = 5

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

// sweepOrphanStragglers strips ownerUID from any namespaced dependent
// still carrying it, tolerating NotFound (a gone dependent is orphaned
// enough) and retrying conflicts per object. It runs at
// finalize-delete time for owners whose "orphan" finalizer was just
// cleared: the real GC's attemptToOrphan strips the dependents ITS
// GRAPH knows about, but its per-GVR watches have no cross-stream
// ordering guarantee -- on a slow runner the pod informer can lag the
// owner's finalizer event, so the GC orphans a subset, clears the
// finalizer, and the not-yet-observed dependents are later live-read as
// dangling and cascade-deleted (run 29138332717: "expect 50 pods, got
// 1" -- exactly one pod, the one the graph knew, survived). Sweeping
// here is race-free where the old synchronous OrphanDependents wasn't:
// the owner has carried deletionTimestamp for the whole lifecycle, so
// its controller is stood down (no re-adopt/back-fill), and the owner
// is removed in this same request AFTER the sweep, so the GC's
// dangling-reference live checks only ever see already-stripped
// dependents.
func sweepOrphanStragglers(ctx context.Context, namespacedStores []*ResourceStore, namespace string, ownerUID string) error {
	if ownerUID == "" || namespace == "" {
		return nil
	}
	for _, rs := range namespacedStores {
		listObj, err := rs.List(ctx, namespace, "", "")
		if err != nil {
			return fmt.Errorf("orphan sweep: list %s: %w", rs.resource, err)
		}
		items, err := meta.ExtractList(listObj)
		if err != nil {
			return fmt.Errorf("orphan sweep: extract %s list: %w", rs.resource, err)
		}
		for _, item := range items {
			m := getObjectMeta(item)
			if m == nil || !hasOwnerUID(m.OwnerReferences, ownerUID) {
				continue
			}
			if err := stripOwnerRef(ctx, rs, namespace, m.Name, ownerUID); err != nil {
				return fmt.Errorf("orphan sweep: %s %s/%s: %w", rs.resource, namespace, m.Name, err)
			}
		}
	}
	return nil
}

// stripOwnerRef removes ownerUID from one named object's
// ownerReferences, re-reading fresh per attempt; NotFound at any point
// is success and conflicts retry.
func stripOwnerRef(ctx context.Context, rs *ResourceStore, namespace, name, ownerUID string) error {
	var lastErr error
	for attempt := 0; attempt < markDeletionRetries; attempt++ {
		obj, err := rs.Get(ctx, namespace, name)
		if isStatusReason(err, metav1.StatusReasonNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		m := getObjectMeta(obj)
		if m == nil || !hasOwnerUID(m.OwnerReferences, ownerUID) {
			return nil
		}
		kept := m.OwnerReferences[:0]
		for _, ref := range m.OwnerReferences {
			if string(ref.UID) != ownerUID {
				kept = append(kept, ref)
			}
		}
		if len(kept) == 0 {
			kept = nil
		}
		m.OwnerReferences = kept
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
	return fmt.Errorf("strip conflicted %d times: %w", markDeletionRetries, lastErr)
}

func hasOwnerUID(refs []metav1.OwnerReference, uid string) bool {
	for _, ref := range refs {
		if string(ref.UID) == uid {
			return true
		}
	}
	return false
}

// finalizeDeleteWithOrphanSweep runs just before the handler completes
// a finalize-delete: if the CURRENT stored object (the one whose last
// finalizer is being cleared) was terminating with the "orphan"
// finalizer, sweep any dependents the GC's graph missed (see
// sweepOrphanStragglers). A no-op for foreground/plain finalizer
// clears and for non-namespaced resources.
func finalizeDeleteWithOrphanSweep(ctx context.Context, rs *ResourceStore, namespacedStores []*ResourceStore, namespace, name string) error {
	if namespacedStores == nil || !rs.namespaced {
		return nil
	}
	cur, err := rs.Get(ctx, namespace, name)
	if err != nil {
		return nil // vanished already; the Delete below reports it properly
	}
	m := getObjectMeta(cur)
	if m == nil || m.DeletionTimestamp == nil || !containsString(m.Finalizers, metav1.FinalizerOrphanDependents) {
		return nil
	}
	return sweepOrphanStragglers(ctx, namespacedStores, namespace, string(m.UID))
}


// RejectCreateWithTerminatingController blocks creating a namespaced
// object whose controller ownerReference points at an owner that is
// currently terminating (deletionTimestamp set). Upstream has no such
// admission check -- it doesn't need one, because its controllers
// observe an owner's deletionTimestamp within milliseconds and stand
// down before back-filling. Here the KCM is a resident dynamic worker
// whose per-resource watch pumps have no cross-stream ordering
// guarantee, so a controller can observe its dependents being orphaned
// (and "missing") seconds before it observes the owner's own
// deletionTimestamp, and back-fill replacements mid-orphan -- run
// 29140071160: "expect 50 pods, got 54", four back-fills created in
// that window, then orphaned along with the originals. Rejecting the
// create is behaviorally equivalent to what upstream's timing produces
// (no such pod ever exists); the controller treats the 403 like any
// create failure and stops for real once its informer catches up.
// Returns "" if the create is allowed, else the rejection message.
func RejectCreateWithTerminatingController(ctx context.Context, namespacedStores []*ResourceStore, namespace string, obj runtime.Object) string {
	if namespacedStores == nil || namespace == "" {
		return ""
	}
	m := getObjectMeta(obj)
	if m == nil {
		return ""
	}
	for _, ref := range m.OwnerReferences {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		ownerStore := storeForKind(namespacedStores, ref.Kind)
		if ownerStore == nil {
			continue // cluster-scoped or unserved owner kind: allow
		}
		owner, err := ownerStore.Get(ctx, namespace, ref.Name)
		if err != nil {
			continue // absent owner: allow; the GC cascades danglings
		}
		om := getObjectMeta(owner)
		if om != nil && om.UID == ref.UID && om.DeletionTimestamp != nil {
			return fmt.Sprintf("cannot create %s: controller owner %s %q is being deleted", m.Name, ref.Kind, ref.Name)
		}
	}
	return ""
}

// storeForKind resolves a namespaced ResourceStore by its object Kind
// (ownerReferences carry Kind, not the plural resource). The kind is
// derived from the store's own newFunc's Go type name -- k8s API type
// names ARE their Kinds.
func storeForKind(namespacedStores []*ResourceStore, kind string) *ResourceStore {
	for _, rs := range namespacedStores {
		if t := reflect.TypeOf(rs.newFunc()); t != nil && t.Kind() == reflect.Ptr && t.Elem().Name() == kind {
			return rs
		}
	}
	return nil
}