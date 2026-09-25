package authz

import (
	"context"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/auth/nodeidentifier"
	rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

type nodeAuthorizer struct {
	client *kine.Client
	ident  nodeidentifier.NodeIdentifier
	rules  []rbacv1.PolicyRule
}

func newNodeAuthorizer(client *kine.Client) authorizer.Authorizer {
	return &nodeAuthorizer{client: client, ident: nodeidentifier.NewDefaultNodeIdentifier(), rules: bootstrappolicy.NodeRules()}
}

func (n *nodeAuthorizer) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	nodeName, isNode := n.ident.NodeIdentity(attrs.GetUser())
	if !isNode {
		return authorizer.DecisionNoOpinion, "", nil
	}
	if nodeName == "" {
		return authorizer.DecisionNoOpinion, "unknown node", nil
	}
	if attrs.IsResourceRequest() {
		switch attrs.GetResource() {
		case "nodes":
			return authorizeOwnName(nodeName, attrs)
		case "pods":
			return n.authorizePod(ctx, nodeName, attrs)
		case "leases":
			return authorizeLease(nodeName, attrs)
		case "secrets", "configmaps", "persistentvolumeclaims":
			return n.authorizeRelated(ctx, nodeName, attrs)
		case "resourceclaims":
			if attrs.GetVerb() != "get" {
				return authorizer.DecisionDeny, "node can only read resource claims referenced by its pods", nil
			}
			return n.authorizeRelated(ctx, nodeName, attrs)
		case "persistentvolumes":
			return n.authorizePersistentVolume(ctx, nodeName, attrs)
		case "volumeattachments":
			return n.authorizeVolumeAttachment(ctx, nodeName, attrs)
		case "serviceaccounts":
			if attrs.GetSubresource() == "token" && attrs.GetVerb() == "create" {
				return authorizer.DecisionAllow, "", nil
			}
			if attrs.GetVerb() != "get" {
				return authorizer.DecisionDeny, "node can only read service accounts referenced by its pods", nil
			}
			return n.authorizeRelated(ctx, nodeName, attrs)
		case "endpoints":
			return n.authorizeEndpoints(ctx, nodeName, attrs)
		case "resourceslices":
			return n.authorizeResourceSlice(ctx, nodeName, attrs)
		case "csinodes":
			return authorizeCSINode(nodeName, attrs)
		}
	}
	if rbacauthorizer.RulesAllow(attrs, n.rules...) {
		return authorizer.DecisionAllow, "", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}

func authorizeOwnName(nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	switch attrs.GetSubresource() {
	case "", "status":
	default:
		return authorizer.DecisionNoOpinion, "", nil
	}
	switch attrs.GetVerb() {
	case "create", "update", "patch":
		if attrs.GetName() == "" || attrs.GetName() == nodeName {
			return authorizer.DecisionAllow, "", nil
		}
	case "get":
		if attrs.GetName() == nodeName {
			return authorizer.DecisionAllow, "", nil
		}
	case "list", "watch":
		if nameFromAttrs(attrs) == nodeName {
			return authorizer.DecisionAllow, "", nil
		}
		return authorizer.DecisionNoOpinion, "node can only read its own Node object", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}

var resourceSliceCodec = func() runtime.Codec {
	s := runtime.NewScheme()
	utilruntime.Must(resourcev1.AddToScheme(s))
	return serializer.NewCodecFactory(s).LegacyCodec(resourcev1.SchemeGroupVersion)
}()

func (n *nodeAuthorizer) authorizeResourceSlice(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetSubresource() != "" {
		return authorizer.DecisionNoOpinion, "cannot authorize ResourceSlice subresources", nil
	}
	switch attrs.GetVerb() {
	case "create":
		return authorizer.DecisionAllow, "", nil
	case "get", "update", "patch", "delete":
		return n.authorizeNamedResourceSlice(ctx, nodeName, attrs)
	case "list", "watch", "deletecollection":
		if hasNodeNameSelector(attrs, nodeName) {
			return authorizer.DecisionAllow, "", nil
		}
		return authorizer.DecisionNoOpinion, "can only list/watch/deletecollection resourceslices with nodeName field selector", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}

func (n *nodeAuthorizer) authorizeNamedResourceSlice(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetName() == "" || n.client == nil {
		return authorizer.DecisionNoOpinion, "can only access ResourceSlice for this node", nil
	}
	slice, err := n.getResourceSlice(ctx, attrs.GetName())
	if err != nil || slice.Spec.NodeName == nil || *slice.Spec.NodeName != nodeName {
		return authorizer.DecisionNoOpinion, "can only access ResourceSlice for this node", nil
	}
	return authorizer.DecisionAllow, "", nil
}

func (n *nodeAuthorizer) getResourceSlice(ctx context.Context, name string) (*resourcev1.ResourceSlice, error) {
	s := kine.NewStorage(n.client, resourceSliceCodec, func() runtime.Object { return &resourcev1.ResourceSlice{} })
	slice := &resourcev1.ResourceSlice{}
	if err := s.Get(ctx, "/resourceslices/"+name, storage.GetOptions{}, slice); err != nil {
		return nil, err
	}
	return slice, nil
}

func authorizeCSINode(nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetSubresource() != "" {
		return authorizer.DecisionNoOpinion, "cannot authorize CSINode subresources", nil
	}
	switch attrs.GetVerb() {
	case "create":
		return authorizer.DecisionAllow, "", nil
	case "get", "update", "patch", "delete":
		if attrs.GetName() == nodeName {
			return authorizer.DecisionAllow, "", nil
		}
		return authorizer.DecisionDeny, "can only access CSINode with the same name as the requesting node", nil
	}
	return authorizer.DecisionNoOpinion, "can only get, create, update, patch, or delete a CSINode", nil
}

func authorizeLease(nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetNamespace() != "kube-node-lease" {
		return authorizer.DecisionNoOpinion, "", nil
	}
	name := attrs.GetName()
	if name == "" {
		name = nameFromAttrs(attrs)
	}
	switch attrs.GetVerb() {
	case "create":
		return authorizer.DecisionAllow, "", nil
	case "get", "update", "patch", "delete", "list", "watch":
		if name == nodeName {
			return authorizer.DecisionAllow, "", nil
		}
		return authorizer.DecisionNoOpinion, "can only access node lease with the same name as the requesting node", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}

func (n *nodeAuthorizer) authorizePod(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	switch attrs.GetSubresource() {
	case "":
		switch attrs.GetVerb() {
		case "get":
			return n.podOnNode(ctx, nodeName, attrs)
		case "list", "watch":
			if hasNodeNameSelector(attrs, nodeName) {
				return authorizer.DecisionAllow, "", nil
			}
			if attrs.GetName() != "" {
				return n.podOnNode(ctx, nodeName, attrs)
			}
			return authorizer.DecisionNoOpinion, "can only list/watch pods with spec.nodeName field selector", nil
		case "create", "delete":
			return authorizer.DecisionAllow, "", nil
		}
	case "status":
		switch attrs.GetVerb() {
		case "update", "patch":
			return authorizer.DecisionAllow, "", nil
		}
	case "eviction":
		if attrs.GetVerb() == "create" {
			return authorizer.DecisionAllow, "", nil
		}
	}
	return authorizer.DecisionNoOpinion, "", nil
}

func hasNodeNameSelector(attrs authorizer.Attributes, nodeName string) bool {
	reqs, err := attrs.GetFieldSelector()
	if err != nil {
		return false
	}
	for _, req := range reqs {
		if req.Field == "spec.nodeName" && req.Operator == selection.Equals && req.Value == nodeName {
			return true
		}
	}
	return false
}

func (n *nodeAuthorizer) podOnNode(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetName() == "" || attrs.GetNamespace() == "" {
		return authorizer.DecisionNoOpinion, "", nil
	}
	pod, err := n.getPod(ctx, attrs.GetNamespace(), attrs.GetName())
	if err != nil || pod == nil {
		return authorizer.DecisionNoOpinion, "", nil
	}
	if pod.Spec.NodeName == nodeName {
		return authorizer.DecisionAllow, "", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}

func (n *nodeAuthorizer) authorizeRelated(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	switch attrs.GetVerb() {
	case "get", "list", "watch":
	case "update", "patch":
		if attrs.GetResource() != "persistentvolumeclaims" || attrs.GetSubresource() != "status" {
			return authorizer.DecisionDeny, "node cannot write related objects", nil
		}
	default:
		return authorizer.DecisionDeny, "node cannot write related objects", nil
	}
	name := nameFromAttrs(attrs)
	if name == "" {
		if attrs.GetVerb() == "get" || attrs.GetNamespace() == "" {
			return authorizer.DecisionDeny, "node can only read named objects referenced by its pods", nil
		}
		return authorizer.DecisionAllow, "", nil
	}
	if attrs.GetNamespace() == "" {
		return authorizer.DecisionDeny, "node can only read named objects referenced by its pods", nil
	}
	if n.client == nil {
		return authorizer.DecisionDeny, "node graph unavailable", nil
	}
	pods, err := n.podsOnNode(ctx, nodeName)
	if err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}
	for _, pod := range pods {
		if podReferences(pod, attrs.GetResource(), attrs.GetNamespace(), name) {
			return authorizer.DecisionAllow, "", nil
		}
	}
	return authorizer.DecisionDeny, "object not referenced by a pod on this node", nil
}

var volumeAttachmentCodec = func() runtime.Codec {
	s := runtime.NewScheme()
	utilruntime.Must(storagev1.AddToScheme(s))
	return serializer.NewCodecFactory(s).LegacyCodec(storagev1.SchemeGroupVersion)
}()

func (n *nodeAuthorizer) authorizeVolumeAttachment(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetVerb() != "get" {
		return authorizer.DecisionDeny, "node cannot write related objects", nil
	}
	if attrs.GetName() == "" || n.client == nil {
		return authorizer.DecisionDeny, "node graph unavailable", nil
	}
	s := kine.NewStorage(n.client, volumeAttachmentCodec, func() runtime.Object { return &storagev1.VolumeAttachment{} })
	va := &storagev1.VolumeAttachment{}
	if err := s.Get(ctx, "/volumeattachments/"+attrs.GetName(), storage.GetOptions{}, va); err != nil || va.Spec.NodeName != nodeName {
		return authorizer.DecisionDeny, "object not referenced by a pod on this node", nil
	}
	return authorizer.DecisionAllow, "", nil
}

func (n *nodeAuthorizer) authorizePersistentVolume(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetVerb() != "get" {
		return authorizer.DecisionDeny, "node cannot write related objects", nil
	}
	if attrs.GetName() == "" {
		return authorizer.DecisionDeny, "node can only read named objects referenced by its pods", nil
	}
	if n.client == nil {
		return authorizer.DecisionDeny, "node graph unavailable", nil
	}
	pods, err := n.podsOnNode(ctx, nodeName)
	if err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}
	for _, pod := range pods {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil {
				continue
			}
			pvc, err := n.getPVC(ctx, pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
			if err != nil || pvc == nil || pvc.Spec.VolumeName != attrs.GetName() {
				continue
			}
			return authorizer.DecisionAllow, "", nil
		}
	}
	return authorizer.DecisionDeny, "object not referenced by a pod on this node", nil
}

func (n *nodeAuthorizer) authorizeEndpoints(ctx context.Context, nodeName string, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetVerb() != "get" {
		return authorizer.DecisionDeny, "node can only read endpoints referenced by its pods", nil
	}
	name := attrs.GetName()
	ns := attrs.GetNamespace()
	if name == "" || ns == "" {
		return authorizer.DecisionDeny, "node can only read named objects referenced by its pods", nil
	}
	if n.client == nil {
		return authorizer.DecisionDeny, "node graph unavailable", nil
	}
	pods, err := n.podsOnNode(ctx, nodeName)
	if err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}
	for _, pod := range pods {
		if podEndpointNames(pod, ns)[name] {
			return authorizer.DecisionAllow, "", nil
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil || vol.PersistentVolumeClaim.ClaimName == "" {
				continue
			}
			pvc, err := n.getPVC(ctx, pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
			if err != nil || pvc == nil || pvc.Spec.VolumeName == "" {
				continue
			}
			pv, err := n.getPV(ctx, pvc.Spec.VolumeName)
			if err != nil || pv == nil || pv.Spec.Glusterfs == nil || pv.Spec.Glusterfs.EndpointsName == "" {
				continue
			}
			ens := pod.Namespace
			if pv.Spec.Glusterfs.EndpointsNamespace != nil && *pv.Spec.Glusterfs.EndpointsNamespace != "" {
				ens = *pv.Spec.Glusterfs.EndpointsNamespace
			}
			if ens == ns && pv.Spec.Glusterfs.EndpointsName == name {
				return authorizer.DecisionAllow, "", nil
			}
		}
	}
	return authorizer.DecisionDeny, "object not referenced by a pod on this node", nil
}

func (n *nodeAuthorizer) getPV(ctx context.Context, name string) (*corev1.PersistentVolume, error) {
	s := kine.NewStorage(n.client, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return &corev1.PersistentVolume{} })
	pv := &corev1.PersistentVolume{}
	if err := s.Get(ctx, "/persistentvolumes/"+name, storage.GetOptions{}, pv); err != nil {
		return nil, err
	}
	return pv, nil
}

func (n *nodeAuthorizer) getPVC(ctx context.Context, ns, name string) (*corev1.PersistentVolumeClaim, error) {
	s := kine.NewStorage(n.client, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return &corev1.PersistentVolumeClaim{} })
	pvc := &corev1.PersistentVolumeClaim{}
	if err := s.Get(ctx, "/persistentvolumeclaims/"+ns+"/"+name, storage.GetOptions{}, pvc); err != nil {
		return nil, err
	}
	return pvc, nil
}

func nameFromAttrs(attrs authorizer.Attributes) string {
	if attrs.GetName() != "" {
		return attrs.GetName()
	}
	reqs, err := attrs.GetFieldSelector()
	if err != nil {
		return ""
	}
	for _, req := range reqs {
		if req.Field == "metadata.name" && req.Operator == selection.Equals {
			return req.Value
		}
	}
	return ""
}

func (n *nodeAuthorizer) getPod(ctx context.Context, ns, name string) (*corev1.Pod, error) {
	s := kine.NewStorage(n.client, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return &corev1.Pod{} })
	pod := &corev1.Pod{}
	if err := s.Get(ctx, "/pods/"+ns+"/"+name, storage.GetOptions{}, pod); err != nil {
		return nil, err
	}
	return pod, nil
}

func (n *nodeAuthorizer) podsOnNode(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	list := &corev1.PodList{}
	s := kine.NewStorage(n.client, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return &corev1.Pod{} })
	if err := s.GetList(ctx, "/pods", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return nil, err
	}
	var out []corev1.Pod
	for _, p := range list.Items {
		if p.Spec.NodeName == nodeName {
			out = append(out, p)
		}
	}
	return out, nil
}

func podReferences(pod corev1.Pod, resource, ns, name string) bool {
	if resource == "endpoints" {
		return podEndpointNames(pod, ns)[name]
	}
	if pod.Namespace != ns {
		return false
	}
	switch resource {
	case "secrets":
		return podSecretNames(pod)[name]
	case "configmaps":
		return podConfigMapNames(pod)[name]
	case "persistentvolumeclaims":
		return podClaimNames(pod)[name]
	case "resourceclaims":
		return podResourceClaimNames(pod)[name]
	case "serviceaccounts":
		account := pod.Spec.ServiceAccountName
		if account == "" {
			account = "default"
		}
		return account == name
	}
	return false
}

func podResourceClaimNames(pod corev1.Pod) map[string]bool {
	out := map[string]bool{}
	for _, claim := range pod.Spec.ResourceClaims {
		if claim.ResourceClaimName != nil {
			out[*claim.ResourceClaimName] = true
		}
	}
	for _, status := range pod.Status.ResourceClaimStatuses {
		if status.ResourceClaimName != nil {
			out[*status.ResourceClaimName] = true
		}
	}
	if pod.Status.ExtendedResourceClaimStatus != nil && pod.Status.ExtendedResourceClaimStatus.ResourceClaimName != "" {
		out[pod.Status.ExtendedResourceClaimStatus.ResourceClaimName] = true
	}
	return out
}

func podSecretNames(pod corev1.Pod) map[string]bool {
	out := map[string]bool{}
	for _, s := range pod.Spec.ImagePullSecrets {
		out[s.Name] = true
	}
	for _, c := range append(append([]corev1.Container{}, pod.Spec.Containers...), pod.Spec.InitContainers...) {
		for _, e := range c.Env {
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
				out[e.ValueFrom.SecretKeyRef.Name] = true
			}
		}
		for _, e := range c.EnvFrom {
			if e.SecretRef != nil {
				out[e.SecretRef.Name] = true
			}
		}
	}
	for _, v := range pod.Spec.Volumes {
		if v.Secret != nil {
			out[v.Secret.SecretName] = true
		}
		if v.Projected != nil {
			for _, src := range v.Projected.Sources {
				if src.Secret != nil {
					out[src.Secret.Name] = true
				}
			}
		}
	}
	return out
}

func podConfigMapNames(pod corev1.Pod) map[string]bool {
	out := map[string]bool{}
	for _, c := range append(append([]corev1.Container{}, pod.Spec.Containers...), pod.Spec.InitContainers...) {
		for _, e := range c.Env {
			if e.ValueFrom != nil && e.ValueFrom.ConfigMapKeyRef != nil {
				out[e.ValueFrom.ConfigMapKeyRef.Name] = true
			}
		}
		for _, e := range c.EnvFrom {
			if e.ConfigMapRef != nil {
				out[e.ConfigMapRef.Name] = true
			}
		}
	}
	for _, v := range pod.Spec.Volumes {
		if v.ConfigMap != nil {
			out[v.ConfigMap.Name] = true
		}
		if v.Projected != nil {
			for _, src := range v.Projected.Sources {
				if src.ConfigMap != nil {
					out[src.ConfigMap.Name] = true
				}
			}
		}
	}
	return out
}

func podEndpointNames(pod corev1.Pod, ns string) map[string]bool {
	out := map[string]bool{}
	for _, vol := range pod.Spec.Volumes {
		if vol.Glusterfs == nil || vol.Glusterfs.EndpointsName == "" || pod.Namespace != ns {
			continue
		}
		out[vol.Glusterfs.EndpointsName] = true
	}
	return out
}

func podClaimNames(pod corev1.Pod) map[string]bool {
	out := map[string]bool{}
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			out[v.PersistentVolumeClaim.ClaimName] = true
		}
	}
	return out
}
