package apiserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

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
// namespacedStores, if non-nil, is swept when a Namespace object itself is
// deleted (see NamespacedResourceStores and the http.MethodDelete case
// below). Only the core/v1 registration passes it: "namespaces" never
// exists as a key in any other group's stores map, so the cascading-delete
// branch is naturally unreachable for group-API calls even when they pass
// their own (always nil) namespacedStores.
//
// This single function replaces what used to be two near-identical
// functions, HandleAPI and HandleGroupAPI: same CRUD switch, same path
// grammar, differing only in how the group+version prefix got stripped
// before parsing and in whether a watch query parameter was checked at all
// (HandleGroupAPI's watch requests fell through to a duplicated list
// pathway -- a real drift merging them fixes, not just a line-count cut).
func HandleResource(w http.ResponseWriter, r *http.Request, prefix string, stores map[string]*ResourceStore, namespacedStores []*ResourceStore) {
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
			writeRuntimeObject(w, http.StatusOK, obj)
		} else {
			obj, err := store.Get(ctx, namespace, name)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			writeRuntimeObject(w, http.StatusOK, obj)
		}

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		rObj, err := decodeBody(body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}

		ApplyDefaults(rObj)

		// Fill in any container resource requests/limits the pod itself
		// didn't specify, then reject it if it still violates a
		// Container-scoped LimitRange's Min/Max, from LimitRanges in its
		// namespace. stores["limitranges"] is absent from every group-API
		// store map (leases/storage/nodeAPI/resourceAPI/apps/policy/discovery/
		// networking/batch), so this is a no-op there -- Pod is core/v1-only
		// and always routes through the core/v1 registration.
		if pod, ok := rObj.(*corev1.Pod); ok {
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

		rObj, err := decodeBody(body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}

		obj, err := store.Update(ctx, namespace, name, rObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
		writeRuntimeObject(w, http.StatusOK, obj)

	case http.MethodDelete:
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
			if svcList, ok := obj.(*corev1.ServiceList); ok {
				for i := range svcList.Items {
					ReleaseClusterIP(ctx, store.storage, &svcList.Items[i])
				}
			}
			if nodeList, ok := obj.(*corev1.NodeList); ok {
				for i := range nodeList.Items {
					ReleasePodCIDR(ctx, store.storage, &nodeList.Items[i])
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
		}
		if node, ok := obj.(*corev1.Node); ok {
			ReleasePodCIDR(ctx, store.storage, node)
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
