package authz

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	certsv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	genericfeatures "k8s.io/apiserver/pkg/features"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/component-base/featuregate"
	"k8s.io/kubernetes/pkg/auth/nodeidentifier"
	"k8s.io/kubernetes/pkg/features"
	upstreamnode "k8s.io/kubernetes/plugin/pkg/auth/authorizer/node"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
	"k8s.io/utils/ptr"
)

var (
	tableNode0            = &user.DefaultInfo{Name: "system:node:node0", Groups: []string{user.NodesGroup}}
	tableNodeUnregistered = &user.DefaultInfo{Name: "system:node:nodeunregistered", Groups: []string{user.NodesGroup}}
)

func tableFields(selector string) fields.Requirements {
	parsed, err := fields.ParseSelector(selector)
	if err != nil {
		panic(err)
	}
	return parsed.Requirements()
}

type diffState struct {
	pods        []*corev1.Pod
	pvcs        []*corev1.PersistentVolumeClaim
	pvs         []*corev1.PersistentVolume
	attachments []*storagev1.VolumeAttachment
	slices      []*resourcev1.ResourceSlice
	requests    []*certsv1beta1.PodCertificateRequest
}

func (s *diffState) merge(other diffState) {
	s.pods = append(s.pods, other.pods...)
	s.pvcs = append(s.pvcs, other.pvcs...)
	s.pvs = append(s.pvs, other.pvs...)
	s.attachments = append(s.attachments, other.attachments...)
	s.slices = append(s.slices, other.slices...)
	s.requests = append(s.requests, other.requests...)
}

func (s *diffState) bind(pv *corev1.PersistentVolume) {
	s.pvs = append(s.pvs, pv)
	s.pvcs = append(s.pvcs, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: pv.Spec.ClaimRef.Namespace, Name: pv.Spec.ClaimRef.Name},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: pv.Name},
	})
}

func (s diffState) upstream() authorizer.Authorizer {
	g := upstreamnode.NewGraph()
	for _, pod := range s.pods {
		if pod.Spec.NodeName != "" {
			g.AddPod(pod)
		}
	}
	for _, pv := range s.pvs {
		g.AddPV(pv)
	}
	for _, attachment := range s.attachments {
		g.AddVolumeAttachment(attachment.Name, attachment.Spec.NodeName)
	}
	for _, slice := range s.slices {
		g.AddResourceSlice(slice.Name, ptr.Deref(slice.Spec.NodeName, ""))
	}
	for _, request := range s.requests {
		g.AddPodCertificateRequest(request)
	}
	return upstreamnode.NewAuthorizer(g, nodeidentifier.NewDefaultNodeIdentifier(), bootstrappolicy.NodeRules())
}

func (s diffState) project(t *testing.T) authorizer.Authorizer {
	t.Helper()
	store := fakeStore{}
	core := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	for _, pod := range s.pods {
		store.put(t, core, "/registry/pods/"+pod.Namespace+"/"+pod.Name, pod)
	}
	for _, pvc := range s.pvcs {
		store.put(t, core, "/registry/persistentvolumeclaims/"+pvc.Namespace+"/"+pvc.Name, pvc)
	}
	for _, pv := range s.pvs {
		store.put(t, core, "/registry/persistentvolumes/"+pv.Name, pv)
	}
	for _, attachment := range s.attachments {
		store.put(t, volumeAttachmentCodec, "/registry/volumeattachments/"+attachment.Name, attachment)
	}
	for _, slice := range s.slices {
		store.put(t, resourceSliceCodec, "/registry/resourceslices/"+slice.Name, slice)
	}
	return newNodeAuthorizer(&kine.Client{HTTP: &http.Client{Transport: store}})
}

type fakeStore map[string][]byte

func (f fakeStore) put(t *testing.T, codec runtime.Codec, key string, obj runtime.Object) {
	t.Helper()
	raw, err := runtime.Encode(codec, obj)
	if err != nil {
		t.Fatal(err)
	}
	f[key] = raw
}

func (f fakeStore) RoundTrip(req *http.Request) (*http.Response, error) {
	status := http.StatusOK
	body := map[string]any{"revision": 1}
	switch req.URL.Path {
	case "/kv":
		key := req.URL.Query().Get("key")
		raw, ok := f[key]
		if !ok {
			status = http.StatusNotFound
			body["error"] = "not found"
			break
		}
		body["kv"] = map[string]any{"key": key, "value": base64.StdEncoding.EncodeToString(raw), "modRevision": 1}
	case "/list":
		prefix := req.URL.Query().Get("prefix")
		keys := make([]string, 0, len(f))
		for key := range f {
			if strings.HasPrefix(key, prefix) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		kvs := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			kvs = append(kvs, map[string]any{"key": key, "value": base64.StdEncoding.EncodeToString(f[key]), "modRevision": 1})
		}
		body["kvs"] = kvs
	default:
		status = http.StatusBadRequest
		body["error"] = "unsupported path " + req.URL.Path
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw)), Request: req}, nil
}

func scenarioForNode(node string) diffState {
	ref := func(prefix string) string { return prefix + "-" + node }
	local := func(name string) corev1.LocalObjectReference { return corev1.LocalObjectReference{Name: name} }
	secretEnv := func(name string) corev1.EnvVar {
		return corev1.EnvVar{Name: "S", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: local(name), Key: "k"}}}
	}
	configMapEnv := func(name string) corev1.EnvVar {
		return corev1.EnvVar{Name: "C", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: local(name), Key: "k"}}}
	}
	envFrom := func(secret, configMap string) []corev1.EnvFromSource {
		return []corev1.EnvFromSource{
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: local(secret)}},
			{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: local(configMap)}},
		}
	}
	claimVolume := func(claim string) corev1.Volume {
		return corev1.Volume{Name: claim, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim}}}
	}
	boundPV := func(pv, claim, namespace string, source corev1.PersistentVolumeSource) *corev1.PersistentVolume {
		return &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: pv},
			Spec:       corev1.PersistentVolumeSpec{PersistentVolumeSource: source, ClaimRef: &corev1.ObjectReference{Namespace: namespace, Name: claim}},
		}
	}

	var state diffState
	state.bind(boundPV(ref("pv"), ref("pvc"), "ns0", corev1.PersistentVolumeSource{FlexVolume: &corev1.FlexPersistentVolumeSource{Driver: "d", SecretRef: &corev1.SecretReference{Name: ref("secret-pv")}}}))
	state.bind(boundPV(ref("pv-csi"), ref("pvc-csi"), "ns0", corev1.PersistentVolumeSource{CSI: &corev1.CSIPersistentVolumeSource{Driver: "d", VolumeHandle: "h", NodePublishSecretRef: &corev1.SecretReference{Name: ref("secret-pvcsi"), Namespace: "ns0"}}}))
	state.bind(boundPV(ref("pv-gluster"), ref("pvc-gluster"), "ns0", corev1.PersistentVolumeSource{Glusterfs: &corev1.GlusterfsPersistentVolumeSource{EndpointsName: ref("ep-pv"), EndpointsNamespace: ptr.To("ns1"), Path: "p"}}))
	state.pvs = append(state.pvs, boundPV(ref("pv-prebound"), ref("pvc-prebound"), "ns0", corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/p"}}))
	state.pvcs = append(state.pvcs, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: ref("pvc-prebound")}})
	state.pvs = append(state.pvs, &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: ref("pv-claimless")}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/c"}}}})
	state.pvcs = append(state.pvcs, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: ref("pvc-claimless")}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: ref("pv-claimless")}})
	state.bind(boundPV(ref("pv-mirror"), ref("pvc-mirror"), "ns0", corev1.PersistentVolumeSource{FlexVolume: &corev1.FlexPersistentVolumeSource{Driver: "d", SecretRef: &corev1.SecretReference{Name: ref("secret-pvmirror")}}}))

	state.pods = append(state.pods,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: ref("p")},
			Spec: corev1.PodSpec{
				NodeName:           node,
				ServiceAccountName: ref("sa"),
				ImagePullSecrets:   []corev1.LocalObjectReference{local(ref("secret-pull"))},
				Containers: []corev1.Container{{
					Name:    "c",
					Env:     []corev1.EnvVar{secretEnv(ref("secret-env")), configMapEnv(ref("cm-env"))},
					EnvFrom: envFrom(ref("secret-envfrom"), ref("cm-envfrom")),
				}},
				InitContainers: []corev1.Container{{Name: "i", Env: []corev1.EnvVar{secretEnv(ref("secret-init"))}}},
				EphemeralContainers: []corev1.EphemeralContainer{{
					EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "e", EnvFrom: envFrom(ref("secret-eph"), ref("cm-eph"))},
				}},
				Volumes: []corev1.Volume{
					{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ref("secret-vol")}}},
					{Name: "configmap", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: local(ref("cm-vol"))}}},
					{Name: "projected", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{
						{Secret: &corev1.SecretProjection{LocalObjectReference: local(ref("secret-proj"))}},
						{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: local(ref("cm-proj"))}},
					}}}},
					{Name: "csi", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: "d", NodePublishSecretRef: &corev1.LocalObjectReference{Name: ref("secret-csi")}}}},
					{Name: "scratch", VolumeSource: corev1.VolumeSource{Ephemeral: &corev1.EphemeralVolumeSource{VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{}}}},
					{Name: "gluster", VolumeSource: corev1.VolumeSource{Glusterfs: &corev1.GlusterfsVolumeSource{EndpointsName: ref("ep-vol"), Path: "p"}}},
					claimVolume(ref("pvc")),
					claimVolume(ref("pvc-csi")),
					claimVolume(ref("pvc-gluster")),
					claimVolume(ref("pvc-prebound")),
					claimVolume(ref("pvc-claimless")),
				},
				ResourceClaims: []corev1.PodResourceClaim{
					{Name: "named", ResourceClaimName: ptr.To(ref("claim"))},
					{Name: "templated", ResourceClaimTemplateName: ptr.To(ref("tmpl"))},
					{Name: "pending", ResourceClaimTemplateName: ptr.To(ref("tmpl-nostatus"))},
				},
			},
			Status: corev1.PodStatus{
				ResourceClaimStatuses:       []corev1.PodResourceClaimStatus{{Name: "templated", ResourceClaimName: ptr.To(ref("gen-claim"))}},
				ExtendedResourceClaimStatus: &corev1.PodExtendedResourceClaimStatus{ResourceClaimName: ref("ext-claim")},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: ref("p-mirror"), Annotations: map[string]string{corev1.MirrorPodAnnotationKey: "hash"}},
			Spec: corev1.PodSpec{
				NodeName:           node,
				ServiceAccountName: ref("sa-mirror"),
				Volumes: []corev1.Volume{
					{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ref("secret-mirror")}}},
					{Name: "configmap", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: local(ref("cm-mirror"))}}},
					claimVolume(ref("pvc-mirror")),
				},
				ResourceClaims: []corev1.PodResourceClaim{{Name: "named", ResourceClaimName: ptr.To(ref("claim-mirror"))}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: ref("p-ns1")},
			Spec: corev1.PodSpec{
				NodeName:           node,
				ServiceAccountName: ref("sa-ns1"),
				Volumes:            []corev1.Volume{{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ref("secret-ns1")}}}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: ref("p-nosa")},
			Spec:       corev1.PodSpec{NodeName: node},
		},
	)
	state.attachments = append(state.attachments, &storagev1.VolumeAttachment{ObjectMeta: metav1.ObjectMeta{Name: ref("va")}, Spec: storagev1.VolumeAttachmentSpec{NodeName: node}})
	state.slices = append(state.slices, &resourcev1.ResourceSlice{ObjectMeta: metav1.ObjectMeta{Name: ref("slice")}, Spec: resourcev1.ResourceSliceSpec{NodeName: ptr.To(node)}})
	state.requests = append(state.requests, &certsv1beta1.PodCertificateRequest{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: ref("pcr")},
		Spec:       certsv1beta1.PodCertificateRequestSpec{PodName: ref("p"), NodeName: types.NodeName(node)},
	})
	return state
}

func scenarioShared() diffState {
	local := func(name string) corev1.LocalObjectReference { return corev1.LocalObjectReference{Name: name} }
	sharedSpec := func(node, sa, secret, configMap, claim, pvc string) corev1.PodSpec {
		return corev1.PodSpec{
			NodeName:           node,
			ServiceAccountName: sa,
			Volumes: []corev1.Volume{
				{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secret}}},
				{Name: "configmap", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: local(configMap)}}},
				{Name: "claim", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc}}},
			},
			ResourceClaims: []corev1.PodResourceClaim{{Name: "named", ResourceClaimName: ptr.To(claim)}},
		}
	}
	flexPV := func(name, claim, secret string) *corev1.PersistentVolume {
		return &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{FlexVolume: &corev1.FlexPersistentVolumeSource{Driver: "d", SecretRef: &corev1.SecretReference{Name: secret}}},
				ClaimRef:               &corev1.ObjectReference{Namespace: "ns0", Name: claim},
			},
		}
	}
	var state diffState
	state.bind(flexPV("pv-both", "pvc-both", "secret-pv-both"))
	state.bind(flexPV("pv-none", "pvc-none", "secret-pv-none"))
	state.pods = append(state.pods,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: "p-pair-node0"}, Spec: sharedSpec("node0", "sa-both", "secret-both", "cm-both", "claim-both", "pvc-both")},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: "p-pair-node1"}, Spec: sharedSpec("node1", "sa-both", "secret-both", "cm-both", "claim-both", "pvc-both")},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns0", Name: "p-unbound"}, Spec: sharedSpec("", "sa-none", "secret-none", "cm-none", "claim-none", "pvc-none")},
	)
	state.attachments = append(state.attachments, &storagev1.VolumeAttachment{ObjectMeta: metav1.ObjectMeta{Name: "va-none"}})
	state.slices = append(state.slices, &resourcev1.ResourceSlice{ObjectMeta: metav1.ObjectMeta{Name: "slice-none"}})
	return state
}

func upstreamFixture() diffState {
	const nodes, podsPerNode, namespaces = 2, 2, 2
	var state diffState
	for n := 0; n < nodes; n++ {
		nodeName := fmt.Sprintf("node%d", n)
		for p := 0; p < podsPerNode; p++ {
			name := fmt.Sprintf("pod%d-%s", p, nodeName)
			namespace := fmt.Sprintf("ns%d", p%namespaces)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
			pod.Spec.NodeName = nodeName
			pod.Spec.ServiceAccountName = fmt.Sprintf("svcacct%d-%s", p, nodeName)
			pod.Spec.Volumes = []corev1.Volume{
				{VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "secret0-" + name}}},
				{VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "secret0-shared"}}},
				{VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "configmap0-" + name}}}},
			}
			pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pv0-%s-%s", name, namespace)}}
			pv.Spec.FlexVolume = &corev1.FlexPersistentVolumeSource{SecretRef: &corev1.SecretReference{Name: "secret-" + pv.Name}}
			pv.Spec.ClaimRef = &corev1.ObjectReference{Name: "pvc0-" + name, Namespace: namespace}
			state.bind(pv)
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pv.Spec.ClaimRef.Name}}})
			claimName := fmt.Sprintf("claim0-%s-%s", name, namespace)
			templateName := fmt.Sprintf("claimtemplate0-%s-%s", name, namespace)
			generated := fmt.Sprintf("generated-claim-%s-%s-0", name, namespace)
			pod.Spec.ResourceClaims = []corev1.PodResourceClaim{
				{Name: "claim0", ResourceClaimName: &claimName},
				{Name: "claimtemplate0", ResourceClaimTemplateName: &templateName},
				{Name: "claimtemplate-with-claim0", ResourceClaimTemplateName: &templateName},
			}
			pod.Status.ResourceClaimStatuses = []corev1.PodResourceClaimStatus{{Name: "claimtemplate-with-claim0", ResourceClaimName: &generated}}
			state.pods = append(state.pods, pod)
			for r := 0; r < 2; r++ {
				state.requests = append(state.requests, &certsv1beta1.PodCertificateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: fmt.Sprintf("pcr%d-%s", r, name)},
					Spec:       certsv1beta1.PodCertificateRequestSpec{PodName: name, ServiceAccountName: pod.Spec.ServiceAccountName, NodeName: types.NodeName(nodeName)},
				})
			}
		}
		state.attachments = append(state.attachments, &storagev1.VolumeAttachment{ObjectMeta: metav1.ObjectMeta{Name: "attachment0-" + nodeName}, Spec: storagev1.VolumeAttachmentSpec{NodeName: nodeName}})
		for s := 0; s <= 2; s++ {
			state.slices = append(state.slices, &resourcev1.ResourceSlice{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("slice%d-%s", s, nodeName)}, Spec: resourcev1.ResourceSliceSpec{NodeName: ptr.To(nodeName)}})
		}
	}
	return state
}

func combinedState() diffState {
	var state diffState
	state.merge(scenarioForNode("node0"))
	state.merge(scenarioForNode("node1"))
	state.merge(scenarioShared())
	state.merge(upstreamFixture())
	return state
}

type diffUser struct {
	label string
	info  user.Info
	node  string
}

var diffUsers = []diffUser{
	{"node0", &user.DefaultInfo{Name: "system:node:node0", Groups: []string{user.NodesGroup}}, "node0"},
	{"node1", &user.DefaultInfo{Name: "system:node:node1", Groups: []string{user.NodesGroup}}, "node1"},
	{"alice", &user.DefaultInfo{Name: "alice", Groups: []string{"system:authenticated"}}, ""},
	{"node-empty-name", &user.DefaultInfo{Name: "system:node:", Groups: []string{user.NodesGroup}}, ""},
	{"node-without-group", &user.DefaultInfo{Name: "system:node:node0"}, ""},
}

var diffVerbs = []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}

type scope int

const (
	namespaced scope = iota
	clusterScoped
	leaseScoped
)

type resourceCase struct {
	group       string
	resource    string
	subresource string
	scope       scope
	names       []string
}

func (r resourceCase) key() string {
	if r.subresource != "" {
		return r.resource + "/" + r.subresource
	}
	return r.resource
}

func (r resourceCase) namespaces() []string {
	switch r.scope {
	case clusterScoped:
		return []string{""}
	case leaseScoped:
		return []string{corev1.NamespaceNodeLease, "ns0"}
	}
	return []string{"ns0", "ns1", ""}
}

func perNode(prefixes ...string) []string {
	var out []string
	for _, node := range []string{"node0", "node1"} {
		for _, prefix := range prefixes {
			out = append(out, prefix+"-"+node)
		}
	}
	return out
}

func nameList(perNodePrefixes []string, others ...string) []string {
	return append(perNode(perNodePrefixes...), others...)
}

var (
	secretNames = nameList([]string{"secret-vol", "secret-pull", "secret-env", "secret-envfrom", "secret-init", "secret-eph", "secret-proj", "secret-csi", "secret-pv", "secret-pvcsi", "secret-mirror", "secret-pvmirror", "secret-ns1"},
		"secret-both", "secret-pv-both", "secret-none", "secret-pv-none", "secret-absent", "secret0-pod0-node0", "secret0-shared", "secret-pv0-pod0-node0-ns0")
	configMapNames = nameList([]string{"cm-vol", "cm-env", "cm-envfrom", "cm-proj", "cm-eph", "cm-mirror"}, "cm-both", "cm-none", "cm-absent", "configmap0-pod0-node0")
	claimNames     = nameList([]string{"pvc", "pvc-csi", "pvc-gluster", "pvc-mirror", "pvc-prebound", "pvc-claimless"}, "p-node0-scratch", "p-node1-scratch", "pvc-both", "pvc-none", "pvc-absent", "pvc0-pod0-node0")
	volumeNames    = nameList([]string{"pv", "pv-csi", "pv-gluster", "pv-mirror", "pv-prebound", "pv-claimless"}, "pv-both", "pv-none", "pv-absent", "pv0-pod0-node0-ns0")
	resourceClaims = nameList([]string{"claim", "gen-claim", "tmpl", "tmpl-nostatus", "ext-claim", "claim-mirror"}, "claim-both", "claim-none", "claim-absent", "claim0-pod0-node0-ns0")
	accountNames   = nameList([]string{"sa", "sa-mirror", "sa-ns1"}, "default", "sa-both", "sa-none", "sa-absent", "svcacct0-node0")
	endpointNames  = nameList([]string{"ep-vol", "ep-pv"}, "ep-absent")
	podNames       = nameList([]string{"p", "p-mirror", "p-ns1", "p-nosa"}, "p-pair-node0", "p-pair-node1", "p-unbound", "p-absent", "pod0-node0")
	attachments    = nameList([]string{"va"}, "va-none", "va-absent", "attachment0-node0")
	sliceNames     = nameList([]string{"slice"}, "slice-none", "slice-absent", "slice0-node0")
	requestNames   = nameList([]string{"pcr"}, "pcr-absent", "pcr0-pod0-node0")
	nodeNames      = []string{"node0", "node1", "node-unknown"}
	miscNames      = []string{"misc-node0", "misc-node1", "misc"}
)

var diffResources = func() []resourceCase {
	cases := []resourceCase{
		{"", "secrets", "", namespaced, secretNames},
		{"", "secrets", "status", namespaced, secretNames[:3]},
		{"", "configmaps", "", namespaced, configMapNames},
		{"", "persistentvolumeclaims", "", namespaced, claimNames},
		{"", "persistentvolumeclaims", "status", namespaced, claimNames},
		{"", "persistentvolumes", "", clusterScoped, volumeNames},
		{"", "serviceaccounts", "", namespaced, accountNames},
		{"", "serviceaccounts", "token", namespaced, accountNames},
		{"", "endpoints", "", namespaced, endpointNames},
		{"", "services", "", namespaced, miscNames},
		{"", "events", "", namespaced, miscNames},
		{"", "pods", "", namespaced, podNames},
		{"", "pods", "status", namespaced, podNames},
		{"", "pods", "eviction", namespaced, podNames},
		{"", "pods", "binding", namespaced, podNames},
		{"", "pods", "log", namespaced, podNames},
		{"", "pods", "exec", namespaced, podNames},
		{"", "nodes", "", clusterScoped, nodeNames},
		{"", "nodes", "status", clusterScoped, nodeNames},
		{"", "nodes", "proxy", clusterScoped, nodeNames},
		{"", "namespaces", "", clusterScoped, miscNames},
		{"resource.k8s.io", "resourceclaims", "", namespaced, resourceClaims},
		{"resource.k8s.io", "resourceclaims", "status", namespaced, resourceClaims},
		{"resource.k8s.io", "resourceslices", "", clusterScoped, sliceNames},
		{"resource.k8s.io", "resourceslices", "status", clusterScoped, sliceNames},
		{"storage.k8s.io", "volumeattachments", "", clusterScoped, attachments},
		{"storage.k8s.io", "volumeattachments", "status", clusterScoped, attachments},
		{"storage.k8s.io", "csinodes", "", clusterScoped, nodeNames},
		{"storage.k8s.io", "csidrivers", "", clusterScoped, miscNames},
		{"storage.k8s.io", "storageclasses", "", clusterScoped, miscNames},
		{"coordination.k8s.io", "leases", "", leaseScoped, nodeNames},
		{"certificates.k8s.io", "certificatesigningrequests", "", clusterScoped, miscNames},
		{"certificates.k8s.io", "podcertificaterequests", "", namespaced, requestNames},
		{"certificates.k8s.io", "podcertificaterequests", "status", namespaced, requestNames},
		{"events.k8s.io", "events", "", namespaced, miscNames},
		{"authentication.k8s.io", "tokenreviews", "", clusterScoped, miscNames},
		{"authorization.k8s.io", "subjectaccessreviews", "", clusterScoped, miscNames},
		{"node.k8s.io", "runtimeclasses", "", clusterScoped, miscNames},
		{"apps", "deployments", "", namespaced, miscNames},
	}
	var wrongGroup []resourceCase
	for _, c := range cases {
		if c.subresource == "" {
			wrongGroup = append(wrongGroup, resourceCase{"wrong.example.com", c.resource, "", c.scope, c.names})
		}
	}
	return append(cases, wrongGroup...)
}()

type question struct {
	source      string
	user        string
	verb        string
	group       string
	resource    string
	subresource string
	namespace   string
	name        string
	selector    string
	requests    fields.Requirements
	path        string
	scope       scope
}

func (q question) String() string {
	if q.path != "" {
		return fmt.Sprintf("%s %s %s", q.user, q.verb, q.path)
	}
	target := q.resource
	if q.subresource != "" {
		target += "/" + q.subresource
	}
	if q.group != "" {
		target += "." + q.group
	}
	return fmt.Sprintf("%s %s %s ns=%q name=%q selector=%q", q.user, q.verb, target, q.namespace, q.name, q.selector)
}

func (q question) resourceKey() string {
	if q.path != "" {
		return "nonresource:" + q.path
	}
	key := q.resource
	if q.subresource != "" {
		key += "/" + q.subresource
	}
	if q.group != "" {
		key += "." + q.group
	}
	return key
}

func (q question) attributes() authorizer.Attributes {
	var info user.Info
	for _, u := range diffUsers {
		if u.label == q.user {
			info = u.info
		}
	}
	if info == nil {
		switch q.user {
		case "table-node0":
			info = tableNode0
		case "table-nodeunregistered":
			info = tableNodeUnregistered
		}
	}
	if q.path != "" {
		return authorizer.AttributesRecord{User: info, Verb: q.verb, Path: q.path}
	}
	return authorizer.AttributesRecord{
		User: info, ResourceRequest: true, Verb: q.verb, APIGroup: q.group, Resource: q.resource, Subresource: q.subresource,
		Namespace: q.namespace, Name: q.name, FieldSelectorRequirements: q.requests,
	}
}

func (q question) userNode() string {
	switch q.user {
	case "node0", "table-node0":
		return "node0"
	case "node1":
		return "node1"
	}
	return ""
}

func objectNode(name string) string {
	hasNode0, hasNode1 := strings.Contains(name, "node0"), strings.Contains(name, "node1")
	switch {
	case strings.Contains(name, "both"), strings.Contains(name, "shared"):
		return "both"
	case hasNode0 && !hasNode1:
		return "node0"
	case hasNode1 && !hasNode0:
		return "node1"
	}
	return "none"
}

func (q question) ownObject() bool {
	node := q.userNode()
	if node == "" {
		return false
	}
	rel := objectNode(q.name)
	return rel == node || rel == "both"
}

func parseRequirements(selector string) fields.Requirements {
	if selector == "" {
		return nil
	}
	return tableFields(selector)
}

func selectorsFor(u diffUser, c resourceCase, name string) []string {
	if u.node == "" {
		return []string{""}
	}
	if name != "" {
		return []string{"", "spec.nodeName=node0", "metadata.name=" + name}
	}
	selectors := []string{"", "spec.nodeName=node0", "spec.nodeName==node0", "spec.nodeName=node1", "spec.nodeName!=node0"}
	for _, n := range c.names {
		selectors = append(selectors, "metadata.name="+n)
	}
	return selectors
}

func sweepQuestions() []question {
	var out []question
	for _, u := range diffUsers {
		for _, c := range diffResources {
			for _, verb := range diffVerbs {
				for _, namespace := range c.namespaces() {
					for _, name := range append([]string{""}, c.names...) {
						for _, selector := range selectorsFor(u, c, name) {
							out = append(out, question{
								source: "sweep", user: u.label, verb: verb, group: c.group, resource: c.resource, subresource: c.subresource,
								namespace: namespace, name: name, selector: selector, requests: parseRequirements(selector), scope: c.scope,
							})
						}
					}
				}
			}
		}
		for _, path := range []string{"/healthz", "/api", "/metrics", "/logs/x"} {
			for _, verb := range []string{"get", "post"} {
				out = append(out, question{source: "sweep", user: u.label, verb: verb, path: path})
			}
		}
	}
	return out
}

func scopeOf(resource string) scope {
	for _, c := range diffResources {
		if c.resource == resource {
			return c.scope
		}
	}
	return namespaced
}

func tableQuestions() []question {
	var out []question
	for _, attrs := range upstreamTableAttributes {
		label := "table-node0"
		if attrs.User == tableNodeUnregistered {
			label = "table-nodeunregistered"
		}
		selector := ""
		for _, r := range attrs.FieldSelectorRequirements {
			selector += fmt.Sprintf("%s%s%s;", r.Field, r.Operator, r.Value)
		}
		out = append(out, question{
			source: "table", user: label, verb: attrs.Verb, group: attrs.APIGroup, resource: attrs.Resource, subresource: attrs.Subresource,
			namespace: attrs.Namespace, name: attrs.Name, selector: selector, requests: attrs.FieldSelectorRequirements, scope: scopeOf(attrs.Resource),
		})
	}
	return out
}

type answer struct {
	decision authorizer.Decision
	reason   string
	err      error
}

func ask(a authorizer.Authorizer, q question) answer {
	d, reason, err := a.Authorize(context.Background(), q.attributes())
	return answer{d, reason, err}
}

func decisionName(d authorizer.Decision) string {
	switch d {
	case authorizer.DecisionAllow:
		return "Allow"
	case authorizer.DecisionDeny:
		return "Deny"
	}
	return "NoOpinion"
}

type outcome struct {
	q        question
	upstream answer
	project  answer
}

func (o outcome) differs() bool {
	return o.upstream.decision != o.project.decision || (o.upstream.err != nil) != (o.project.err != nil)
}

func (o outcome) direction() string {
	switch {
	case o.project.decision == authorizer.DecisionAllow:
		return "project-allows"
	case o.upstream.decision == authorizer.DecisionAllow:
		return "project-denies"
	}
	return "project-blocks-rbac"
}

func (o outcome) pair() string {
	return decisionName(o.upstream.decision) + "/" + decisionName(o.project.decision)
}

func runOutcomes(t *testing.T) []outcome {
	t.Helper()
	state := combinedState()
	up, proj := state.upstream(), state.project(t)
	questions := append(tableQuestions(), sweepQuestions()...)
	out := make([]outcome, 0, len(questions))
	for _, q := range questions {
		out = append(out, outcome{q: q, upstream: ask(up, q), project: ask(proj, q)})
	}
	return out
}

func TestNodeAuthorizerFeatureGates(t *testing.T) {
	for _, gate := range []featuregate.Feature{
		features.AuthorizeNodeWithSelectors,
		features.KubeletServiceAccountTokenForCredentialProviders,
		features.PodCertificateRequest,
		genericfeatures.AuthorizeWithSelectors,
	} {
		t.Logf("gate %s=%v", gate, utilfeature.DefaultFeatureGate.Enabled(gate))
	}
}
