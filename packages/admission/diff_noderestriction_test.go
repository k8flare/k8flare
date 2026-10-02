package admission

import (
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/utils/ptr"
)

func nodeUser(name string) user.Info {
	return &user.DefaultInfo{Name: "system:node:" + name, Groups: []string{"system:nodes", "system:authenticated"}}
}

func TestDiffNodeRestriction(t *testing.T) {
	const plugin = "NodeRestriction"
	n1 := nodeUser("n1")
	alice := &user.DefaultInfo{Name: "alice", Groups: []string{"system:authenticated"}}
	nodeNoName := &user.DefaultInfo{Name: "system:node:", Groups: []string{"system:nodes"}}
	nodeNotInGroup := &user.DefaultInfo{Name: "system:node:n1", Groups: []string{"system:authenticated"}}

	node := func(name string, mutate func(*corev1.Node)) *corev1.Node {
		n := diffNode(name, nil)
		if mutate != nil {
			mutate(n)
		}
		return n
	}
	nodeCase := func(name, op, subresource string, who user.Info, obj, old *corev1.Node) diffCase {
		c := diffCase{plugin: plugin, name: name, resource: schema.GroupVersionResource{Version: "v1", Resource: "nodes"}, kind: schema.GroupVersionKind{Version: "v1", Kind: "Node"}, objName: obj.Name, object: obj, operation: admissionOperation(op), subresource: subresource, user: who}
		if old != nil {
			c.oldObject = old
		}
		return c
	}
	cases := []diffCase{
		nodeCase("not a node user", "UPDATE", "", alice, node("n2", nil), node("n2", nil)),
		nodeCase("node user without the nodes group", "UPDATE", "", nodeNotInGroup, node("n2", nil), node("n2", nil)),
		nodeCase("node user with an empty node name", "UPDATE", "", nodeNoName, node("n1", nil), node("n1", nil)),
		nodeCase("node creates itself", "CREATE", "", n1, node("n1", nil), nil),
		nodeCase("node creates another node", "CREATE", "", n1, node("n2", nil), nil),
		nodeCase("node updates itself", "UPDATE", "", n1, node("n1", nil), node("n1", nil)),
		nodeCase("node updates another node", "UPDATE", "", n1, node("n2", nil), node("n2", nil)),
		nodeCase("node updates its status", "UPDATE", "status", n1, node("n1", nil), node("n1", nil)),
		nodeCase("node updates another node's status", "UPDATE", "status", n1, node("n2", nil), node("n2", nil)),
		nodeCase("node deletes itself", "DELETE", "", n1, node("n1", nil), node("n1", nil)),
		nodeCase("node deletes another node", "DELETE", "", n1, node("n2", nil), node("n2", nil)),
		nodeCase("create with a configSource", "CREATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Spec.ConfigSource = &corev1.NodeConfigSource{ConfigMap: &corev1.ConfigMapNodeConfigSource{Name: "c", Namespace: "ns", KubeletConfigKey: "k"}}
		}), nil),
		nodeCase("update adding a configSource", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Spec.ConfigSource = &corev1.NodeConfigSource{ConfigMap: &corev1.ConfigMapNodeConfigSource{Name: "c", Namespace: "ns", KubeletConfigKey: "k"}}
		}), node("n1", nil)),
		nodeCase("update changing taints", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Spec.Taints = []corev1.Taint{{Key: "x", Effect: corev1.TaintEffectNoSchedule}}
		}), node("n1", nil)),
		nodeCase("update keeping taints", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Spec.Taints = []corev1.Taint{{Key: "x", Effect: corev1.TaintEffectNoSchedule}}
		}), node("n1", func(n *corev1.Node) { n.Spec.Taints = []corev1.Taint{{Key: "x", Effect: corev1.TaintEffectNoSchedule}} })),
		nodeCase("update changing owner references", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: "p", UID: "u"}}
		}), node("n1", nil)),
		nodeCase("create with an allowed kubelet label", "CREATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"kubernetes.io/hostname": "n1", "node.kubernetes.io/instance-type": "x", "kubelet.kubernetes.io/foo": "y", "example.com/free": "z"}
		}), nil),
		nodeCase("create with a node-restriction label", "CREATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"node-restriction.kubernetes.io/foo": "bar"}
		}), nil),
		nodeCase("create with a sub-domain node-restriction label", "CREATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"example.node-restriction.kubernetes.io/foo": "bar"}
		}), nil),
		nodeCase("create with a non-kubelet kubernetes.io label", "CREATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"kubernetes.io/unknown": "bar"}
		}), nil),
		nodeCase("create with a k8s.io label", "CREATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"foo.k8s.io/x": "bar"}
		}), nil),
		nodeCase("update changing a node-restriction label", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"node-restriction.kubernetes.io/foo": "new"}
		}), node("n1", func(n *corev1.Node) { n.Labels = map[string]string{"node-restriction.kubernetes.io/foo": "old"} })),
		nodeCase("update removing a node-restriction label", "UPDATE", "", n1, node("n1", nil), node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"node-restriction.kubernetes.io/foo": "old"}
		})),
		nodeCase("update changing an allowed label", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"kubernetes.io/os": "linux", "topology.kubernetes.io/zone": "z"}
		}), node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"kubernetes.io/os": "windows", "topology.kubernetes.io/zone": "y"}
		})),
		nodeCase("update changing a beta kubelet label", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"beta.kubernetes.io/arch": "arm64", "failure-domain.beta.kubernetes.io/zone": "z"}
		}), node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"beta.kubernetes.io/arch": "amd64", "failure-domain.beta.kubernetes.io/zone": "y"}
		})),
		nodeCase("update changing a non-kubelet kubernetes.io label", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"kubernetes.io/unknown": "new"}
		}), node("n1", func(n *corev1.Node) { n.Labels = map[string]string{"kubernetes.io/unknown": "old"} })),
		nodeCase("update changing a custom label", "UPDATE", "", n1, node("n1", func(n *corev1.Node) {
			n.Labels = map[string]string{"example.com/x": "new"}
		}), node("n1", func(n *corev1.Node) { n.Labels = map[string]string{"example.com/x": "old"} })),
		nodeCase("subresource other than status", "UPDATE", "proxy", n1, node("n2", nil), node("n2", nil)),
	}

	p1 := func(mutate func(*corev1.Pod)) *corev1.Pod {
		return diffPod(func(p *corev1.Pod) {
			p.Spec.NodeName = "n1"
			if mutate != nil {
				mutate(p)
			}
		})
	}
	mirror := func(mutate func(*corev1.Pod)) *corev1.Pod {
		return p1(func(p *corev1.Pod) {
			p.Annotations = map[string]string{"kubernetes.io/config.mirror": "h"}
			p.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Node", Name: "n1", UID: "node-uid", Controller: ptr.To(true)}}
			if mutate != nil {
				mutate(p)
			}
		})
	}
	n1Object := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1", UID: "node-uid"}}
	podCase := func(name, op, subresource string, who user.Info, obj, old *corev1.Pod, cluster ...runtime.Object) diffCase {
		c := podDiffCase(plugin, name, obj)
		c.operation = admissionOperation(op)
		c.subresource = subresource
		c.user = who
		c.cluster = cluster
		if old != nil {
			c.oldObject = old
		}
		return c
	}
	deletePod := func(name string, who user.Info, cluster ...runtime.Object) diffCase {
		c := podDiffCase(plugin, name, nil)
		c.object = nil
		c.operation = "DELETE"
		c.user = who
		c.cluster = cluster
		return c
	}
	cases = append(cases,
		podCase("node creates a mirror pod for itself", "CREATE", "", n1, mirror(nil), nil, n1Object),
		podCase("node creates a pod without the mirror annotation", "CREATE", "", n1, p1(nil), nil, n1Object),
		podCase("node creates a mirror pod for another node", "CREATE", "", n1, mirror(func(p *corev1.Pod) { p.Spec.NodeName = "n2" }), nil, n1Object),
		podCase("mirror pod without an owner reference", "CREATE", "", n1, mirror(func(p *corev1.Pod) { p.OwnerReferences = nil }), nil, n1Object),
		podCase("mirror pod with two owner references", "CREATE", "", n1, mirror(func(p *corev1.Pod) {
			p.OwnerReferences = append(p.OwnerReferences, metav1.OwnerReference{APIVersion: "v1", Kind: "Node", Name: "n1", UID: "node-uid"})
		}), nil, n1Object),
		podCase("mirror pod owned by another node", "CREATE", "", n1, mirror(func(p *corev1.Pod) { p.OwnerReferences[0].Name = "n2" }), nil, n1Object),
		podCase("mirror pod whose owner is not a controller", "CREATE", "", n1, mirror(func(p *corev1.Pod) { p.OwnerReferences[0].Controller = nil }), nil, n1Object),
		podCase("mirror pod blocking owner deletion", "CREATE", "", n1, mirror(func(p *corev1.Pod) { p.OwnerReferences[0].BlockOwnerDeletion = ptr.To(true) }), nil, n1Object),
		podCase("mirror pod with the wrong owner UID", "CREATE", "", n1, mirror(func(p *corev1.Pod) { p.OwnerReferences[0].UID = "other" }), nil, n1Object),
		podCase("mirror pod whose node object is missing", "CREATE", "", n1, mirror(nil), nil),
		podCase("mirror pod referencing a secret volume", "CREATE", "", n1, mirror(func(p *corev1.Pod) {
			p.Spec.Volumes = []corev1.Volume{{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "s"}}}}
		}), nil, n1Object),
		podCase("mirror pod referencing a service account", "CREATE", "", n1, mirror(func(p *corev1.Pod) { p.Spec.ServiceAccountName = "sa" }), nil, n1Object),
		podCase("mirror pod referencing a config map env", "CREATE", "", n1, mirror(func(p *corev1.Pod) {
			p.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm"}}}}
		}), nil, n1Object),
		podCase("mirror pod referencing a claim", "CREATE", "", n1, mirror(func(p *corev1.Pod) {
			p.Spec.Volumes = []corev1.Volume{{Name: "v", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "c"}}}}
		}), nil, n1Object),
		podCase("mirror pod with an emptyDir volume", "CREATE", "", n1, mirror(func(p *corev1.Pod) {
			p.Spec.Volumes = []corev1.Volume{{Name: "v", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
		}), nil, n1Object),
		podCase("not a node user creates a pod", "CREATE", "", alice, p1(nil), nil),
		podCase("node updates a pod", "UPDATE", "", n1, p1(nil), p1(nil)),
		podCase("node updates its pod's status", "UPDATE", "status", n1, p1(nil), p1(nil)),
		podCase("node updates another node's pod status", "UPDATE", "status", n1, p1(func(p *corev1.Pod) { p.Spec.NodeName = "n2" }), p1(func(p *corev1.Pod) { p.Spec.NodeName = "n2" })),
		podCase("node updates the status of an unbound pod", "UPDATE", "status", n1, p1(func(p *corev1.Pod) { p.Spec.NodeName = "" }), p1(func(p *corev1.Pod) { p.Spec.NodeName = "" })),
		podCase("node uses an unexpected subresource", "UPDATE", "binding", n1, p1(nil), p1(nil)),
		podCase("node uses the ephemeralcontainers subresource", "UPDATE", "ephemeralcontainers", n1, p1(nil), p1(nil)),
		deletePod("node deletes its own pod", n1, p1(nil)),
		deletePod("node deletes another node's pod", n1, p1(func(p *corev1.Pod) { p.Spec.NodeName = "n2" })),
		deletePod("node deletes a missing pod", n1),
		deletePod("not a node user deletes a pod", alice, p1(func(p *corev1.Pod) { p.Spec.NodeName = "n2" })),
	)
	evictionCase := func(name string, who user.Info, cluster ...runtime.Object) diffCase {
		c := podDiffCase(plugin, name, nil)
		c.object = &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
		c.kind = schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "Eviction"}
		c.subresource = "eviction"
		c.user = who
		c.cluster = cluster
		return c
	}
	cases = append(cases,
		evictionCase("node evicts its own pod", n1, p1(nil)),
		evictionCase("node evicts another node's pod", n1, p1(func(p *corev1.Pod) { p.Spec.NodeName = "n2" })),
		evictionCase("node evicts a missing pod", n1),
	)

	lease := func(ns, name string) *coordinationv1.Lease {
		return &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	}
	leaseCase := func(name, op string, who user.Info, l *coordinationv1.Lease) diffCase {
		c := diffCase{plugin: plugin, name: name, resource: schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}, kind: schema.GroupVersionKind{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"}, namespace: l.Namespace, objName: l.Name, operation: admissionOperation(op), user: who, object: l}
		if op == "UPDATE" {
			c.oldObject = l
		}
		return c
	}
	cases = append(cases,
		leaseCase("node creates its own lease", "CREATE", n1, lease("kube-node-lease", "n1")),
		leaseCase("node creates another node's lease", "CREATE", n1, lease("kube-node-lease", "n2")),
		leaseCase("node creates a lease in another namespace", "CREATE", n1, lease("default", "n1")),
		leaseCase("node updates its own lease", "UPDATE", n1, lease("kube-node-lease", "n1")),
		leaseCase("node updates another node's lease", "UPDATE", n1, lease("kube-node-lease", "n2")),
		leaseCase("not a node user writes a lease", "UPDATE", alice, lease("default", "x")),
	)

	csiNode := func(name string) *storagev1.CSINode {
		return &storagev1.CSINode{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	csiCase := func(name, op string, who user.Info, obj *storagev1.CSINode) diffCase {
		c := diffCase{plugin: plugin, name: name, resource: schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "csinodes"}, kind: schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "CSINode"}, objName: obj.Name, operation: admissionOperation(op), user: who, object: obj}
		if op == "UPDATE" {
			c.oldObject = obj
		}
		return c
	}
	cases = append(cases,
		csiCase("node creates its own CSINode", "CREATE", n1, csiNode("n1")),
		csiCase("node creates another CSINode", "CREATE", n1, csiNode("n2")),
		csiCase("node updates another CSINode", "UPDATE", n1, csiNode("n2")),
	)

	csrFor := func(cn, signer string) *certificatesv1.CertificateSigningRequest {
		return diffCSR(signer, csrPEMWithCN(t, cn), nil)
	}
	csrNodeCase := func(name string, who user.Info, csr *certificatesv1.CertificateSigningRequest) diffCase {
		c := csrCase(plugin, name, "CREATE", "", csr, nil)
		c.phase = ""
		c.user = who
		return c
	}
	cases = append(cases,
		csrNodeCase("node creates a kubelet client CSR for itself", n1, csrFor("system:node:n1", certificatesv1.KubeAPIServerClientKubeletSignerName)),
		csrNodeCase("node creates a kubelet client CSR for another node", n1, csrFor("system:node:n2", certificatesv1.KubeAPIServerClientKubeletSignerName)),
		csrNodeCase("node creates a kubelet serving CSR for itself", n1, csrFor("system:node:n1", certificatesv1.KubeletServingSignerName)),
		csrNodeCase("node creates a kubelet serving CSR for another node", n1, csrFor("system:node:n2", certificatesv1.KubeletServingSignerName)),
		csrNodeCase("node creates a CSR for another signer", n1, csrFor("anything", "example.com/custom")),
		csrNodeCase("node creates a CSR with an unparsable request", n1, diffCSR(certificatesv1.KubeletServingSignerName, []byte("garbage"), nil)),
	)

	tokenRequest := func(ref *authenticationv1.BoundObjectReference) *authenticationv1.TokenRequest {
		return &authenticationv1.TokenRequest{ObjectMeta: metav1.ObjectMeta{Name: "sa", Namespace: "ns"}, Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"aud"}, BoundObjectRef: ref}}
	}
	tokenCase := func(name string, who user.Info, tr *authenticationv1.TokenRequest, cluster ...runtime.Object) diffCase {
		return diffCase{plugin: plugin, name: name, resource: schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}, kind: schema.GroupVersionKind{Group: "authentication.k8s.io", Version: "v1", Kind: "TokenRequest"}, subresource: "token", namespace: "ns", objName: "sa", user: who, object: tr, cluster: cluster}
	}
	boundPod := p1(func(p *corev1.Pod) {
		p.UID = types.UID("pod-uid")
		p.Spec.Volumes = []corev1.Volume{{Name: "t", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Audience: "aud", Path: "token"}}}}}}}
	})
	podRef := func(uid string) *authenticationv1.BoundObjectReference {
		return &authenticationv1.BoundObjectReference{APIVersion: "v1", Kind: "Pod", Name: "p", UID: types.UID(uid)}
	}
	sa := diffServiceAccount("sa", nil)
	cases = append(cases,
		tokenCase("node requests a token bound to its pod", n1, tokenRequest(podRef("pod-uid")), boundPod, sa),
		tokenCase("node requests a token for an audience the pod does not reference", n1, &authenticationv1.TokenRequest{ObjectMeta: metav1.ObjectMeta{Name: "sa", Namespace: "ns"}, Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"other"}, BoundObjectRef: podRef("pod-uid")}}, boundPod, sa),
		tokenCase("node requests a token with two audiences", n1, &authenticationv1.TokenRequest{ObjectMeta: metav1.ObjectMeta{Name: "sa", Namespace: "ns"}, Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"aud", "aud2"}, BoundObjectRef: podRef("pod-uid")}}, boundPod, sa),
		tokenCase("node requests a token without a bound object", n1, tokenRequest(nil), boundPod, sa),
		tokenCase("node requests a token bound to a secret", n1, tokenRequest(&authenticationv1.BoundObjectReference{APIVersion: "v1", Kind: "Secret", Name: "s", UID: "u"}), boundPod, sa),
		tokenCase("node requests a token bound to a pod without a UID", n1, tokenRequest(podRef("")), boundPod, sa),
		tokenCase("node requests a token with the wrong pod UID", n1, tokenRequest(podRef("other")), boundPod, sa),
		tokenCase("node requests a token for a pod on another node", n1, tokenRequest(podRef("pod-uid")), p1(func(p *corev1.Pod) { p.UID = "pod-uid"; p.Spec.NodeName = "n2" }), sa),
		tokenCase("node requests a token for a missing pod", n1, tokenRequest(podRef("pod-uid")), sa),
		tokenCase("not a node user requests a token", alice, tokenRequest(nil), sa),
	)

	claim := func(mutate func(*corev1.PersistentVolumeClaim)) *corev1.PersistentVolumeClaim {
		return diffClaim(func(p *corev1.PersistentVolumeClaim) {
			p.Status.Phase = corev1.ClaimBound
			p.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: quantity("1Gi")}
			if mutate != nil {
				mutate(p)
			}
		})
	}
	pvcNodeCase := func(name, op, subresource string, who user.Info, obj, old *corev1.PersistentVolumeClaim) diffCase {
		c := claimCase(plugin, name, obj)
		c.operation = admissionOperation(op)
		c.subresource = subresource
		c.user = who
		c.oldObject = old
		return c
	}
	cases = append(cases,
		pvcNodeCase("node updates claim status capacity", "UPDATE", "status", n1, claim(func(p *corev1.PersistentVolumeClaim) {
			p.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: quantity("2Gi")}
		}), claim(nil)),
		pvcNodeCase("node updates claim status conditions", "UPDATE", "status", n1, claim(func(p *corev1.PersistentVolumeClaim) {
			p.Status.Conditions = []corev1.PersistentVolumeClaimCondition{{Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue}}
		}), claim(nil)),
		pvcNodeCase("node updates claim status phase", "UPDATE", "status", n1, claim(func(p *corev1.PersistentVolumeClaim) { p.Status.Phase = corev1.ClaimLost }), claim(nil)),
		pvcNodeCase("node updates claim spec through status", "UPDATE", "status", n1, claim(func(p *corev1.PersistentVolumeClaim) {
			p.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: quantity("5Gi")}
		}), claim(nil)),
		pvcNodeCase("node updates claim labels through status", "UPDATE", "status", n1, claim(func(p *corev1.PersistentVolumeClaim) { p.Labels = map[string]string{"a": "b"} }), claim(nil)),
		pvcNodeCase("node updates claim main resource", "UPDATE", "", n1, claim(nil), claim(nil)),
		pvcNodeCase("node creates a claim status", "CREATE", "status", n1, claim(nil), nil),
		pvcNodeCase("not a node user updates a claim", "UPDATE", "", alice, claim(nil), claim(nil)),
	)
	notFound := "outcome: upstream denied 404 NotFound; ours denied 403 Forbidden"
	allowed := "outcome: upstream denied 403 Forbidden; ours allowed"
	ownerReason := "ours does not check the mirror pod's owner references (single owner, kind Node, own name, controller, no blockOwnerDeletion, matching node UID) (noderestriction/admission.go admitPodCreate)"
	refReason := "ours does not reject a mirror pod that references secrets, config maps, claims or service accounts (noderestriction/admission.go admitPodCreate, podutil.HasAPIObjectReference)"
	missingReason := "a missing target is NotFound for upstream and Forbidden for ours (noderestriction/admission.go admitPod, admitPodEviction, admitServiceAccount)"
	cases = applyKnown(t, cases, map[string]*knownDifference{
		"mirror pod without an owner reference":                            {reason: ownerReason, signature: allowed},
		"mirror pod with two owner references":                             {reason: ownerReason, signature: allowed},
		"mirror pod owned by another node":                                 {reason: ownerReason, signature: allowed},
		"mirror pod whose owner is not a controller":                       {reason: ownerReason, signature: allowed},
		"mirror pod blocking owner deletion":                               {reason: ownerReason, signature: allowed},
		"mirror pod with the wrong owner UID":                              {reason: ownerReason, signature: allowed},
		"mirror pod whose node object is missing":                          {reason: ownerReason, signature: "outcome: upstream denied 404 NotFound; ours allowed"},
		"mirror pod referencing a secret volume":                           {reason: refReason, signature: allowed},
		"mirror pod referencing a service account":                         {reason: refReason, signature: allowed},
		"mirror pod referencing a config map env":                          {reason: refReason, signature: allowed},
		"mirror pod referencing a claim":                                   {reason: refReason, signature: allowed},
		"node deletes a missing pod":                                       {reason: missingReason, signature: notFound},
		"node evicts a missing pod":                                        {reason: missingReason, signature: notFound},
		"node requests a token for a missing pod":                          {reason: missingReason, signature: notFound},
		"node requests a token for an audience the pod does not reference": {reason: "ours has no ServiceAccountNodeAudienceRestriction: it allows any audience, upstream requires the audience in a pod volume or an authorizer decision (noderestriction/admission.go validateNodeServiceAccountAudience)", signature: allowed},
		"node requests a token with two audiences":                         {reason: "ours has no ServiceAccountNodeAudienceRestriction: it does not limit a node to 0 or 1 audiences (noderestriction/admission.go validateNodeServiceAccountAudience)", signature: allowed},
		"node updates claim status phase":                                  {reason: "upstream appends a field diff to the message, ours does not (noderestriction/admission.go admitPVCStatus)", messageOnly: true},
		"node updates claim spec through status":                           {reason: "upstream appends a field diff to the message, ours does not (noderestriction/admission.go admitPVCStatus)", messageOnly: true},
		"node updates claim labels through status":                         {reason: "upstream appends a field diff to the message, ours does not (noderestriction/admission.go admitPVCStatus)", messageOnly: true},
	})
	runDiffCases(t, cases)
}
