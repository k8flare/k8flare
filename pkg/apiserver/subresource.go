package apiserver

import (
	"fmt"
	"io"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	jsonpatch "gopkg.in/evanphx/json-patch.v4"
)

// HandleSubresource routes subresource requests (e.g. pods/status, pods/binding, nodes/status)
// to the appropriate handler based on resource, subresource, and HTTP method.
func HandleSubresource(w http.ResponseWriter, r *http.Request, stores map[string]*ResourceStore, resource, namespace, name, subresource string) {
	store, exists := stores[resource]
	if !exists {
		writeStatusError(w, http.StatusNotFound, "NotFound", "the server doesn't have a resource type \""+resource+"\"")
		return
	}

	ctx := r.Context()

	switch resource + "/" + subresource {
	case "pods/status":
		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
				return
			}
			defer r.Body.Close()

			incomingObj, err := decodeBody(body)
			if err != nil {
				writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
				return
			}
			incoming := incomingObj.(*corev1.Pod)

			currentObj, err := store.Get(ctx, namespace, name)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			currentPod := currentObj.(*corev1.Pod)

			currentPod.Status = incoming.Status

			obj, err := store.Update(ctx, namespace, name, currentPod)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			writeRuntimeObject(w, http.StatusOK, obj)

		case http.MethodPatch:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
				return
			}
			defer r.Body.Close()

			currentObj, err := store.Get(ctx, namespace, name)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}

			ct := r.Header.Get("Content-Type")
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
			writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported for "+resource+"/"+subresource)
		}

	case "pods/binding":
		if r.Method != http.MethodPost {
			writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported for "+resource+"/"+subresource)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		bindingObj, err := decodeBody(body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode binding: "+err.Error())
			return
		}
		binding := bindingObj.(*corev1.Binding)

		currentObj, err := store.Get(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}
		pod := currentObj.(*corev1.Pod)

		pod.Spec.NodeName = binding.Target.Name

		_, err = store.Update(ctx, namespace, name, pod)
		if err != nil {
			writeResourceError(w, err, resource, name)
			return
		}

		writeRuntimeObject(w, http.StatusCreated, binding)

	case "nodes/status":
		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
				return
			}
			defer r.Body.Close()

			incomingObj, err := decodeBody(body)
			if err != nil {
				writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
				return
			}
			incoming := incomingObj.(*corev1.Node)

			currentObj, err := store.Get(ctx, namespace, name)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			currentNode := currentObj.(*corev1.Node)

			currentNode.Status = incoming.Status

			obj, err := store.Update(ctx, namespace, name, currentNode)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}
			writeRuntimeObject(w, http.StatusOK, obj)

		case http.MethodPatch:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
				return
			}
			defer r.Body.Close()

			currentObj, err := store.Get(ctx, namespace, name)
			if err != nil {
				writeResourceError(w, err, resource, name)
				return
			}

			ct := r.Header.Get("Content-Type")
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
			writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported for "+resource+"/"+subresource)
		}

	case "pods/log":
		// Handled by JS layer (worker.mjs) via VPC Service binding.
		// This case is only hit in integration tests where JS layer is bypassed.
		writeStatusError(w, http.StatusNotImplemented, "NotImplemented",
			"pod logs require kubelet proxy (handled by JS layer in production)")

	case "pods/exec", "pods/attach":
		// Handled by JS layer (worker.mjs) via VPC Service binding WebSocket proxy.
		// In production, worker.mjs intercepts these requests before they reach Go WASM.
		// Full support requires:
		//   1. Workers WebSocket upgrade for the client (kubectl) connection
		//   2. VPC Service binding WebSocket upgrade for the kubelet connection
		//   3. Bidirectional stream bridging between client and kubelet WebSockets
		// This fallback is hit when the JS layer is bypassed (e.g. integration tests).
		writeStatusError(w, http.StatusNotImplemented, "NotImplemented",
			fmt.Sprintf("%s requires WebSocket proxy (handled by JS layer in production)", subresource))

	default:
		writeStatusError(w, http.StatusNotFound, "NotFound", "the server does not support the subresource \""+subresource+"\" for resource \""+resource+"\"")
	}
}

// applyPatch applies a patch to a runtime.Object based on the given content type.
// Supported patch types: merge-patch+json, strategic-merge-patch+json, json-patch+json.
func applyPatch(currentObj runtime.Object, patchBytes []byte, contentType string) (runtime.Object, error) {
	currentJSON, err := Encode(currentObj)
	if err != nil {
		return nil, err
	}

	var patchedJSON []byte

	switch contentType {
	case "application/merge-patch+json":
		patchedJSON, err = jsonpatch.MergePatch(currentJSON, patchBytes)
		if err != nil {
			return nil, err
		}

	case "application/strategic-merge-patch+json":
		patchedJSON, err = strategicpatch.StrategicMergePatch(currentJSON, patchBytes, currentObj)
		if err != nil {
			return nil, err
		}

	case "application/json-patch+json":
		patch, err := jsonpatch.DecodePatch(patchBytes)
		if err != nil {
			return nil, err
		}
		patchedJSON, err = patch.Apply(currentJSON)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported patch type")
	}

	obj, _, err := Codecs.UniversalDeserializer().Decode(patchedJSON, nil, nil)
	if err != nil {
		return nil, err
	}
	return obj, nil
}
