package authz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	resourcev1 "k8s.io/api/resource/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func TestNodeAuthorizerIsolation(t *testing.T) {
	n := &nodeAuthorizer{ident: nodeidentifierStub{}, rules: nil}
	node := &user.DefaultInfo{Name: "system:node:k8flare-c1", Groups: []string{user.NodesGroup}}
	admin := &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup}}

	if d, _, _ := n.Authorize(context.Background(), &authorizer.AttributesRecord{User: admin, ResourceRequest: true, Resource: "nodes", Verb: "get", Name: "other"}); d != authorizer.DecisionNoOpinion {
		t.Fatalf("admin: %v", d)
	}

	own := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "nodes", Verb: "get", Name: "k8flare-c1", APIGroup: ""}
	if d, _, _ := n.Authorize(context.Background(), own); d != authorizer.DecisionAllow {
		t.Fatalf("own node: %v", d)
	}
	other := *own
	other.Name = "other"
	if d, _, _ := n.Authorize(context.Background(), &other); d != authorizer.DecisionNoOpinion {
		t.Fatalf("other node: %v", d)
	}

	status := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "nodes", Subresource: "status", Verb: "patch", Name: "k8flare-c1"}
	if d, _, _ := n.Authorize(context.Background(), status); d != authorizer.DecisionAllow {
		t.Fatalf("own status: %v", d)
	}

	lease := &authorizer.AttributesRecord{User: node, ResourceRequest: true, APIGroup: "coordination.k8s.io", Resource: "leases", Verb: "update", Namespace: "kube-node-lease", Name: "k8flare-c1"}
	if d, _, _ := n.Authorize(context.Background(), lease); d != authorizer.DecisionAllow {
		t.Fatalf("own lease: %v", d)
	}
	leaseDelete := *lease
	leaseDelete.Verb = "delete"
	if d, _, _ := n.Authorize(context.Background(), &leaseDelete); d != authorizer.DecisionAllow {
		t.Fatalf("own lease delete: %v", d)
	}
	lease.Name = "other"
	if d, _, _ := n.Authorize(context.Background(), lease); d != authorizer.DecisionNoOpinion {
		t.Fatalf("other lease: %v", d)
	}
	leaseList := &authorizer.AttributesRecord{User: node, ResourceRequest: true, APIGroup: "coordination.k8s.io", Resource: "leases", Verb: "list", Namespace: "kube-node-lease"}
	if d, reason, _ := n.Authorize(context.Background(), leaseList); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("unscoped lease list: %v %q", d, reason)
	}

	listAll := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "pods", Verb: "list", Namespace: "default"}
	if d, reason, _ := n.Authorize(context.Background(), listAll); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("unscoped list: %v %q", d, reason)
	}
	sel, _ := fields.ParseSelector("spec.nodeName=k8flare-c1")
	listMine := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "pods", Verb: "list", Namespace: "default", FieldSelectorRequirements: sel.Requirements()}
	if d, _, _ := n.Authorize(context.Background(), listMine); d != authorizer.DecisionAllow {
		t.Fatalf("scoped list: %v", d)
	}

	secretSel, _ := fields.ParseSelector("metadata.name=sample-webhook-secret")
	secretList := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "secrets", Verb: "list", Namespace: "webhook-7325", FieldSelectorRequirements: secretSel.Requirements()}
	if d, _, _ := n.Authorize(context.Background(), secretList); d != authorizer.DecisionNoOpinion {
		t.Fatalf("named secret without graph: %v", d)
	}
	saList := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "serviceaccounts", Verb: "list", Namespace: "default"}
	if d, reason, _ := n.Authorize(context.Background(), saList); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("serviceaccount list: %v %q", d, reason)
	}
	claimList := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "resourceclaims", Verb: "list", Namespace: "default", APIGroup: "resource.k8s.io"}
	if d, reason, _ := n.Authorize(context.Background(), claimList); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("resourceclaim list: %v %q", d, reason)
	}
	allSecrets := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "secrets", Verb: "list", Namespace: "webhook-7325"}
	if d, _, _ := n.Authorize(context.Background(), allSecrets); d != authorizer.DecisionNoOpinion {
		t.Fatalf("unscoped secrets list: %v", d)
	}
	allCM := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "configmaps", Verb: "watch", Namespace: "kube-system"}
	if d, _, _ := n.Authorize(context.Background(), allCM); d != authorizer.DecisionNoOpinion {
		t.Fatalf("configmap watch: %v", d)
	}
	getUnref := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "secrets", Verb: "get", Namespace: "default", Name: "unref-probe"}
	if d, _, _ := n.Authorize(context.Background(), getUnref); d != authorizer.DecisionNoOpinion {
		t.Fatalf("unreferenced get: %v", d)
	}
	pvcWrite := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "persistentvolumeclaims", Verb: "patch", Namespace: "default", Name: "disk"}
	if d, reason, _ := n.Authorize(context.Background(), pvcWrite); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("pvc write: %v %q", d, reason)
	}
	pv := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "persistentvolumes", Verb: "get", Name: "disk"}
	if d, reason, _ := n.Authorize(context.Background(), pv); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("pv get: %v %q", d, reason)
	}
	pvWrite := *pv
	pvWrite.Verb = "patch"
	if d, reason, _ := n.Authorize(context.Background(), &pvWrite); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("pv write: %v %q", d, reason)
	}
	vaGet := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "volumeattachments", Verb: "get", APIGroup: "storage.k8s.io", Name: "disk"}
	if d, reason, _ := n.Authorize(context.Background(), vaGet); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("volumeattachment get: %v %q", d, reason)
	}
	pvcStatus := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "persistentvolumeclaims", Subresource: "status", Verb: "patch", Namespace: "default", Name: "disk"}
	if d, reason, _ := n.Authorize(context.Background(), pvcStatus); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("pvc status: %v %q", d, reason)
	}
	ownCSI := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "csinodes", Verb: "patch", APIGroup: "storage.k8s.io", Name: "k8flare-c1"}
	if d, _, _ := n.Authorize(context.Background(), ownCSI); d != authorizer.DecisionAllow {
		t.Fatalf("own csinode: %v", d)
	}
	otherCSI := *ownCSI
	otherCSI.Name = "k8flare-agent"
	if d, reason, _ := n.Authorize(context.Background(), &otherCSI); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("other csinode: %v %q", d, reason)
	}
	slice := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "resourceslices", Verb: "create", APIGroup: "resource.k8s.io", Name: "probe"}
	if d, _, _ := n.Authorize(context.Background(), slice); d != authorizer.DecisionAllow {
		t.Fatalf("resourceslice create: %v", d)
	}
	sliceSel, _ := fields.ParseSelector("spec.nodeName=k8flare-c1")
	wipe := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "resourceslices", Verb: "deletecollection", APIGroup: "resource.k8s.io", FieldSelectorRequirements: sliceSel.Requirements()}
	if d, _, _ := n.Authorize(context.Background(), wipe); d != authorizer.DecisionAllow {
		t.Fatalf("resourceslice deletecollection: %v", d)
	}
	wipeAll := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "resourceslices", Verb: "deletecollection", APIGroup: "resource.k8s.io"}
	if d, reason, _ := n.Authorize(context.Background(), wipeAll); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("unscoped resourceslice deletecollection: %v %q", d, reason)
	}
	sliceListAll := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "resourceslices", Verb: "list", APIGroup: "resource.k8s.io"}
	if d, reason, _ := n.Authorize(context.Background(), sliceListAll); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("unscoped resourceslice list: %v %q", d, reason)
	}
	sliceWatch := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "resourceslices", Verb: "watch", APIGroup: "resource.k8s.io", FieldSelectorRequirements: sliceSel.Requirements()}
	if d, _, _ := n.Authorize(context.Background(), sliceWatch); d != authorizer.DecisionAllow {
		t.Fatalf("scoped resourceslice watch: %v", d)
	}
}

func TestNodeAuthorizerResourceSliceOwner(t *testing.T) {
	slice := &resourcev1.ResourceSlice{}
	slice.Name = "probe"
	slice.Spec.Driver = "example.com/dra"
	nodeName := "k8flare-c1"
	slice.Spec.NodeName = &nodeName
	slice.Spec.Pool.Name = "p"
	slice.Spec.Pool.ResourceSliceCount = 1
	raw, err := runtime.Encode(resourceSliceCodec, slice)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"revision": 1,
			"kv": map[string]any{
				"key": r.URL.Query().Get("key"), "value": base64.StdEncoding.EncodeToString(raw), "modRevision": 1,
			},
		})
	}))
	defer srv.Close()
	n := &nodeAuthorizer{
		client: &kine.Client{HTTP: &http.Client{Transport: rewriteHost{base: srv.URL, next: srv.Client().Transport}}},
		ident:  nodeidentifierStub{},
	}
	node := &user.DefaultInfo{Name: "system:node:k8flare-c1", Groups: []string{user.NodesGroup}}
	own := &authorizer.AttributesRecord{User: node, ResourceRequest: true, Resource: "resourceslices", Verb: "get", APIGroup: "resource.k8s.io", Name: "probe"}
	if d, _, err := n.Authorize(context.Background(), own); err != nil || d != authorizer.DecisionAllow {
		t.Fatalf("own slice: %v %v", d, err)
	}
	other := &user.DefaultInfo{Name: "system:node:k8flare-agent", Groups: []string{user.NodesGroup}}
	own.User = other
	if d, reason, _ := n.Authorize(context.Background(), own); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("other slice: %v %q", d, reason)
	}
}

type rewriteHost struct {
	base string
	next http.RoundTripper
}

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	u.Scheme = "http"
	u.Host = strings.TrimPrefix(strings.TrimPrefix(r.base, "https://"), "http://")
	next := req.Clone(req.Context())
	next.URL = &u
	next.Host = u.Host
	return r.next.RoundTrip(next)
}

func TestNodeAuthorizerVolumeAttachmentOwner(t *testing.T) {
	va := &storagev1.VolumeAttachment{}
	va.Name = "disk"
	va.Spec.Attacher = "example.com/csi"
	va.Spec.NodeName = "k8flare-c1"
	pvName := "pv"
	va.Spec.Source.PersistentVolumeName = &pvName
	raw, err := runtime.Encode(volumeAttachmentCodec, va)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"revision": 1,
			"kv": map[string]any{
				"key": r.URL.Query().Get("key"), "value": base64.StdEncoding.EncodeToString(raw), "modRevision": 1,
			},
		})
	}))
	defer srv.Close()
	n := &nodeAuthorizer{
		client: &kine.Client{HTTP: &http.Client{Transport: rewriteHost{base: srv.URL, next: srv.Client().Transport}}},
		ident:  nodeidentifierStub{},
	}
	ownNode := &user.DefaultInfo{Name: "system:node:k8flare-c1", Groups: []string{user.NodesGroup}}
	own := &authorizer.AttributesRecord{User: ownNode, ResourceRequest: true, Resource: "volumeattachments", Verb: "get", APIGroup: "storage.k8s.io", Name: "disk"}
	if d, _, err := n.Authorize(context.Background(), own); err != nil || d != authorizer.DecisionAllow {
		t.Fatalf("own volumeattachment: %v %v", d, err)
	}
	own.User = &user.DefaultInfo{Name: "system:node:k8flare-agent", Groups: []string{user.NodesGroup}}
	if d, reason, _ := n.Authorize(context.Background(), own); d != authorizer.DecisionNoOpinion || reason == "" {
		t.Fatalf("other volumeattachment: %v %q", d, reason)
	}
}

type nodeidentifierStub struct{}

func (nodeidentifierStub) NodeIdentity(u user.Info) (string, bool) {
	if u == nil {
		return "", false
	}
	const p = "system:node:"
	name := u.GetName()
	if len(name) <= len(p) || name[:len(p)] != p {
		return "", false
	}
	for _, g := range u.GetGroups() {
		if g == user.NodesGroup {
			return name[len(p):], true
		}
	}
	return "", false
}
