package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// HandleSubresource routes subresource requests (e.g. pods/status,
// pods/binding, nodes/status) to the appropriate handler based on resource,
// subresource, and HTTP method.
//
// "status" dispatches to one generic handler (handleStatusSubresource) for
// every resource apidef.Table actually declares a "status" subresource for
// (pods, nodes, replicasets, deployments, daemonsets, jobs, cronjobs) --
// replacing what used to be 7 hand-copied ~55-line blocks, one per
// resource, that had already drifted from each other (apps/v1 and batch/v1's
// copies existed here but were never advertised in discovery.go; see
// apidef.Table's doc comment). Gated on apidef.HasSubresource rather than
// dispatching by subresource name alone: Service, for one, has a real Go
// `.Status` field but no declared "status" subresource in the table, and
// early versions of this dispatch let it (and anything else with a Status
// field) through anyway -- the exact kind of table/handler drift this
// project's generator work elsewhere exists to eliminate, found by review
// and covered by TestStatusSubresourceRejectsUndeclaredResource.
//
// binding/log/exec/attach are still Pod-specific (apidef.Table only ever
// declares them under "pods") and are explicitly rejected for any other
// resource before reaching their handlers below -- handleBindingSubresource
// in particular assumes it was only ever called for a Pod (it type-asserts
// the fetched object straight to *corev1.Pod), and pkg/apiserver has no
// recover() anywhere, so a resource mismatch reaching it would panic the
// whole request instead of cleanly 404ing.
func HandleSubresource(w http.ResponseWriter, r *http.Request, stores map[string]*ResourceStore, resource, namespace, name, subresource string) {
	store, exists := stores[resource]
	if !exists {
		writeStatusError(w, http.StatusNotFound, "NotFound", "the server doesn't have a resource type \""+resource+"\"")
		return
	}

	switch subresource {
	case "status":
		if !apidef.HasSubresource(resource, "status") {
			writeStatusError(w, http.StatusNotFound, "NotFound", "the server does not support the subresource \""+subresource+"\" for resource \""+resource+"\"")
			return
		}
		handleStatusSubresource(w, r, store, namespace, name)

	case "scale":
		if !apidef.HasSubresource(resource, "scale") {
			writeStatusError(w, http.StatusNotFound, "NotFound", "the server does not support the subresource \""+subresource+"\" for resource \""+resource+"\"")
			return
		}
		handleScaleSubresource(w, r, store, namespace, name)

	case "binding", "log", "exec", "attach":
		if resource != "pods" {
			writeStatusError(w, http.StatusNotFound, "NotFound", "the server does not support the subresource \""+subresource+"\" for resource \""+resource+"\"")
			return
		}
		handlePodOnlySubresource(w, r, store, namespace, name, subresource)

	default:
		writeStatusError(w, http.StatusNotFound, "NotFound", "the server does not support the subresource \""+subresource+"\" for resource \""+resource+"\"")
	}
}

// handlePodOnlySubresource dispatches the subresources only Pod has.
// Callers must have already confirmed resource == "pods".
func handlePodOnlySubresource(w http.ResponseWriter, r *http.Request, store *ResourceStore, namespace, name, subresource string) {
	switch subresource {
	case "binding":
		handleBindingSubresource(w, r, store, namespace, name)

	case "log":
		// Handled by JS layer (worker.mjs) via VPC Service binding.
		// This case is only hit in integration tests where JS layer is bypassed.
		writeStatusError(w, http.StatusNotImplemented, "NotImplemented",
			"pod logs require kubelet proxy (handled by JS layer in production)")

	case "exec", "attach":
		// Handled by JS layer (worker.mjs) via VPC Service binding WebSocket proxy.
		// In production, worker.mjs intercepts these requests before they reach Go WASM.
		// Full support requires:
		//   1. Workers WebSocket upgrade for the client (kubectl) connection
		//   2. VPC Service binding WebSocket upgrade for the kubelet connection
		//   3. Bidirectional stream bridging between client and kubelet WebSockets
		// This fallback is hit when the JS layer is bypassed (e.g. integration tests).
		writeStatusError(w, http.StatusNotImplemented, "NotImplemented",
			fmt.Sprintf("%s requires WebSocket proxy (handled by JS layer in production)", subresource))
	}
}

// handleStatusSubresource implements GET/PUT/PATCH for any resource's
// /status subresource generically:
//   - GET returns the whole object (its Status is already part of it).
//   - PUT copies only .Status from the request body onto the currently
//     stored object (via copyStatus/reflection), leaving spec/metadata as
//     they were -- a client PUTting .../status is only supposed to be able
//     to change status.
//   - PATCH applies the patch to the whole object, same as the top-level
//     PATCH handler (already generic; only .Status is expected to differ).
func handleStatusSubresource(w http.ResponseWriter, r *http.Request, store *ResourceStore, namespace, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		obj, err := store.Get(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}
		writeRuntimeObject(w, http.StatusOK, obj)

	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		incoming, err := decodeBody(body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}

		current, err := store.Get(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}

		if err := copyStatus(current, incoming); err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "status update failed: "+err.Error())
			return
		}

		obj, err := store.Update(ctx, namespace, name, current)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}
		// A Pod's status is the real trigger for most Endpoints/EndpointSlice
		// changes (podIP/readiness populate here, via kubelet's UpdateStatus
		// call) -- see endpoints.go's TriggerEndpointsReconcile.
		TriggerEndpointsReconcile(ctx, store.storage, namespace, obj)
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
			writeResourceError(w, err, store.resource, name)
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
			writeResourceError(w, err, store.resource, name)
			return
		}
		TriggerEndpointsReconcile(ctx, store.storage, namespace, obj)
		writeRuntimeObject(w, http.StatusOK, obj)

	default:
		writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported for "+store.resource+"/status")
	}
}

// copyStatus copies the .Status field from src onto dst using reflection.
// Every Kubernetes API type with a /status subresource has a top-level
// Status field of the same name; this generalizes what used to be a
// per-type "currentX.Status = incomingX.Status" assignment (repeated once
// per resource) the same way k8s.io/apimachinery/pkg/api/meta.SetList
// (store.go) already generalizes "listX.Items = append(...)" without
// per-type code. dst and src are always the same concrete type in practice
// (both come from the same ResourceStore's newFunc), so this is a same-type
// field copy, not a cross-type conversion.
func copyStatus(dst, src runtime.Object) error {
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()
	if dstVal.Type() != srcVal.Type() {
		return fmt.Errorf("status update body is %T, expected %T", src, dst)
	}

	dstStatus := dstVal.FieldByName("Status")
	srcStatus := srcVal.FieldByName("Status")
	if !dstStatus.IsValid() || !srcStatus.IsValid() {
		return fmt.Errorf("%T has no Status field", dst)
	}
	if !dstStatus.CanSet() {
		return fmt.Errorf("%T.Status is not settable", dst)
	}

	dstStatus.Set(srcStatus)
	return nil
}

// handleBindingSubresource implements POST for pods/binding: the real
// kube-scheduler's bind path, which sets spec.nodeName by creating a
// Binding object rather than PATCHing the Pod directly. Pod-specific (no
// other resource in apidef.Table has a "binding" subresource), so it stays
// its own small case rather than a generic table-driven handler. Callers
// (HandleSubresource) must have already confirmed store's resource is
// "pods" -- the type assertions below still use the comma-ok form instead
// of trusting that, since pkg/apiserver has no recover() anywhere and a
// client-supplied body that doesn't actually decode to a Binding (e.g. a
// Pod manifest POSTed to .../binding by mistake) is squarely
// attacker/caller-controlled input, not just an internal invariant.
func handleBindingSubresource(w http.ResponseWriter, r *http.Request, store *ResourceStore, namespace, name string) {
	if r.Method != http.MethodPost {
		writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported for "+store.resource+"/binding")
		return
	}

	ctx := r.Context()

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
	binding, ok := bindingObj.(*corev1.Binding)
	if !ok {
		writeStatusError(w, http.StatusBadRequest, "BadRequest", fmt.Sprintf("request body is %T, expected Binding", bindingObj))
		return
	}

	currentObj, err := store.Get(ctx, namespace, name)
	if err != nil {
		writeResourceError(w, err, store.resource, name)
		return
	}
	pod, ok := currentObj.(*corev1.Pod)
	if !ok {
		writeInternalError(w, fmt.Errorf("%s store returned %T, expected *corev1.Pod", store.resource, currentObj))
		return
	}

	pod.Spec.NodeName = binding.Target.Name

	_, err = store.Update(ctx, namespace, name, pod)
	if err != nil {
		writeResourceError(w, err, store.resource, name)
		return
	}

	writeRuntimeObject(w, http.StatusCreated, binding)
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

// handleScaleSubresource implements GET/PUT/PATCH for a resource's /scale
// subresource, generically across every type in apidef.Table that declares
// one (deployments, replicasets, statefulsets): all three share the same
// Spec.Replicas *int32 / Status.Replicas int32 / Spec.Selector
// *metav1.LabelSelector shape, so this is one reflection-based handler
// (scaleFromObject/applyReplicasToObject below) instead of one hand-copied
// ScaleREST per resource, the same generalization copyStatus already applies
// to /status above.
func handleScaleSubresource(w http.ResponseWriter, r *http.Request, store *ResourceStore, namespace, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		obj, err := store.Get(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}
		scale, err := scaleFromObject(obj)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writeRuntimeObject(w, http.StatusOK, scale)

	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		// decodeBody (not a plain json.Unmarshal) because client-go's
		// generated UpdateScale calls UseProtobufAsDefault(), so real
		// clients (kubectl scale included) PUT this body as protobuf, not
		// JSON -- found by TestScaleSubresource against the real typed
		// client, which a hand-rolled JSON-only fixture wouldn't have
		// caught. decodeBody's UniversalDeserializer auto-detects either.
		incomingObj, err := decodeBody(body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to decode request body: "+err.Error())
			return
		}
		incoming, ok := incomingObj.(*autoscalingv1.Scale)
		if !ok {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", fmt.Sprintf("request body is %T, expected Scale", incomingObj))
			return
		}

		current, err := store.Get(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}
		if err := applyReplicasToObject(current, incoming.Spec.Replicas); err != nil {
			writeInternalError(w, err)
			return
		}

		updated, err := store.Update(ctx, namespace, name, current)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}
		scale, err := scaleFromObject(updated)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writeRuntimeObject(w, http.StatusOK, scale)

	case http.MethodPatch:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "failed to read request body")
			return
		}
		defer r.Body.Close()

		current, err := store.Get(ctx, namespace, name)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}
		currentScale, err := scaleFromObject(current)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		// Scale isn't stored as its own object -- there's nothing for the
		// generic applyPatch (which round-trips through the *stored*
		// resource's Encode/Decode) to patch. Patch the derived Scale's own
		// JSON representation instead, then fold just .spec.replicas back
		// onto the real object, the same two-step PUT does above.
		currentJSON, err := json.Marshal(currentScale)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		var patchedJSON []byte
		switch r.Header.Get("Content-Type") {
		case "application/merge-patch+json":
			patchedJSON, err = jsonpatch.MergePatch(currentJSON, body)
		case "application/strategic-merge-patch+json":
			patchedJSON, err = strategicpatch.StrategicMergePatch(currentJSON, body, &autoscalingv1.Scale{})
		case "application/json-patch+json":
			var patch jsonpatch.Patch
			patch, err = jsonpatch.DecodePatch(body)
			if err == nil {
				patchedJSON, err = patch.Apply(currentJSON)
			}
		default:
			err = fmt.Errorf("unsupported patch type")
		}
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "patch failed: "+err.Error())
			return
		}

		var patchedScale autoscalingv1.Scale
		if err := json.Unmarshal(patchedJSON, &patchedScale); err != nil {
			writeInternalError(w, err)
			return
		}
		if err := applyReplicasToObject(current, patchedScale.Spec.Replicas); err != nil {
			writeInternalError(w, err)
			return
		}

		updated, err := store.Update(ctx, namespace, name, current)
		if err != nil {
			writeResourceError(w, err, store.resource, name)
			return
		}
		scale, err := scaleFromObject(updated)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writeRuntimeObject(w, http.StatusOK, scale)

	default:
		writeStatusError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method "+r.Method+" is not supported for "+store.resource+"/scale")
	}
}

// scaleFromObject builds an autoscaling/v1.Scale view of obj (a Deployment,
// ReplicaSet, or StatefulSet) via reflection on the Spec.Replicas/
// Status.Replicas/Spec.Selector fields every one of those types shares,
// matching upstream's per-type convertToScale (e.g.
// pkg/registry/apps/deployment/storage/storage.go) without a copy of it per
// type.
func scaleFromObject(obj runtime.Object) (*autoscalingv1.Scale, error) {
	objMeta := getObjectMeta(obj)
	if objMeta == nil {
		return nil, fmt.Errorf("%T does not implement ObjectMetaAccessor", obj)
	}

	v := reflect.ValueOf(obj).Elem()
	specVal := v.FieldByName("Spec")
	statusVal := v.FieldByName("Status")
	if !specVal.IsValid() || !statusVal.IsValid() {
		return nil, fmt.Errorf("%T has no Spec/Status field", obj)
	}

	var specReplicas int32
	if replicasField := specVal.FieldByName("Replicas"); replicasField.IsValid() && !replicasField.IsNil() {
		specReplicas = int32(replicasField.Elem().Int())
	}

	var statusReplicas int32
	if replicasField := statusVal.FieldByName("Replicas"); replicasField.IsValid() {
		statusReplicas = int32(replicasField.Int())
	}

	var selectorStr string
	if selectorField := specVal.FieldByName("Selector"); selectorField.IsValid() && !selectorField.IsNil() {
		if selector, ok := selectorField.Interface().(*metav1.LabelSelector); ok {
			if s, err := metav1.LabelSelectorAsSelector(selector); err == nil {
				selectorStr = s.String()
			}
		}
	}

	return &autoscalingv1.Scale{
		// Encode (scheme.go) is a bare json.Marshal underneath -- it does
		// not consult Scheme to fill in TypeMeta the way a full versioning
		// codec would, so every response this apiserver writes is
		// responsible for setting its own kind/apiVersion if it needs one.
		// Most callers get away without it (typed client-go Get/List calls
		// already know their expected type and never look at the response's
		// kind), but kubectl's `scale` command decodes this specific
		// response with a kind-aware (effectively dynamic/unstructured)
		// client and hard-errors with "Object 'Kind' is missing" without
		// this -- found via real kubectl, not just client-go, against this
		// handler.
		TypeMeta: metav1.TypeMeta{Kind: "Scale", APIVersion: "autoscaling/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              objMeta.Name,
			Namespace:         objMeta.Namespace,
			UID:               objMeta.UID,
			ResourceVersion:   objMeta.ResourceVersion,
			CreationTimestamp: objMeta.CreationTimestamp,
		},
		Spec:   autoscalingv1.ScaleSpec{Replicas: specReplicas},
		Status: autoscalingv1.ScaleStatus{Replicas: statusReplicas, Selector: selectorStr},
	}, nil
}

// applyReplicasToObject sets obj's Spec.Replicas (a Deployment, ReplicaSet,
// or StatefulSet) to replicas, in place. The counterpart write half of
// scaleFromObject above.
func applyReplicasToObject(obj runtime.Object, replicas int32) error {
	v := reflect.ValueOf(obj).Elem()
	specVal := v.FieldByName("Spec")
	if !specVal.IsValid() {
		return fmt.Errorf("%T has no Spec field", obj)
	}
	replicasField := specVal.FieldByName("Replicas")
	if !replicasField.IsValid() || replicasField.Type() != reflect.TypeOf((*int32)(nil)) {
		return fmt.Errorf("%T.Spec.Replicas is not *int32", obj)
	}
	replicasField.Set(reflect.ValueOf(&replicas))
	return nil
}
