package admission

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestNodeRestrictionDeniesNonMirrorPodCreate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, nodePodReq("k8flare-c1", "", false))
	if out.Allowed {
		t.Fatal("expected node non-mirror pod deny")
	}
	if !strings.Contains(out.Message, `pod does not have "kubernetes.io/config.mirror" annotation, node "k8flare-c1" can only create mirror pods`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionAllowsMirrorPodOnSelf(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, nodePodReq("k8flare-c1", "k8flare-c1", true))
	if !out.Allowed {
		t.Fatalf("mirror pod on self denied: %+v", out)
	}
}

func TestNodeRestrictionDeniesOtherNode(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	req.Kind = schema.GroupVersionKind{Version: "v1", Kind: "Node"}
	req.Name = "k8flare-agent"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.Object = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-agent"}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected other-node deny")
	}
	if !strings.Contains(out.Message, `node "k8flare-c1" is not allowed to modify node "k8flare-agent"`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesForeignPodStatus(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "k8flare-agent", false)
	req.Subresource = "status"
	req.Operation = "UPDATE"
	req.OldObject = req.Object
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected foreign pod status deny")
	}
	if !strings.Contains(out.Message, `node "k8flare-c1" can only update pod status for pods with spec.nodeName set to itself`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesOtherNodeLease(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}
	req.Kind = schema.GroupVersionKind{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"}
	req.Name = "k8flare-agent"
	req.Namespace = "kube-node-lease"
	req.Operation = "UPDATE"
	req.Object = map[string]any{"apiVersion": "coordination.k8s.io/v1", "kind": "Lease", "metadata": map[string]any{"name": "k8flare-agent", "namespace": "kube-node-lease"}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected other-node lease deny")
	}
	if !strings.Contains(out.Message, "can only access node lease with the same name as the requesting node") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesLeaseOutsideNodeLeaseNS(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}
	req.Kind = schema.GroupVersionKind{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"}
	req.Name = "k8flare-c1"
	req.Namespace = "default"
	req.Operation = "UPDATE"
	req.Object = map[string]any{"apiVersion": "coordination.k8s.io/v1", "kind": "Lease", "metadata": map[string]any{"name": "k8flare-c1", "namespace": "default"}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected lease namespace deny")
	}
	if !strings.Contains(out.Message, `can only access leases in the "kube-node-lease" system namespace`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesOtherCSINode(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "csinodes"}
	req.Kind = schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "CSINode"}
	req.Name = "k8flare-agent"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.Object = map[string]any{"apiVersion": "storage.k8s.io/v1", "kind": "CSINode", "metadata": map[string]any{"name": "k8flare-agent"}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected other CSINode deny")
	}
	if !strings.Contains(out.Message, "can only access CSINode with the same name as the requesting node") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesWrongCNCSR(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := csrAdmitReq("wrong-cn", certificatesv1.KubeAPIServerClientKubeletSignerName, pemWithCN("system:node:k8flare-agent"))
	req.User = admit.User{Username: "system:node:k8flare-c1", Groups: []string{user.NodesGroup, user.AllAuthenticated}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected wrong CN deny")
	}
	if !strings.Contains(out.Message, "can only create a node CSR with CN=system:node:k8flare-c1") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionAllowsOwnCNCSR(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := csrAdmitReq("own-cn", certificatesv1.KubeletServingSignerName, pemWithCN("system:node:k8flare-c1"))
	req.User = admit.User{Username: "system:node:k8flare-c1", Groups: []string{user.NodesGroup, user.AllAuthenticated}}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("own CN denied: %+v", out)
	}
}

func TestNodeRestrictionDeniesPVCNonStatus(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePVCReq("k8flare-c1")
	req.Subresource = ""
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected non-status PVC deny")
	}
	if !strings.Contains(out.Message, "may only update PVC status") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesPVCExtraFields(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePVCReq("k8flare-c1")
	meta, _ := req.Object["metadata"].(map[string]any)
	meta["labels"] = map[string]any{"k8flare.com/probe": "1"}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected extra field deny")
	}
	if !strings.Contains(out.Message, `node "k8flare-c1" is not allowed to update fields other than status.quantity and status.conditions`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionAllowsPVCCapacity(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePVCReq("k8flare-c1")
	st, _ := req.Object["status"].(map[string]any)
	st["capacity"] = map[string]any{"storage": "2Gi"}
	st["conditions"] = []any{map[string]any{"type": "FileSystemResizePending", "status": "True"}}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("capacity update denied: %+v", out)
	}
}

func TestNodeRestrictionDeniesOtherResourceSlice(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceslices"}
	req.Kind = schema.GroupVersionKind{Group: "resource.k8s.io", Version: "v1", Kind: "ResourceSlice"}
	req.Name = "probe"
	req.Namespace = ""
	req.Operation = "CREATE"
	req.Object = map[string]any{
		"apiVersion": "resource.k8s.io/v1",
		"kind":       "ResourceSlice",
		"metadata":   map[string]any{"name": "probe"},
		"spec":       map[string]any{"driver": "k8flare.com/probe", "nodeName": "k8flare-agent", "pool": map[string]any{"name": "p", "resourceSliceCount": 1}},
	}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected other ResourceSlice deny")
	}
	if !strings.Contains(out.Message, "can only create ResourceSlice with the same NodeName as the requesting node") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionAllowsOwnResourceSlice(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceslices"}
	req.Kind = schema.GroupVersionKind{Group: "resource.k8s.io", Version: "v1", Kind: "ResourceSlice"}
	req.Name = "probe"
	req.Namespace = ""
	req.Operation = "CREATE"
	req.Object = map[string]any{
		"apiVersion": "resource.k8s.io/v1",
		"kind":       "ResourceSlice",
		"metadata":   map[string]any{"name": "probe"},
		"spec":       map[string]any{"driver": "k8flare.com/probe", "nodeName": "k8flare-c1", "pool": map[string]any{"name": "p", "resourceSliceCount": 1}},
	}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("own ResourceSlice denied: %+v", out)
	}
}

func TestNodeRestrictionDeniesResourceSliceUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceslices"}
	req.Kind = schema.GroupVersionKind{Group: "resource.k8s.io", Version: "v1", Kind: "ResourceSlice"}
	req.Name = "probe"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.OldObject = map[string]any{
		"apiVersion": "resource.k8s.io/v1",
		"kind":       "ResourceSlice",
		"metadata":   map[string]any{"name": "probe"},
		"spec":       map[string]any{"driver": "k8flare.com/probe", "nodeName": "k8flare-agent"},
	}
	req.Object = map[string]any{
		"apiVersion": "resource.k8s.io/v1",
		"kind":       "ResourceSlice",
		"metadata":   map[string]any{"name": "probe"},
		"spec":       map[string]any{"driver": "k8flare.com/probe", "nodeName": "k8flare-agent"},
	}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected other ResourceSlice update deny")
	}
	if !strings.Contains(out.Message, "can only update ResourceSlice with the same NodeName as the requesting node") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesUnboundToken(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, nodeTokenReq("k8flare-c1", nil))
	if out.Allowed {
		t.Fatal("expected unbound token deny")
	}
	if !strings.Contains(out.Message, "node requested token not bound to a pod") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesTokenWithoutUID(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, nodeTokenReq("k8flare-c1", map[string]any{"apiVersion": "v1", "kind": "Pod", "name": "user"}))
	if out.Allowed {
		t.Fatal("expected missing uid deny")
	}
	if !strings.Contains(out.Message, "node requested token with a pod binding without a uid") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesTokenOnOtherNode(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/pods/default/user": mustJSON(t, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default", UID: "uid-1"},
			Spec:       corev1.PodSpec{NodeName: "k8flare-agent"},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, nodeTokenReq("k8flare-c1", map[string]any{"apiVersion": "v1", "kind": "Pod", "name": "user", "uid": "uid-1"}))
	if out.Allowed {
		t.Fatal("expected other-node token deny")
	}
	if !strings.Contains(out.Message, "node requested token bound to a pod scheduled on a different node") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesForeignEviction(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/pods/default/user": mustJSON(t, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "k8flare-agent"},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "k8flare-agent", false)
	req.Name = "user"
	req.Subresource = "eviction"
	req.Operation = "CREATE"
	req.Kind = schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "Eviction"}
	req.Object = map[string]any{"apiVersion": "policy/v1", "kind": "Eviction", "metadata": map[string]any{"name": "user", "namespace": "default"}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected foreign eviction deny")
	}
	if !strings.Contains(out.Message, "node k8flare-c1 can only evict pods with spec.nodeName set to itself") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionAllowsOwnEviction(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/pods/default/user": mustJSON(t, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "k8flare-c1"},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "k8flare-c1", false)
	req.Name = "user"
	req.Subresource = "eviction"
	req.Operation = "CREATE"
	req.Kind = schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "Eviction"}
	req.Object = map[string]any{"apiVersion": "policy/v1", "kind": "Eviction", "metadata": map[string]any{"name": "user", "namespace": "default"}}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("own eviction denied: %+v", out)
	}
}

func TestNodeRestrictionDeniesNodeRestrictionLabel(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	req.Kind = schema.GroupVersionKind{Version: "v1", Kind: "Node"}
	req.Name = "k8flare-c1"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.OldObject = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1"}}
	req.Object = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1", "labels": map[string]any{"node-restriction.kubernetes.io/foo": "1"}}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected restricted label deny")
	}
	if !strings.Contains(out.Message, "is not allowed to modify labels: node-restriction.kubernetes.io/foo") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesUnknownKubernetesLabel(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	req.Kind = schema.GroupVersionKind{Version: "v1", Kind: "Node"}
	req.Name = "k8flare-c1"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.OldObject = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1"}}
	req.Object = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1", "labels": map[string]any{"kubernetes.io/foo": "1"}}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected unknown kubernetes.io label deny")
	}
	if !strings.Contains(out.Message, "is not allowed to modify labels: kubernetes.io/foo") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesTaintUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	req.Kind = schema.GroupVersionKind{Version: "v1", Kind: "Node"}
	req.Name = "k8flare-c1"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.OldObject = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1"}, "spec": map[string]any{}}
	req.Object = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1"}, "spec": map[string]any{"taints": []any{map[string]any{"key": "k8flare.com/probe", "effect": "NoSchedule"}}}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected taint deny")
	}
	if !strings.Contains(out.Message, `node "k8flare-c1" is not allowed to modify taints`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesConfigSourceUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	req.Kind = schema.GroupVersionKind{Version: "v1", Kind: "Node"}
	req.Name = "k8flare-c1"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.OldObject = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1"}, "spec": map[string]any{}}
	req.Object = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1"}, "spec": map[string]any{"configSource": map[string]any{"configMap": map[string]any{"name": "cfg", "namespace": "kube-system"}}}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected configSource deny")
	}
	if !strings.Contains(out.Message, `node "k8flare-c1" is not allowed to update configSource to a new non-nil configSource`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesOwnerReferences(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.Resource = schema.GroupVersionResource{Version: "v1", Resource: "nodes"}
	req.Kind = schema.GroupVersionKind{Version: "v1", Kind: "Node"}
	req.Name = "k8flare-c1"
	req.Namespace = ""
	req.Operation = "UPDATE"
	req.OldObject = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1"}}
	req.Object = map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": map[string]any{"name": "k8flare-c1", "ownerReferences": []any{map[string]any{"apiVersion": "v1", "kind": "Node", "name": "other", "uid": "x"}}}}
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected ownerReferences deny")
	}
	if !strings.Contains(out.Message, `node "k8flare-c1" is not allowed to modify ownerReferences`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesPodUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "k8flare-c1", true)
	req.Operation = "UPDATE"
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected pod update deny")
	}
	if !strings.Contains(out.Message, `unexpected operation "UPDATE", node "k8flare-c1" can only create and delete mirror pods`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionDeniesForeignPodDelete(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/pods/default/p": mustJSON(t, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "k8flare-agent"},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "k8flare-agent", false)
	req.Operation = "DELETE"
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected foreign delete deny")
	}
	if !strings.Contains(out.Message, `node "k8flare-c1" can only delete pods with spec.nodeName set to itself`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestNodeRestrictionIgnoresNonNode(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := nodePodReq("k8flare-c1", "", false)
	req.User = admit.User{Username: "admin", Groups: []string{"system:masters", "system:authenticated"}}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("admin pod create denied: %+v", out)
	}
}

func nodePodReq(node, nodeName string, mirror bool) admit.Request {
	meta := map[string]any{"name": "p", "namespace": "default"}
	if mirror {
		meta["annotations"] = map[string]any{mirrorPodAnnotationKey: "1"}
	}
	spec := map[string]any{"serviceAccountName": "default", "containers": []any{map[string]any{"name": "c", "image": "img"}}}
	if nodeName != "" {
		spec["nodeName"] = nodeName
	}
	return admit.Request{
		Phase:     "validate",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		User:      admit.User{Username: "system:node:" + node, Groups: []string{user.NodesGroup, user.AllAuthenticated}},
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": meta,
			"spec":     spec,
		},
	}
}

func nodeTokenReq(node string, ref map[string]any) admit.Request {
	spec := map[string]any{}
	if ref != nil {
		spec["boundObjectRef"] = ref
	}
	return admit.Request{
		Phase:       "validate",
		Name:        "default",
		Namespace:   "default",
		Resource:    schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"},
		Kind:        schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"},
		Subresource: "token",
		Operation:   "CREATE",
		User:        admit.User{Username: "system:node:" + node, Groups: []string{user.NodesGroup, user.AllAuthenticated}},
		Object: map[string]any{
			"apiVersion": "authentication.k8s.io/v1",
			"kind":       "TokenRequest",
			"spec":       spec,
		},
	}
}

func nodePVCReq(node string) admit.Request {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata":   map[string]any{"name": "disk", "namespace": "default", "resourceVersion": "1"},
		"spec":       map[string]any{"accessModes": []any{"ReadWriteOnce"}},
		"status":     map[string]any{"phase": "Bound", "capacity": map[string]any{"storage": "1Gi"}},
	}
	old := map[string]any{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata":   map[string]any{"name": "disk", "namespace": "default", "resourceVersion": "1"},
		"spec":       map[string]any{"accessModes": []any{"ReadWriteOnce"}},
		"status":     map[string]any{"phase": "Bound", "capacity": map[string]any{"storage": "1Gi"}},
	}
	return admit.Request{
		Phase:       "validate",
		Name:        "disk",
		Namespace:   "default",
		Resource:    schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"},
		Kind:        schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"},
		Subresource: "status",
		Operation:   "UPDATE",
		User:        admit.User{Username: "system:node:" + node, Groups: []string{user.NodesGroup, user.AllAuthenticated}},
		Object:      obj,
		OldObject:   old,
	}
}

func pemWithCN(cn string) string {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: cn},
	}, key)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}
