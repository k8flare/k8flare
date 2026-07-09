package apiserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// parseDeletePropagationPolicy extracts spec.propagationPolicy from a DELETE
// request. Real clients (client-go's Delete/DeleteCollection) send it as a
// JSON-encoded metav1.DeleteOptions request body; the query parameter form
// (?propagationPolicy=Foreground) some direct callers use instead is also
// accepted, matching upstream kube-apiserver's dual acceptance. Restores
// r.Body after reading it so later code in the same request (there is none
// today, but this must not be a trap for a future caller) still sees it.
// An absent/empty policy defaults to Background, matching this project's
// registered resources' upstream default (none opt into Orphan-by-default).
func parseDeletePropagationPolicy(r *http.Request) (metav1.DeletionPropagation, error) {
	if q := r.URL.Query().Get("propagationPolicy"); q != "" {
		return metav1.DeletionPropagation(q), nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("read request body: %w", err)
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) == 0 {
		return metav1.DeletePropagationBackground, nil
	}
	// client-go's default negotiated content type is protobuf, not JSON --
	// same reason decodeBody below needs Codecs.UniversalDeserializer rather
	// than plain encoding/json (found by running this against a real
	// client-go client, not assumed: a bare json.Unmarshal failed to decode
	// with "invalid character 'k' looking for beginning of value", the tell
	// for feeding protobuf bytes to a JSON decoder).
	obj, err := decodeBody(body)
	if err != nil {
		// Real kubectl sends its DeleteOptions body WITHOUT TypeMeta
		// (`{"propagationPolicy":"Background"}`), which the universal
		// deserializer rejects with "Object 'Kind' is missing" -- the
		// upstream apiserver decodes options leniently for exactly this
		// reason. Fall back to a plain JSON unmarshal before failing;
		// found live when `kubectl delete deployment` 400'd against
		// production while curl (with TypeMeta) worked.
		var jsonOpts metav1.DeleteOptions
		if jsonErr := json.Unmarshal(body, &jsonOpts); jsonErr == nil {
			if jsonOpts.PropagationPolicy == nil {
				return metav1.DeletePropagationBackground, nil
			}
			return *jsonOpts.PropagationPolicy, nil
		}
		return "", fmt.Errorf("decode delete options: %w", err)
	}
	opts, ok := obj.(*metav1.DeleteOptions)
	if !ok {
		return "", fmt.Errorf("decode delete options: unexpected type %T", obj)
	}
	if opts.PropagationPolicy == nil {
		return metav1.DeletePropagationBackground, nil
	}
	return *opts.PropagationPolicy, nil
}

// decodeBody decodes the request body as a Kubernetes runtime.Object.
// Uses UniversalDeserializer which auto-detects JSON and protobuf formats.
func decodeBody(body []byte) (runtime.Object, error) {
	obj, _, err := Codecs.UniversalDeserializer().Decode(body, nil, nil)
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
func HandleResource(w http.ResponseWriter, r *http.Request, prefix string, stores map[string]*ResourceStore, namespacedStores []*ResourceStore, priorityClassStore *ResourceStore) {
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

		rObj, warnings, err := decodeBodyWithFieldValidation(body, fieldValidation)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}
		writeFieldValidationWarnings(w, warnings)

		ApplyDefaults(rObj)

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
			// stores["namespaces"] only exists in the core/v1 stores map,
			// which is also the only map that can contain pods -- so the
			// lookup is always available on this path.
			var nsLabels map[string]string
			if nsStore, exists := stores["namespaces"]; exists {
				if nsObj, err := nsStore.Get(ctx, "", namespace); err == nil {
					if nsTyped, ok := nsObj.(*corev1.Namespace); ok {
						nsLabels = nsTyped.Labels
					}
				}
			}
			if PodWantsContainers(pod, nsLabels) {
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
		}

		// Services that don't specify a ClusterIP get one allocated here,
		// synchronously, before the first write -- see clusterip.go.
		if svc, ok := rObj.(*corev1.Service); ok {
			if err := AssignClusterIP(ctx, store.storage, svc); err != nil {
				writeInternalError(w, fmt.Errorf("allocate ClusterIP: %w", err))
				return
			}
		}

		// Nodes that don't specify a PodCIDR get one allocated here,
		// synchronously, before the first write -- see nodecidr.go.
		if node, ok := rObj.(*corev1.Node); ok {
			if err := AssignPodCIDR(ctx, store.storage, node); err != nil {
				writeInternalError(w, fmt.Errorf("allocate PodCIDR: %w", err))
				return
			}
		}

		obj, err := store.Create(ctx, namespace, rObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}

		ApplyPostCreateEffects(ctx, stores, obj)
		TriggerEndpointsReconcile(ctx, store.storage, namespace, obj)

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

		rObj, warnings, err := decodeBodyWithFieldValidation(body, fieldValidation)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}
		writeFieldValidationWarnings(w, warnings)

		obj, err := store.Update(ctx, namespace, name, rObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
		TriggerEndpointsReconcile(ctx, store.storage, namespace, obj)
		writeRuntimeObject(w, http.StatusOK, obj)

	case http.MethodDelete:
		policy, err := parseDeletePropagationPolicy(r)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", err.Error())
			return
		}

		if name == "" {
			labelSelector := r.URL.Query().Get("labelSelector")

			if resource == "namespaces" && namespacedStores != nil {
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

			obj, err := store.DeleteCollection(ctx, namespace, labelSelector)
			if err != nil {
				writeInternalError(w, err)
				return
			}
			if store.namespaced && namespacedStores != nil && policy == metav1.DeletePropagationOrphan {
				// Real GC (pkg/controllers/gc) handles Background/Foreground
				// cascade delete asynchronously; Orphan is the one policy
				// this apiserver still handles synchronously (see
				// orphan.go). Uses each item's own namespace, not the
				// request's (which is "" for an all-namespaces collection
				// delete).
				items, err := meta.ExtractList(obj)
				if err != nil {
					writeInternalError(w, fmt.Errorf("extract deleted %s list: %w", resource, err))
					return
				}
				for _, item := range items {
					if m := getObjectMeta(item); m != nil {
						if err := OrphanDependents(ctx, namespacedStores, m.Namespace, m.UID); err != nil {
							writeInternalError(w, err)
							return
						}
					}
				}
			}
			if svcList, ok := obj.(*corev1.ServiceList); ok {
				for i := range svcList.Items {
					ReleaseClusterIP(ctx, store.storage, &svcList.Items[i])
					DeleteServiceEndpoints(ctx, store.storage, namespace, svcList.Items[i].Name)
				}
			}
			if nodeList, ok := obj.(*corev1.NodeList); ok {
				for i := range nodeList.Items {
					ReleasePodCIDR(ctx, store.storage, &nodeList.Items[i])
				}
			}
			if _, ok := obj.(*corev1.PodList); ok {
				// Every matching Pod in this namespace is gone -- one
				// reconcile pass recomputes every affected Service's
				// Endpoints/EndpointSlice, same as a single Pod delete
				// below, without needing to iterate per-Pod.
				if err := ReconcileNamespaceEndpoints(ctx, store.storage, namespace); err != nil {
					log.Printf("endpoints reconciliation error for namespace %s: %v", namespace, err)
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
			if err := DeleteNamespaceDependents(ctx, namespacedStores, name); err != nil {
				writeInternalError(w, err)
				return
			}
		}

		obj, err := store.Delete(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
		if svc, ok := obj.(*corev1.Service); ok {
			ReleaseClusterIP(ctx, store.storage, svc)
			DeleteServiceEndpoints(ctx, store.storage, namespace, svc.Name)
		}
		if node, ok := obj.(*corev1.Node); ok {
			ReleasePodCIDR(ctx, store.storage, node)
		}
		if store.namespaced && namespacedStores != nil && policy == metav1.DeletePropagationOrphan {
			// Real GC (pkg/controllers/gc) handles Background/Foreground
			// cascade delete asynchronously; Orphan is the one policy
			// this apiserver still handles synchronously (see orphan.go).
			if m := getObjectMeta(obj); m != nil {
				if err := OrphanDependents(ctx, namespacedStores, namespace, m.UID); err != nil {
					writeInternalError(w, err)
					return
				}
			}
		}
		TriggerEndpointsReconcile(ctx, store.storage, namespace, obj)
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

		obj, err := store.Update(ctx, namespace, name, patchedObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
		TriggerEndpointsReconcile(ctx, store.storage, namespace, obj)
		writeRuntimeObject(w, http.StatusOK, obj)

	default:
		writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported")
	}
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
	writeInternalError(w, err)
}
