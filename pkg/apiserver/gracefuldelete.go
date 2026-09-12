package apiserver

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
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

// markDeletionRetries bounds the per-object conflict-retry loops in
// this file (stripOwnerRef).
const markDeletionRetries = 5

// patchConflictRetries bounds the PATCH handler's re-read-and-reapply
// loop (handler.go) -- upstream's patch handler retries conflicts the
// same way (its maxRetryWhenPatchConflicts).
const patchConflictRetries = 5

// isStatusReason reports whether err is a StatusError carrying reason.
// isStatusReason matches both error shapes in play during/after the S25
// migration: this project's *StatusError and upstream's
// apierrors.StatusError (what genericregistry.Store returns).
func isStatusReason(err error, reason metav1.StatusReason) bool {
	var se *StatusError
	if errors.As(err, &se) && se.Status.Reason == reason {
		return true
	}
	return apierrors.ReasonForError(err) == reason
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

// blockingDependent returns "<resource>/<name>" for a namespaced object
// that still carries a BlockOwnerDeletion ownerReference to ownerUID, or
// "" when none remain. That is the same predicate the real
// garbagecollector's node.blockingDependents() applies, evaluated
// against storage instead of against its dependency graph.
//
// first, when non-nil, is looked at before the rest: a cascade's
// dependents are nearly always all of one kind, so starting there lets
// the common "still blocked" answer come back after one list instead of
// walking every namespaced resource.
func blockingDependent(ctx context.Context, namespacedStores []*ResourceStore, namespace, ownerUID string, first *ResourceStore) (string, error) {
	if ownerUID == "" || namespace == "" {
		return "", nil
	}
	ordered := namespacedStores
	if first != nil {
		ordered = append([]*ResourceStore{first}, namespacedStores...)
	}
	for _, rs := range ordered {
		listObj, err := rs.List(ctx, namespace, "", "")
		if err != nil {
			return "", fmt.Errorf("foreground guard: list %s: %w", rs.resource, err)
		}
		items, err := meta.ExtractList(listObj)
		if err != nil {
			return "", fmt.Errorf("foreground guard: extract %s list: %w", rs.resource, err)
		}
		for _, item := range items {
			m := getObjectMeta(item)
			if m == nil {
				continue
			}
			for _, ref := range m.OwnerReferences {
				if string(ref.UID) == ownerUID && ref.BlockOwnerDeletion != nil && *ref.BlockOwnerDeletion {
					return rs.resource + "/" + m.Name, nil
				}
			}
		}
	}
	return "", nil
}

// FinishUnblockedForegroundOwners completes the deletion of any owner
// that `deleted` was the last blocking dependent of, so that finishing a
// foreground cascade never depends on the real garbagecollector
// retrying the finalizer patch refuseForegroundFinalize above rejected.
// Why that retry is expensive enough to matter:
// docs/platform-verification.md S36.
func FinishUnblockedForegroundOwners(ctx context.Context, namespacedStores []*ResourceStore, deletedFrom *ResourceStore, namespace string, deleted runtime.Object) {
	if namespacedStores == nil || namespace == "" {
		return
	}
	m := getObjectMeta(deleted)
	if m == nil {
		return
	}
	for _, ref := range m.OwnerReferences {
		if ref.BlockOwnerDeletion == nil || !*ref.BlockOwnerDeletion {
			continue
		}
		ownerStore := storeForKind(namespacedStores, ref.Kind)
		if ownerStore == nil {
			continue
		}
		owner, err := ownerStore.Get(ctx, namespace, ref.Name)
		if err != nil {
			continue
		}
		om := getObjectMeta(owner)
		if om == nil || om.UID != ref.UID || om.DeletionTimestamp == nil ||
			!containsString(om.Finalizers, metav1.FinalizerDeleteDependents) {
			continue
		}
		blocker, err := blockingDependent(ctx, namespacedStores, namespace, string(om.UID), deletedFrom)
		if err != nil || blocker != "" {
			continue
		}
		clearForegroundFinalizer(ctx, ownerStore, namespace, ref.Name, string(ref.UID))
	}
}

func clearForegroundFinalizer(ctx context.Context, rs *ResourceStore, namespace, name, uid string) {
	for attempt := 0; attempt < markDeletionRetries; attempt++ {
		obj, err := rs.Get(ctx, namespace, name)
		if err != nil {
			return
		}
		m := getObjectMeta(obj)
		if m == nil || string(m.UID) != uid || m.DeletionTimestamp == nil ||
			!containsString(m.Finalizers, metav1.FinalizerDeleteDependents) {
			return
		}
		kept := make([]string, 0, len(m.Finalizers))
		for _, f := range m.Finalizers {
			if f != metav1.FinalizerDeleteDependents {
				kept = append(kept, f)
			}
		}
		if len(kept) == 0 {
			kept = nil
		}
		m.Finalizers = kept
		// Clearing the last finalizer completes the delete, and the
		// per-resource effects that follow (a Service's ClusterIP) run from
		// the store's own AfterDelete -- upstream routes this path through
		// deleteWithoutFinalizers, which fires the same hook.
		_, err = rs.Update(ctx, namespace, name, obj, nil)
		if isStatusReason(err, metav1.StatusReasonConflict) {
			continue
		}
		return
	}
}

// CountPendingGracefulDeletions counts objects the real garbagecollector
// still owes work on: a deletionTimestamp plus the "orphan" or
// "foregroundDeletion" finalizer that only it clears. The Controllers DO
// treats a non-zero count as unconverged work, because a cascade in
// flight is exactly the state its workload spec/status probe cannot see --
// docs/platform-verification.md S36.
func CountPendingGracefulDeletions(ctx context.Context, namespacedStores []*ResourceStore) (int, error) {
	pending := 0
	for _, rs := range namespacedStores {
		listObj, err := rs.List(ctx, "", "", "")
		if err != nil {
			return 0, fmt.Errorf("pending deletions: list %s: %w", rs.resource, err)
		}
		items, err := meta.ExtractList(listObj)
		if err != nil {
			return 0, fmt.Errorf("pending deletions: extract %s list: %w", rs.resource, err)
		}
		for _, item := range items {
			m := getObjectMeta(item)
			if m == nil || m.DeletionTimestamp == nil {
				continue
			}
			if containsString(m.Finalizers, metav1.FinalizerDeleteDependents) ||
				containsString(m.Finalizers, metav1.FinalizerOrphanDependents) {
				pending++
			}
		}
	}
	return pending, nil
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
		_, err = rs.Update(ctx, namespace, name, obj, nil)
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

// SweepOrphansOnFinalize is finalizeDeleteWithOrphanSweep evaluated against
// the update that clears the finalizer rather than against a re-read. The
// object being orphaned still carries the orphan finalizer in `old`; by the
// time the delete has completed it does not, so the condition has to be read
// here and the sweep deferred until the write succeeds.
//
// Wired into the store's BeginUpdate. It used to run inside the hand-written
// DELETE handler, and when routing moved to the real installer it was the one
// graceful-deletion guard of four that was left behind (found by an audit,
// 2026-09-13).
func SweepOrphansOnFinalize(rs *ResourceStore, namespacedStores []*ResourceStore, old, next runtime.Object) func(context.Context) {
	if namespacedStores == nil || rs == nil || !rs.namespaced {
		return nil
	}
	oldMeta, nextMeta := getObjectMeta(old), getObjectMeta(next)
	if oldMeta == nil || nextMeta == nil {
		return nil
	}
	if oldMeta.DeletionTimestamp == nil || !containsString(oldMeta.Finalizers, metav1.FinalizerOrphanDependents) {
		return nil
	}
	if containsString(nextMeta.Finalizers, metav1.FinalizerOrphanDependents) {
		return nil
	}
	namespace, ownerUID := oldMeta.Namespace, string(oldMeta.UID)
	return func(ctx context.Context) {
		if err := sweepOrphanStragglers(ctx, namespacedStores, namespace, ownerUID); err != nil {
			klog.ErrorS(err, "sweeping orphaned dependents", "namespace", namespace, "owner", ownerUID)
		}
	}
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
		if isStatusReason(err, metav1.StatusReasonNotFound) {
			// Absent owner: also rejected. Run 29140842888 showed why
			// terminating-only isn't enough -- under [Serial] load the
			// KCM's owner informer lagged its pod informer by ~30s, so
			// it kept back-filling for an owner that was ALREADY fully
			// deleted; those creates carried a dangling controller ref
			// from birth and raced the GC's cascade against the
			// conformance count. No legitimate controller creates
			// dependents for an owner it hasn't observed alive, and a
			// human doing it manually is constructing instant GC food;
			// upstream tolerates it only because its informers never
			// lag enough to matter.
			return fmt.Sprintf("cannot create %s: controller owner %s %q does not exist", m.Name, ref.Kind, ref.Name)
		}
		if err != nil {
			continue // transient read error: allow rather than block writes
		}
		om := getObjectMeta(owner)
		if om != nil && om.UID == ref.UID && om.DeletionTimestamp != nil {
			return fmt.Sprintf("cannot create %s: controller owner %s %q is being deleted", m.Name, ref.Kind, ref.Name)
		}
		if om != nil && om.UID != ref.UID {
			// Same name, different UID: the referenced incarnation is
			// gone (deleted and recreated) -- same dangling-from-birth
			// situation as NotFound above.
			return fmt.Sprintf("cannot create %s: controller owner %s %q (uid %s) no longer exists", m.Name, ref.Kind, ref.Name, ref.UID)
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

// RefuseForegroundFinalizeOn is refuseForegroundFinalize evaluated against an
// update already in flight rather than against a re-read: the store's
// BeginUpdate hook hands over both objects, so the check costs no extra read
// and runs for every path that can clear the finalizer, not only the one
// hand-written handler it used to sit in.
func RefuseForegroundFinalizeOn(ctx context.Context, rs *ResourceStore, namespacedStores []*ResourceStore, old, next runtime.Object) error {
	if namespacedStores == nil || rs == nil || !rs.namespaced {
		return nil
	}
	oldMeta, nextMeta := getObjectMeta(old), getObjectMeta(next)
	if oldMeta == nil || nextMeta == nil {
		return nil
	}
	if oldMeta.DeletionTimestamp == nil || !containsString(oldMeta.Finalizers, metav1.FinalizerDeleteDependents) {
		return nil
	}
	if containsString(nextMeta.Finalizers, metav1.FinalizerDeleteDependents) {
		return nil
	}
	blocker, err := blockingDependent(ctx, namespacedStores, oldMeta.Namespace, string(oldMeta.UID), nil)
	if err != nil {
		return err
	}
	if blocker == "" {
		return nil
	}
	return apierrors.NewConflict(schema.GroupResource{Resource: rs.resource}, oldMeta.Name,
		fmt.Errorf("foreground deletion is still waiting on dependent %s", blocker))
}
