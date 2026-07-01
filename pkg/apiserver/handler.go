package apiserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// RegisterAPIHandlers registers a single handler on mux that dispatches
// Kubernetes-style /api/v1/ requests to the appropriate ResourceStore.
func RegisterAPIHandlers(mux *http.ServeMux, stores map[string]*ResourceStore) {
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		HandleAPI(w, r, stores)
	})
}

// HandleAPI parses a Kubernetes API URL and dispatches to the correct
// ResourceStore method based on the HTTP method and path segments.
func HandleAPI(w http.ResponseWriter, r *http.Request, stores map[string]*ResourceStore) {
	// Check for watch requests
	if r.URL.Query().Get("watch") == "true" {
		resource, namespace := parseWatchParams(r.URL.Path)
		HandleWatch(w, r, resource, namespace)
		return
	}

	resource, namespace, name, subresource, ok := parsePath(r.URL.Path)
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
			obj, err := store.List(ctx, namespace, fieldSelector)
			if err != nil {
				writeInternalError(w, err)
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

		obj, err := store.Create(ctx, namespace, rObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
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
			writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "name is required for delete")
			return
		}

		obj, err := store.Delete(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
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

// parsePath extracts resource, namespace, name, and subresource from a /api/v1/ path.
// Returns (resource, namespace, name, subresource, ok).
//
// Supported patterns:
//   - /api/v1/{resource}                                    -> resource list (cluster or all-namespaces)
//   - /api/v1/{resource}/{name}                             -> cluster-scoped get/update/delete (e.g. namespaces)
//   - /api/v1/{resource}/{name}/{subresource}               -> cluster-scoped subresource (e.g. nodes/mynode/status)
//   - /api/v1/namespaces/{ns}/{resource}                    -> namespaced list
//   - /api/v1/namespaces/{ns}/{resource}/{name}             -> namespaced get/update/delete
//   - /api/v1/namespaces/{ns}/{resource}/{name}/{subresource} -> namespaced subresource (e.g. pods/nginx/status)
func parsePath(path string) (resource, namespace, name, subresource string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/api/v1/")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", "", "", "", false
	}

	parts := strings.Split(trimmed, "/")

	switch len(parts) {
	case 1:
		// /api/v1/{resource}
		return parts[0], "", "", "", true

	case 2:
		if parts[0] == "namespaces" {
			// /api/v1/namespaces/{name} -> get a specific namespace
			return "namespaces", "", parts[1], "", true
		}
		// cluster-scoped resource with name
		return parts[0], "", parts[1], "", true

	case 3:
		if parts[0] == "namespaces" {
			// /api/v1/namespaces/{ns}/{resource}
			return parts[2], parts[1], "", "", true
		}
		// /api/v1/{resource}/{name}/{subresource}
		return parts[0], "", parts[1], parts[2], true

	case 4:
		if parts[0] == "namespaces" {
			// /api/v1/namespaces/{ns}/{resource}/{name}
			return parts[2], parts[1], parts[3], "", true
		}
		return "", "", "", "", false

	case 5:
		if parts[0] == "namespaces" {
			// /api/v1/namespaces/{ns}/{resource}/{name}/{subresource}
			return parts[2], parts[1], parts[3], parts[4], true
		}
		return "", "", "", "", false

	default:
		return "", "", "", "", false
	}
}

// parseWatchParams extracts resource and namespace from a watch request path.
func parseWatchParams(path string) (resource, namespace string) {
	resource, namespace, _, _, _ = parsePath(path)
	return resource, namespace
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

// HandleGroupAPI handles requests to /apis/{group}/{version}/... paths.
// It strips the group+version prefix and dispatches to the appropriate ResourceStore.
func HandleGroupAPI(w http.ResponseWriter, r *http.Request, stores map[string]*ResourceStore) {
	// Strip /apis/{group}/{version}/ prefix to get the remaining path segments
	path := r.URL.Path
	trimmed := strings.TrimPrefix(path, "/apis/")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		writeStatusError(w, http.StatusNotFound, "NotFound", "the path is not valid")
		return
	}

	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 3 {
		writeStatusError(w, http.StatusNotFound, "NotFound", "the path is not valid")
		return
	}

	// parts[0] = group (e.g. "coordination.k8s.io")
	// parts[1] = version (e.g. "v1")
	// parts[2] = remaining path (e.g. "namespaces/kube-system/leases/mynode")
	remaining := parts[2]

	// Parse the remaining path the same way as parsePath but without the /api/v1/ prefix
	resource, namespace, name, subresource, ok := parseRemainingPath(remaining)
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
			obj, err := store.List(ctx, namespace, fieldSelector)
			if err != nil {
				writeInternalError(w, err)
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

		obj, err := store.Create(ctx, namespace, rObj)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
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
			writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "name is required for delete")
			return
		}

		obj, err := store.Delete(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
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

// parseRemainingPath parses a path without the /api/v1/ or /apis/{group}/{version}/ prefix.
// It handles the same patterns as parsePath but operates on the remaining segments directly.
func parseRemainingPath(path string) (resource, namespace, name, subresource string, ok bool) {
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
			return "namespaces", "", parts[1], "", true
		}
		return parts[0], "", parts[1], "", true

	case 3:
		if parts[0] == "namespaces" {
			return parts[2], parts[1], "", "", true
		}
		return parts[0], "", parts[1], parts[2], true

	case 4:
		if parts[0] == "namespaces" {
			return parts[2], parts[1], parts[3], "", true
		}
		return "", "", "", "", false

	case 5:
		if parts[0] == "namespaces" {
			return parts[2], parts[1], parts[3], parts[4], true
		}
		return "", "", "", "", false

	default:
		return "", "", "", "", false
	}
}
