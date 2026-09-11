package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/util/dryrun"
)

// parseDeleteOptions decodes the caller's whole metav1.DeleteOptions from
// a DELETE request. Real clients (client-go's Delete/DeleteCollection)
// send them as a JSON- or protobuf-encoded request body; the query
// parameter form (?propagationPolicy=Foreground) some direct callers use
// for the policy is also accepted, matching upstream kube-apiserver's dual
// acceptance. Restores r.Body after reading it so later code in the same
// request (there is none today, but this must not be a trap for a future
// caller) still sees it.
//
// PropagationPolicy stays nil when the caller sent none: deletePropagation
// below defaults the handler's own branching to Background, but the
// upstream store reads an absent policy off the object's existing
// finalizers, and substituting Background there would strip a
// foreground/orphan finalizer the caller never mentioned.
func parseDeleteOptions(r *http.Request) (*metav1.DeleteOptions, error) {
	opts := &metav1.DeleteOptions{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) > 0 {
		// client-go's default negotiated content type is protobuf, not JSON --
		// same reason decodeBody below needs Codecs.UniversalDeserializer rather
		// than plain encoding/json (found by running this against a real
		// client-go client, not assumed: a bare json.Unmarshal failed to decode
		// with "invalid character 'k' looking for beginning of value", the tell
		// for feeding protobuf bytes to a JSON decoder).
		obj, decodeErr := decodeBody(body, nil)
		switch {
		case decodeErr == nil:
			typed, ok := obj.(*metav1.DeleteOptions)
			if !ok {
				return nil, fmt.Errorf("decode delete options: unexpected type %T", obj)
			}
			opts = typed
		default:
			// Real kubectl sends its DeleteOptions body WITHOUT TypeMeta
			// (`{"propagationPolicy":"Background"}`), which the universal
			// deserializer rejects with "Object 'Kind' is missing" -- the
			// upstream apiserver decodes options leniently for exactly this
			// reason. Fall back to a plain JSON unmarshal before failing;
			// found live when `kubectl delete deployment` 400'd against
			// production while curl (with TypeMeta) worked.
			if jsonErr := json.Unmarshal(body, opts); jsonErr != nil {
				return nil, fmt.Errorf("decode delete options: %w", decodeErr)
			}
		}
	}
	if q := r.URL.Query().Get("propagationPolicy"); q != "" {
		policy := metav1.DeletionPropagation(q)
		opts.PropagationPolicy = &policy
	}
	return opts, nil
}

// deletePropagation is the policy the DELETE handler branches on. An
// absent policy is Background, matching this project's registered
// resources' upstream default (none opt into Orphan-by-default).
func deletePropagation(opts *metav1.DeleteOptions) metav1.DeletionPropagation {
	if opts.PropagationPolicy == nil {
		return metav1.DeletePropagationBackground
	}
	return *opts.PropagationPolicy
}

// decodeBody decodes the request body as a Kubernetes runtime.Object.
// Uses UniversalDeserializer which auto-detects JSON and protobuf formats.
// defaults, when non-nil, supplies the apiVersion/kind for a body that
// carries none (see ResourceStore.decodeDefaults).
func decodeBody(body []byte, defaults *schema.GroupVersionKind) (runtime.Object, error) {
	obj, _, err := Codecs.UniversalDeserializer().Decode(body, defaults, nil)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return obj, nil
}

// HandleResource parses a Kubernetes API URL under prefix and dispatches to
// the appropriate ResourceStore method based on HTTP method and path
// segments. prefix is "/api/v1/" for the legacy core group, or
// "/apis/{group}/{version}/" for a named API group -- main.go registers one
// mux route per prefix and passes the matching stores map for each, so
// prefix and stores always agree on which GroupVersion is being served.
//
// namespacedStores, if non-nil, is used two ways by the http.MethodDelete
// case below (see NamespacedResourceStores): swept when a Namespace object
// itself is deleted ("namespaces" never exists as a key in any other
// group's stores map, so that branch is naturally unreachable for group-API
// calls even though they pass the same namespacedStores), and walked by
// orphan.go's OrphanDependents whenever any namespaced object is deleted
// with propagationPolicy=Orphan, regardless of group -- deleting an apps/v1
// Deployment must reach ReplicaSets (apps/v1) and Pods (core/v1) alike, so
// every group's HandleResource call passes the same full, cross-group
// slice. Background/Foreground cascade delete is no longer this
// apiserver's job at all -- the real pkg/controllers/gc garbagecollector
// controller handles it asynchronously.
//
// priorityClassStore, if non-nil, is the scheduling.k8s.io/v1 priorityclasses
// store -- Pod is core/v1-only, but the store it needs to resolve
// spec.priorityClassName against lives in a different GroupVersion's store
// map, so it's threaded through the same way namespacedStores is (every
// group's HandleResource call passes the same store; see priority.go).
//
// This single function replaces what used to be two near-identical
// functions, HandleAPI and HandleGroupAPI: same CRUD switch, same path
// grammar, differing only in how the group+version prefix got stripped
// before parsing and in whether a watch query parameter was checked at all
// (HandleGroupAPI's watch requests fell through to a duplicated list
// pathway -- a real drift merging them fixes, not just a line-count cut).
func HandleResource(w http.ResponseWriter, r *http.Request, prefix string, stores map[string]*ResourceStore, namespacedStores []*ResourceStore, priorityClassStore *ResourceStore, namespaceStore *ResourceStore) {
	trimmed := strings.TrimPrefix(r.URL.Path, prefix)

	if r.URL.Query().Get("watch") == "true" {
		resource, namespace, _, _, _ := parseResourcePath(trimmed)
		HandleWatch(w, r, resource, namespace)
		return
	}

	resource, namespace, name, subresource, ok := parseResourcePath(trimmed)
	if !ok {
		writeStatusError(w, http.StatusNotFound, "NotFound", "the path is not valid")
		return
	}

	if subresource != "" {
		HandleSubresource(w, r, stores, resource, namespace, name, subresource)
		return
	}

	store, exists := stores[resource]
	if !exists {
		writeStatusError(w, http.StatusNotFound, "NotFound", "the server doesn't have a resource type \""+resource+"\"")
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if name == "" {
			fieldSelector := r.URL.Query().Get("fieldSelector")
			labelSelector := r.URL.Query().Get("labelSelector")
			obj, err := store.List(ctx, namespace, fieldSelector, labelSelector)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			writeGetResponse(w, r, obj)
		} else {
			obj, err := store.Get(ctx, namespace, name)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			writeGetResponse(w, r, obj)
		}

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		fieldValidation, err := parseFieldValidation(r)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", err.Error())
			return
		}

		rObj, warnings, err := decodeBodyWithFieldValidation(body, fieldValidation, store.decodeDefaults())
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}
		writeFieldValidationWarnings(w, warnings)

		// Namespace-lifecycle admission (upstream's NamespaceLifecycle
		// plugin): creating a namespaced object requires its namespace to
		// exist. Without this, anything could be created into a deleted
		// namespace -- observed live 2026-07-25 as KCM resurrecting Events
		// into a namespace right after `kubectl delete ns` had swept it,
		// leaving permanent orphans (`kubectl get events -A` showed rows
		// for a 404 namespace). The check-then-create race window remains,
		// same as upstream's admission plugin; upstream closes it with the
		// namespace controller's re-sweep, which this project doesn't need
		// at its scale. The fetched Namespace is kept: the Pod
		// compute-class routing below reads its labels, so this is the
		// one namespace read on the create path.
		var nsObj runtime.Object
		if namespace != "" && resource != "namespaces" && namespaceStore != nil {
			obj, err := namespaceStore.Get(ctx, "", namespace)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					writeStatusError(w, http.StatusNotFound, "NotFound", "namespaces \""+namespace+"\" not found")
				} else {
					writeResourceError(w, err, "namespaces", namespace)
				}
				return
			}
			nsObj = obj
		}

		ApplyDefaults(rObj)

		// Back-fill guard for the graceful-deletion lifecycle: refuse to
		// create an object whose controller owner is already terminating
		// (see RejectCreateWithTerminatingController's doc comment for
		// why this closes a watch-ordering window upstream doesn't have).
		if msg := RejectCreateWithTerminatingController(ctx, namespacedStores, namespace, rObj); msg != "" {
			writeStatusError(w, http.StatusForbidden, "Forbidden", msg)
			return
		}

		// Fill in any container resource requests/limits the pod itself
		// didn't specify, then reject it if it still violates a
		// Container-scoped LimitRange's Min/Max, from LimitRanges in its
		// namespace. stores["limitranges"] is absent from every group-API
		// store map (leases/storage/nodeAPI/resourceAPI/apps/policy/discovery/
		// networking/batch), so this is a no-op there -- Pod is core/v1-only
		// and always routes through the core/v1 registration.
		if pod, ok := rObj.(*corev1.Pod); ok {
			// Priority admission: resolve spec.priorityClassName -> a real
			// spec.priority before the scheduler ever sees this Pod (see
			// priority.go). priorityClassStore is nil in callers that don't
			// have scheduling.k8s.io/v1 registered (none today, but this
			// mirrors stores["limitranges"]'s existence check below rather
			// than assuming non-nil).
			if priorityClassStore != nil {
				if err := ResolvePodPriority(ctx, priorityClassStore, pod); err != nil {
					writeStatusError(w, http.StatusForbidden, "Forbidden", err.Error())
					return
				}
			}

			// Compute-class routing (namespace-first; see computeclass.go).
			// nsObj was fetched by namespace-lifecycle admission above --
			// a Pod create always has a namespace, so it's non-nil here
			// whenever a namespaces store exists at all.
			var nsLabels map[string]string
			if nsTyped, ok := nsObj.(*corev1.Namespace); ok {
				nsLabels = nsTyped.Labels
			}
			wantsContainers := PodWantsContainers(pod, nsLabels)
			if wantsContainers {
				MutatePodForComputeClass(pod)
			}
			if lrStore, exists := stores["limitranges"]; exists {
				if lrList, err := lrStore.List(ctx, namespace, "", ""); err == nil {
					limitRanges := lrList.(*corev1.LimitRangeList).Items
					ApplyLimitRangeDefaults(pod, limitRanges)
					if err := ValidateLimitRange(pod, limitRanges); err != nil {
						writeStatusError(w, http.StatusForbidden, "Forbidden", err.Error())
						return
					}
				}
			}
			// Must run after the LimitRange defaulting above: it sizes
			// the Pod's dedicated NodeVM from its resource requests, so
			// a Pod relying on a namespace's LimitRange default (rather
			// than specifying resources itself) needs that default
			// filled in first, or every such Pod would resolve as if it
			// asked for nothing.
			if wantsContainers {
				if err := AssignContainersNode(pod); err != nil {
					writeStatusError(w, http.StatusForbidden, "Forbidden", err.Error())
					return
				}
			}
		}

		// Services that don't specify a ClusterIP get one allocated here,
		// synchronously, before the first write -- see clusterip.go.
		if svc, ok := rObj.(*corev1.Service); ok {
			if err := AssignClusterIP(ctx, store.storage, svc); err != nil {
				writeInternalError(w, fmt.Errorf("allocate ClusterIP: %w", err))
				return
			}
		}

		obj, err := store.Create(ctx, namespace, rObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}

		ApplyPostCreateEffects(ctx, stores, obj)

		writeRuntimeObject(w, http.StatusCreated, obj)

	case http.MethodPut:
		if name == "" {
			writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "name is required for update")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		fieldValidation, err := parseFieldValidation(r)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", err.Error())
			return
		}

		rObj, warnings, err := decodeBodyWithFieldValidation(body, fieldValidation, store.decodeDefaults())
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}
		writeFieldValidationWarnings(w, warnings)

		// A write that leaves a terminating object with no finalizers
		// completes its deletion instead of persisting (upstream
		// semantics; see gracefuldelete.go). This is how the real GC's
		// finalizer-clearing patch/update actually removes an owner it
		// finished orphaning or foreground-cascading.
		if shouldFinalizeDelete(rObj) {
			obj, err := finalizeDelete(ctx, store, namespacedStores, namespace, name, rObj)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			writeRuntimeObject(w, http.StatusOK, obj)
			return
		}

		obj, err := store.Update(ctx, namespace, name, rObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
		writeRuntimeObject(w, http.StatusOK, obj)

	case http.MethodDelete:
		if IsProtectedClusterDelete(prefix, resource, name) {
			writeStatusError(w, http.StatusForbidden, "Forbidden",
				"clusters \""+name+"\" is the management cluster and cannot be deleted")
			return
		}

		deleteOpts, err := parseDeleteOptions(r)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", err.Error())
			return
		}
		policy := deletePropagation(deleteOpts)
		// Everything the upstream store does is dry-run aware; the
		// cascades, sweeps and ClusterIP releases this handler runs
		// around it are not, so they are skipped instead.
		dryRun := dryrun.IsDryRun(deleteOpts.DryRun)

		if name == "" {
			labelSelector := r.URL.Query().Get("labelSelector")

			if resource == "namespaces" && namespacedStores != nil && !dryRun {
				// A collection-delete of Namespaces needs the same
				// dependents sweep as a single named delete below, or
				// DELETE /api/v1/namespaces would silently orphan every
				// namespaced resource in every namespace it removes.
				// List first, before anything is actually deleted, so the
				// sweep runs against exactly the Namespaces this request
				// is about to remove -- same idempotent-retry reasoning as
				// the single-delete case: nothing is deleted until every
				// sweep has succeeded.
				listObj, err := store.List(ctx, namespace, "", labelSelector)
				if err != nil {
					writeInternalError(w, err)
					return
				}
				nsList, ok := listObj.(*corev1.NamespaceList)
				if !ok {
					writeInternalError(w, fmt.Errorf("unexpected list type %T for namespaces collection delete", listObj))
					return
				}
				for _, ns := range nsList.Items {
					if err := DeleteNamespaceDependents(ctx, namespacedStores, ns.Name); err != nil {
						writeInternalError(w, err)
						return
					}
				}
			}

			if store.namespaced && namespacedStores != nil &&
				(policy == metav1.DeletePropagationOrphan || policy == metav1.DeletePropagationForeground) {
				// Graceful-deletion handoff, per matching item: mark each
				// terminating (deletionTimestamp + policy finalizer) and
				// return the list WITHOUT deleting anything -- the real
				// garbagecollector orphans / foreground-cascades and
				// completes each delete by clearing the finalizer, same
				// as the single-name path below. Uses each item's own
				// namespace, not the request's (which is "" for an
				// all-namespaces collection delete).
				listObj, err := store.List(ctx, namespace, "", labelSelector)
				if err != nil {
					writeInternalError(w, err)
					return
				}
				items, err := meta.ExtractList(listObj)
				if err != nil {
					writeInternalError(w, fmt.Errorf("extract %s list for graceful delete: %w", resource, err))
					return
				}
				terminating := make([]runtime.Object, 0, len(items))
				for _, item := range items {
					m := getObjectMeta(item)
					if m == nil {
						continue
					}
					marked, err := markForDeletion(ctx, store, m.Namespace, m.Name, deleteOpts.DeepCopy())
					if isStatusReason(err, metav1.StatusReasonNotFound) {
						continue // vanished between the list and the mark
					}
					if err != nil {
						writeInternalError(w, err)
						return
					}
					terminating = append(terminating, marked)
				}
				resultList := store.newListFunc()
				if err := meta.SetList(resultList, terminating); err != nil {
					writeInternalError(w, fmt.Errorf("assemble %s graceful-delete list: %w", resource, err))
					return
				}
				writeRuntimeObject(w, http.StatusOK, resultList)
				return
			}
			obj, err := store.DeleteCollection(ctx, namespace, labelSelector,
				ProtectedClusterCollectionKeep(prefix, resource), deleteOpts)
			if err != nil {
				writeInternalError(w, err)
				return
			}
			if svcList, ok := obj.(*corev1.ServiceList); ok && !dryRun {
				for i := range svcList.Items {
					ReleaseClusterIP(ctx, store.storage, &svcList.Items[i])
				}
			}
			writeRuntimeObject(w, http.StatusOK, obj)
			return
		}

		if resource == "namespaces" && namespacedStores != nil {
			// Check existence first, so deleting an already-gone namespace
			// still reports NotFound immediately rather than doing a List
			// call per namespaced resource type first.
			if _, err := store.Get(ctx, namespace, name); err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			// Sweep dependents BEFORE deleting the Namespace object: if this
			// fails partway, the Namespace stays visible/gettable, so a
			// client retry of the same DELETE is the correct recovery path
			// (every step is idempotent).
			if !dryRun {
				if err := DeleteNamespaceDependents(ctx, namespacedStores, name); err != nil {
					writeInternalError(w, err)
					return
				}
			}
		}

		if store.namespaced && namespacedStores != nil &&
			(policy == metav1.DeletePropagationOrphan || policy == metav1.DeletePropagationForeground) {
			// Graceful-deletion handoff to the real garbagecollector:
			// stamp deletionTimestamp + the policy's finalizer and
			// return the terminating object WITHOUT deleting it. The GC
			// orphans (or foreground-cascades) the dependents and then
			// patches the finalizer off, which completes the delete via
			// shouldFinalizeDelete in the write paths. See
			// gracefuldelete.go for why the earlier synchronous
			// alternatives all raced the live controllers.
			terminating, err := markForDeletion(ctx, store, namespace, name, deleteOpts)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			writeRuntimeObject(w, http.StatusOK, terminating)
			return
		}
		obj, err := store.Delete(ctx, namespace, name, deleteOpts)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
		if dryRun {
			writeRuntimeObject(w, http.StatusOK, obj)
			return
		}
		if resource == "namespaces" && namespacedStores != nil {
			if err := SweepNamespaceEventsAfterDelete(ctx, namespacedStores, name); err != nil {
				writeInternalError(w, err)
				return
			}
		}
		settleDeletedObject(ctx, store.storage, obj)
		if store.namespaced {
			FinishUnblockedForegroundOwners(ctx, namespacedStores, store, namespace, obj)
		}
		writeRuntimeObject(w, http.StatusOK, obj)

	case http.MethodPatch:
		if name == "" {
			writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "name is required for patch")
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		ct := r.Header.Get("Content-Type")
		// Get -> apply -> conditional-update, retried on conflict: a
		// patch expresses intent against WHATEVER the current object is
		// (it carries no resourceVersion of its own), so when another
		// writer lands between the read and the write, the correct
		// behavior is to re-read and re-apply -- upstream's patch
		// handler retries exactly this way. Without it, the GC circle
		// conformance test flaked whenever the kubelet's status write
		// raced the test's ownerReference patch ("the object has been
		// modified", runs 29118462901/29139324774).
		var lastPatchErr error
		for attempt := 0; attempt < patchConflictRetries; attempt++ {
			currentObj, err := store.Get(ctx, namespace, name)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}

			patchedObj, err := applyPatch(currentObj, body, ct)
			if err != nil {
				writeStatusError(w, http.StatusBadRequest, "BadRequest", "patch failed: "+err.Error())
				return
			}

			// Same finalizer-completion rule as the PUT path above (see
			// gracefuldelete.go) -- the GC clears finalizers via PATCH.
			if shouldFinalizeDelete(patchedObj) {
				obj, err := finalizeDelete(ctx, store, namespacedStores, namespace, name, patchedObj)
				if err != nil {
					writeResourceError(w, err, resource, name)
					return
				}
				writeRuntimeObject(w, http.StatusOK, obj)
				return
			}

			obj, err := store.Update(ctx, namespace, name, patchedObj)
			if err == nil {
				writeRuntimeObject(w, http.StatusOK, obj)
				return
			}
			if isStatusReason(err, metav1.StatusReasonConflict) {
				lastPatchErr = err
				continue
			}
			writeResourceError(w, err, resource, name)
			return
		}
		writeResourceError(w, lastPatchErr, resource, name)

	default:
		writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported")
	}
}

// settleDeletedObject runs the per-resource effects every COMPLETED
// deletion needs: ClusterIP release for Services. Shared
// by the DELETE path and finalizeDelete -- before 2026-07-25 the
// finalizer-completion deletes (PUT/PATCH) skipped the Service
// effects, leaking the ClusterIP of any Service that finished deleting
// via a cleared finalizer (found by review).
func settleDeletedObject(ctx context.Context, storage *Storage, obj runtime.Object) {
	if svc, ok := obj.(*corev1.Service); ok {
		ReleaseClusterIP(ctx, storage, svc)
	}
}

// finalizeDelete completes the deletion of an object whose last
// finalizer was just cleared (shouldFinalizeDelete): orphan-straggler
// sweep, storage delete, per-resource settle. Shared by the PUT and
// PATCH finalizer-completion paths.
func finalizeDelete(ctx context.Context, store *ResourceStore, namespacedStores []*ResourceStore, namespace, name string, write runtime.Object) (runtime.Object, error) {
	if err := refuseForegroundFinalize(ctx, store, namespacedStores, namespace, name); err != nil {
		return nil, err
	}
	if err := finalizeDeleteWithOrphanSweep(ctx, store, namespacedStores, namespace, name); err != nil {
		return nil, err
	}
	// For a migrated resource, the write itself is what completes the
	// deletion: upstream's Store.Update detects that the update empties
	// the finalizers of an object already marked for deletion and removes
	// it (ShouldDeleteDuringUpdate), returning the object it deleted. A
	// plain Delete would NOT do it -- upstream treats a delete of an
	// already-terminating object as a no-op that just reports the object.
	obj, err := store.Update(ctx, namespace, name, write)
	if err != nil {
		return nil, err
	}
	settleDeletedObject(ctx, store.storage, obj)
	if store.namespaced {
		FinishUnblockedForegroundOwners(ctx, namespacedStores, store, namespace, obj)
	}
	return obj, nil
}

// parseResourcePath extracts resource, namespace, name, and subresource
// from a path that's already had its "/api/v1/" or "/apis/{group}/{version}/"
// prefix stripped. Returns (resource, namespace, name, subresource, ok).
//
// Supported patterns:
//   - {resource}                                    -> resource list (cluster or all-namespaces)
//   - {resource}/{name}                              -> cluster-scoped get/update/delete (e.g. namespaces)
//   - {resource}/{name}/{subresource}                -> cluster-scoped subresource (e.g. nodes/mynode/status)
//   - namespaces/{ns}/{resource}                     -> namespaced list
//   - namespaces/{ns}/{resource}/{name}              -> namespaced get/update/delete
//   - namespaces/{ns}/{resource}/{name}/{subresource} -> namespaced subresource (e.g. pods/nginx/status)
func parseResourcePath(path string) (resource, namespace, name, subresource string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return "", "", "", "", false
	}

	parts := strings.Split(path, "/")

	switch len(parts) {
	case 1:
		return parts[0], "", "", "", true

	case 2:
		if parts[0] == "namespaces" {
			// namespaces/{name} -> get a specific namespace
			return "namespaces", "", parts[1], "", true
		}
		// cluster-scoped resource with name
		return parts[0], "", parts[1], "", true

	case 3:
		if parts[0] == "namespaces" {
			// namespaces/{ns}/{resource}
			return parts[2], parts[1], "", "", true
		}
		// {resource}/{name}/{subresource}
		return parts[0], "", parts[1], parts[2], true

	case 4:
		if parts[0] == "namespaces" {
			// namespaces/{ns}/{resource}/{name}
			return parts[2], parts[1], parts[3], "", true
		}
		return "", "", "", "", false

	case 5:
		if parts[0] == "namespaces" {
			// namespaces/{ns}/{resource}/{name}/{subresource}
			return parts[2], parts[1], parts[3], parts[4], true
		}
		return "", "", "", "", false

	default:
		return "", "", "", "", false
	}
}

// writeGetResponse writes obj as the response to a GET (get or list), as a
// meta.k8s.io/v1 Table if r's Accept header requested one (kubectl's default
// human-readable "get" output), or as obj itself otherwise.
func writeGetResponse(w http.ResponseWriter, r *http.Request, obj runtime.Object) {
	if wantsTable(r) {
		table, err := ConvertToTable(obj)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writeRuntimeObject(w, http.StatusOK, table)
		return
	}
	writeRuntimeObject(w, http.StatusOK, obj)
}

// writeRuntimeObject encodes a runtime.Object to JSON and writes it to the response.
func writeRuntimeObject(w http.ResponseWriter, statusCode int, obj runtime.Object) {
	data, err := Encode(obj)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(data)
}

// writeStatusError writes a Kubernetes Status error response.
func writeStatusError(w http.ResponseWriter, code int, reason string, message string) {
	writeJSON(w, code, statusResponse{
		Kind:       "Status",
		APIVersion: "v1",
		Metadata:   map[string]string{},
		Status:     "Failure",
		Message:    message,
		Reason:     reason,
		Code:       code,
	})
}

// writeInternalError writes a 500 Internal Server Error as a Kubernetes Status object.
func writeInternalError(w http.ResponseWriter, err error) {
	writeStatusError(w, http.StatusInternalServerError, "InternalError", err.Error())
}

// writeResourceError maps storage errors to appropriate Kubernetes Status responses.
func writeResourceError(w http.ResponseWriter, err error, resource string, name string) {
	var se *StatusError
	if errors.As(err, &se) {
		writeJSON(w, int(se.Status.Code), se.Status)
		return
	}
	// Upstream registry errors (genericregistry.Store, S25 migration)
	// arrive as apierrors.StatusError -- pass their Status through so
	// clients get the real apiserver-shaped error body.
	if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
		st := status.Status()
		if st.Code == 0 {
			st.Code = http.StatusInternalServerError
		}
		writeJSON(w, int(st.Code), st)
		return
	}
	writeInternalError(w, err)
}
