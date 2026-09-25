package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/kubernetes/pkg/auth/nodeidentifier"
)

const (
	mirrorPodAnnotationKey = "kubernetes.io/config.mirror"
	nodeLeaseNamespace     = "kube-node-lease"
)

func applyNodeRestriction(ctx context.Context, s *store, req *admit.Request) error {
	nodeName, isNode := nodeidentifier.NewDefaultNodeIdentifier().NodeIdentity(&user.DefaultInfo{
		Name:   req.User.Username,
		UID:    req.User.UID,
		Groups: req.User.Groups,
		Extra:  req.User.Extra,
	})
	if !isNode {
		return nil
	}
	if nodeName == "" {
		return fmt.Errorf("could not determine node from user %q", req.User.Username)
	}
	switch req.Resource.Resource {
	case "nodes":
		return restrictNodeObject(nodeName, req)
	case "pods":
		return restrictNodePod(ctx, s, nodeName, req)
	case "leases":
		return restrictNodeLease(nodeName, req)
	case "csinodes":
		return restrictCSINode(nodeName, req)
	case "certificatesigningrequests":
		return restrictNodeCSR(nodeName, req)
	case "persistentvolumeclaims":
		return restrictNodePVC(nodeName, req)
	case "resourceslices":
		return restrictResourceSlice(nodeName, req)
	case "serviceaccounts":
		return restrictNodeToken(ctx, s, nodeName, req)
	default:
		return nil
	}
}

func restrictNodeObject(nodeName string, req *admit.Request) error {
	name := req.Name
	if name == "" {
		name = objectMetaName(req.Object)
	}
	if name != "" && name != nodeName {
		return fmt.Errorf("node %q is not allowed to modify node %q", nodeName, name)
	}
	if req.Subresource != "" && req.Subresource != "status" {
		return nil
	}
	switch req.Operation {
	case "CREATE":
		if nodeSpecField(req.Object, "configSource") != nil {
			return fmt.Errorf("node %q is not allowed to create pods with a non-nil configSource", nodeName)
		}
		if forbidden := forbiddenNodeLabels(objectLabels(req.Object, nil), nil); len(forbidden) > 0 {
			return fmt.Errorf("node %q is not allowed to set the following labels: %s", nodeName, strings.Join(forbidden, ", "))
		}
	case "UPDATE":
		if src := nodeSpecField(req.Object, "configSource"); src != nil && !reflect.DeepEqual(src, nodeSpecField(req.OldObject, "configSource")) {
			return fmt.Errorf("node %q is not allowed to update configSource to a new non-nil configSource", nodeName)
		}
		if !reflect.DeepEqual(nodeSpecTaints(req.OldObject), nodeSpecTaints(req.Object)) {
			return fmt.Errorf("node %q is not allowed to modify taints", nodeName)
		}
		if !reflect.DeepEqual(objectOwnerRefs(req.OldObject), objectOwnerRefs(req.Object)) {
			return fmt.Errorf("node %q is not allowed to modify ownerReferences", nodeName)
		}
		if forbidden := forbiddenNodeLabels(objectLabels(req.Object, nil), objectLabels(req.OldObject, nil)); len(forbidden) > 0 {
			return fmt.Errorf("is not allowed to modify labels: %s", strings.Join(forbidden, ", "))
		}
	}
	return nil
}

func nodeSpecTaints(obj map[string]any) any {
	return nodeSpecField(obj, "taints")
}

func objectOwnerRefs(obj map[string]any) any {
	if obj == nil {
		return nil
	}
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		return nil
	}
	return meta["ownerReferences"]
}

func nodeSpecField(obj map[string]any, key string) any {
	if obj == nil {
		return nil
	}
	spec, _ := obj["spec"].(map[string]any)
	return spec[key]
}

func forbiddenNodeLabels(now, old map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	for k, v := range now {
		if old[k] != v && isForbiddenNodeLabel(k) && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for k, v := range old {
		if now[k] != v && isForbiddenNodeLabel(k) && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func isForbiddenNodeLabel(key string) bool {
	ns := labelNamespace(key)
	if ns == corev1.LabelNamespaceNodeRestriction || strings.HasSuffix(ns, "."+corev1.LabelNamespaceNodeRestriction) {
		return true
	}
	return isKubernetesLabel(key) && !isKubeletLabel(key)
}

func labelNamespace(key string) string {
	if i := strings.Index(key, "/"); i >= 0 {
		return key[:i]
	}
	return ""
}

func isKubernetesLabel(key string) bool {
	ns := labelNamespace(key)
	return ns == "kubernetes.io" || strings.HasSuffix(ns, ".kubernetes.io") || ns == "k8s.io" || strings.HasSuffix(ns, ".k8s.io")
}

func isKubeletLabel(key string) bool {
	ns := labelNamespace(key)
	if ns == corev1.LabelNamespaceSuffixKubelet || ns == corev1.LabelNamespaceSuffixNode ||
		strings.HasSuffix(ns, "."+corev1.LabelNamespaceSuffixKubelet) ||
		strings.HasSuffix(ns, "."+corev1.LabelNamespaceSuffixNode) {
		return true
	}
	switch key {
	case 		corev1.LabelHostname, corev1.LabelTopologyZone, corev1.LabelTopologyRegion,
		corev1.LabelInstanceTypeStable, corev1.LabelOSStable, corev1.LabelArchStable,
		"beta.kubernetes.io/os", "beta.kubernetes.io/arch",
		corev1.LabelWindowsBuild, corev1.LabelFailureDomainBetaZone,
		corev1.LabelFailureDomainBetaRegion, corev1.LabelInstanceType:
		return true
	default:
		return false
	}
}

func restrictNodePod(ctx context.Context, s *store, nodeName string, req *admit.Request) error {
	if req.Subresource == "eviction" {
		return restrictNodeEviction(ctx, s, nodeName, req)
	}
	if req.Subresource == "status" {
		if req.Operation == "CREATE" {
			return nil
		}
		bound := objectNodeName(req.OldObject)
		if bound == "" {
			bound = objectNodeName(req.Object)
		}
		if bound != nodeName {
			return fmt.Errorf("node %q can only update pod status for pods with spec.nodeName set to itself", nodeName)
		}
		return nil
	}
	if req.Subresource != "" {
		return fmt.Errorf("unexpected pod subresource %q, only 'status' and 'eviction' are allowed", req.Subresource)
	}
	if req.Operation == "DELETE" {
		return restrictNodePodDelete(ctx, s, nodeName, req)
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return fmt.Errorf("unexpected operation %q, node %q can only create and delete mirror pods", req.Operation, nodeName)
	}
	if req.Object == nil {
		return fmt.Errorf("pod does not have %q annotation, node %q can only create mirror pods", mirrorPodAnnotationKey, nodeName)
	}
	meta, _ := req.Object["metadata"].(map[string]any)
	ann, _ := meta["annotations"].(map[string]any)
	if _, ok := ann[mirrorPodAnnotationKey]; !ok {
		return fmt.Errorf("pod does not have %q annotation, node %q can only create mirror pods", mirrorPodAnnotationKey, nodeName)
	}
	if objectNodeName(req.Object) != nodeName {
		return fmt.Errorf("node %q can only create pods with spec.nodeName set to itself", nodeName)
	}
	return nil
}

func restrictNodePodDelete(ctx context.Context, s *store, nodeName string, req *admit.Request) error {
	if s == nil {
		return fmt.Errorf("node %q can only delete pods with spec.nodeName set to itself", nodeName)
	}
	pod, ok, err := s.pod(ctx, req.Namespace, req.Name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("pods %q not found", req.Name)
	}
	if pod.Spec.NodeName != nodeName {
		return fmt.Errorf("node %q can only delete pods with spec.nodeName set to itself", nodeName)
	}
	return nil
}

func objectNodeName(obj map[string]any) string {
	spec, _ := obj["spec"].(map[string]any)
	name, _ := spec["nodeName"].(string)
	return name
}

func restrictNodeLease(nodeName string, req *admit.Request) error {
	if req.Namespace != nodeLeaseNamespace {
		return fmt.Errorf("can only access leases in the %q system namespace", nodeLeaseNamespace)
	}
	name := req.Name
	if req.Operation == "CREATE" {
		name = objectMetaName(req.Object)
	}
	if name != nodeName {
		return fmt.Errorf("can only access node lease with the same name as the requesting node")
	}
	return nil
}

func restrictCSINode(nodeName string, req *admit.Request) error {
	name := req.Name
	if req.Operation == "CREATE" {
		name = objectMetaName(req.Object)
	}
	if name != nodeName {
		return fmt.Errorf("can only access CSINode with the same name as the requesting node")
	}
	return nil
}

func objectMetaName(obj map[string]any) string {
	meta, _ := obj["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	return name
}

func restrictNodePVC(nodeName string, req *admit.Request) error {
	if req.Subresource != "status" {
		return fmt.Errorf("may only update PVC status")
	}
	if req.Operation != "" && req.Operation != "UPDATE" {
		return fmt.Errorf("unexpected operation %q", req.Operation)
	}
	if !pvcStatusOnly(req.OldObject, req.Object) {
		return fmt.Errorf("node %q is not allowed to update fields other than status.quantity and status.conditions", nodeName)
	}
	return nil
}

func pvcStatusOnly(oldObj, newObj map[string]any) bool {
	return reflect.DeepEqual(stripPVCStatusExtras(oldObj), stripPVCStatusExtras(newObj))
}

func stripPVCStatusExtras(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}
	var clone map[string]any
	raw, err := json.Marshal(obj)
	if err != nil || json.Unmarshal(raw, &clone) != nil {
		return obj
	}
	if meta, ok := clone["metadata"].(map[string]any); ok {
		delete(meta, "resourceVersion")
		delete(meta, "managedFields")
	}
	if st, ok := clone["status"].(map[string]any); ok {
		delete(st, "capacity")
		delete(st, "conditions")
		delete(st, "allocatedResourceStatuses")
		delete(st, "allocatedResources")
	}
	return clone
}

func restrictResourceSlice(nodeName string, req *admit.Request) error {
	switch req.Operation {
	case "", "CREATE":
		return resourceSliceNodeName(nodeName, req.Object, "create")
	case "DELETE":
		return resourceSliceNodeName(nodeName, req.OldObject, "delete")
	case "UPDATE", "PATCH":
		if err := resourceSliceNodeName(nodeName, req.OldObject, "update"); err != nil {
			return err
		}
		return resourceSliceNodeName(nodeName, req.Object, "update")
	default:
		return nil
	}
}

func resourceSliceNodeName(nodeName string, obj map[string]any, verb string) error {
	spec, _ := obj["spec"].(map[string]any)
	got, _ := spec["nodeName"].(string)
	if got == nodeName {
		return nil
	}
	return fmt.Errorf("can only %s ResourceSlice with the same NodeName as the requesting node", verb)
}

func restrictNodeEviction(ctx context.Context, s *store, nodeName string, req *admit.Request) error {
	if req.Operation != "" && req.Operation != "CREATE" {
		return fmt.Errorf("unexpected operation %s", req.Operation)
	}
	name := req.Name
	if name == "" {
		name = objectMetaName(req.Object)
	}
	if name == "" {
		return fmt.Errorf("could not determine pod from request data")
	}
	if s == nil {
		return fmt.Errorf("node %s can only evict pods with spec.nodeName set to itself", nodeName)
	}
	pod, ok, err := s.pod(ctx, req.Namespace, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("pods %q not found", name)
	}
	if pod.Spec.NodeName != nodeName {
		return fmt.Errorf("node %s can only evict pods with spec.nodeName set to itself", nodeName)
	}
	return nil
}

func restrictNodeToken(ctx context.Context, s *store, nodeName string, req *admit.Request) error {
	if req.Subresource != "token" {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	ref, _ := spec["boundObjectRef"].(map[string]any)
	if ref == nil {
		return fmt.Errorf("node requested token not bound to a pod")
	}
	apiVersion, _ := ref["apiVersion"].(string)
	kind, _ := ref["kind"].(string)
	name, _ := ref["name"].(string)
	if apiVersion != "v1" || kind != "Pod" || name == "" {
		return fmt.Errorf("node requested token not bound to a pod")
	}
	uid, _ := ref["uid"].(string)
	if uid == "" {
		return fmt.Errorf("node requested token with a pod binding without a uid")
	}
	if s == nil {
		return fmt.Errorf("node requested token bound to a pod scheduled on a different node")
	}
	pod, ok, err := s.pod(ctx, req.Namespace, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("pods %q not found", name)
	}
	if string(pod.UID) != uid {
		return fmt.Errorf("the UID in the bound object reference (%s) does not match the UID in record. The object might have been deleted and then recreated", uid)
	}
	if pod.Spec.NodeName != nodeName {
		return fmt.Errorf("node requested token bound to a pod scheduled on a different node")
	}
	return nil
}

func restrictNodeCSR(nodeName string, req *admit.Request) error {
	if req.Subresource != "" {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	if req.Object == nil {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	signer, _ := spec["signerName"].(string)
	if signer != certificatesv1.KubeletServingSignerName && signer != certificatesv1.KubeAPIServerClientKubeletSignerName {
		return nil
	}
	csr, err := parseCSRRequest(spec["request"])
	if err != nil {
		return fmt.Errorf("unable to parse csr: %w", err)
	}
	want := "system:node:" + nodeName
	if csr.Subject.CommonName != want {
		return fmt.Errorf("can only create a node CSR with CN=%s", want)
	}
	return nil
}
