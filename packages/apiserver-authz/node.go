package authz

import (
	"context"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/auth/nodeidentifier"
	upstreamnode "k8s.io/kubernetes/plugin/pkg/auth/authorizer/node"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
	"k8s.io/utils/ptr"
)

type nodeAuthorizer struct {
	client *kine.Client
	ident  nodeidentifier.NodeIdentifier
	rules  []rbacv1.PolicyRule
}

func newNodeAuthorizer(client *kine.Client) authorizer.Authorizer {
	return &nodeAuthorizer{client: client, ident: nodeidentifier.NewDefaultNodeIdentifier(), rules: bootstrappolicy.NodeRules()}
}

var (
	secretsResource           = schema.GroupResource{Resource: "secrets"}
	configMapsResource        = schema.GroupResource{Resource: "configmaps"}
	claimsResource            = schema.GroupResource{Resource: "persistentvolumeclaims"}
	volumesResource           = schema.GroupResource{Resource: "persistentvolumes"}
	serviceAccountsResource   = schema.GroupResource{Resource: "serviceaccounts"}
	podsResource              = schema.GroupResource{Resource: "pods"}
	resourceClaimsResource    = schema.GroupResource{Group: "resource.k8s.io", Resource: "resourceclaims"}
	resourceSlicesResource    = schema.GroupResource{Group: "resource.k8s.io", Resource: "resourceslices"}
	volumeAttachmentsResource = schema.GroupResource{Group: "storage.k8s.io", Resource: "volumeattachments"}
	resourceSliceCodec        = codecFor(resourcev1.AddToScheme, resourcev1.SchemeGroupVersion)
	volumeAttachmentCodec     = codecFor(storagev1.AddToScheme, storagev1.SchemeGroupVersion)
	coreCodec                 = scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	readVerbs                 = map[string]bool{"get": true, "list": true, "watch": true}
	namedSliceVerbs           = map[string]bool{"get": true, "update": true, "patch": true, "delete": true}
)

func codecFor(add func(*runtime.Scheme) error, version schema.GroupVersion) runtime.Codec {
	s := runtime.NewScheme()
	utilruntime.Must(add(s))
	return serializer.NewCodecFactory(s).LegacyCodec(version)
}

type graphContent struct {
	pods       bool
	volumes    bool
	attachment bool
	slice      bool
}

func graphContentFor(attrs authorizer.Attributes) graphContent {
	if !attrs.IsResourceRequest() || attrs.GetName() == "" {
		return graphContent{}
	}
	switch (schema.GroupResource{Group: attrs.GetAPIGroup(), Resource: attrs.GetResource()}) {
	case secretsResource, volumesResource:
		return graphContent{pods: true, volumes: true}
	case configMapsResource, claimsResource, serviceAccountsResource, resourceClaimsResource:
		return graphContent{pods: true}
	case podsResource:
		return graphContent{pods: attrs.GetSubresource() == "" && readVerbs[attrs.GetVerb()]}
	case volumeAttachmentsResource:
		return graphContent{attachment: true}
	case resourceSlicesResource:
		return graphContent{slice: attrs.GetSubresource() == "" && namedSliceVerbs[attrs.GetVerb()]}
	}
	return graphContent{}
}

func (n *nodeAuthorizer) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	graph := upstreamnode.NewGraph()
	if nodeName, isNode := n.ident.NodeIdentity(attrs.GetUser()); isNode && nodeName != "" && n.client != nil {
		if err := n.load(ctx, graph, nodeName, graphContentFor(attrs), attrs.GetName()); err != nil {
			return authorizer.DecisionNoOpinion, "", err
		}
	}
	return upstreamnode.NewAuthorizer(graph, n.ident, n.rules).Authorize(ctx, attrs)
}

func (n *nodeAuthorizer) load(ctx context.Context, graph *upstreamnode.Graph, nodeName string, content graphContent, name string) error {
	if content.pods {
		if err := n.loadPods(ctx, graph, nodeName); err != nil {
			return err
		}
	}
	if content.volumes {
		if err := n.loadVolumes(ctx, graph); err != nil {
			return err
		}
	}
	if content.attachment {
		attachment := &storagev1.VolumeAttachment{}
		found, err := n.get(ctx, volumeAttachmentCodec, "/volumeattachments/"+name, attachment)
		if err != nil {
			return err
		}
		if found {
			graph.AddVolumeAttachment(attachment.Name, attachment.Spec.NodeName)
		}
	}
	if content.slice {
		slice := &resourcev1.ResourceSlice{}
		found, err := n.get(ctx, resourceSliceCodec, "/resourceslices/"+name, slice)
		if err != nil {
			return err
		}
		if found {
			graph.AddResourceSlice(slice.Name, ptr.Deref(slice.Spec.NodeName, ""))
		}
	}
	return nil
}

func (n *nodeAuthorizer) get(ctx context.Context, codec runtime.Codec, key string, into runtime.Object) (bool, error) {
	s := kine.NewStorage(n.client, codec, func() runtime.Object { return into.DeepCopyObject() })
	err := s.Get(ctx, key, storage.GetOptions{}, into)
	if storage.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (n *nodeAuthorizer) loadPods(ctx context.Context, graph *upstreamnode.Graph, nodeName string) error {
	list := &corev1.PodList{}
	s := kine.NewStorage(n.client, coreCodec, func() runtime.Object { return &corev1.Pod{} })
	if err := s.GetList(ctx, "/pods", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return err
	}
	for i := range list.Items {
		if list.Items[i].Spec.NodeName == nodeName {
			graph.AddPod(&list.Items[i])
		}
	}
	return nil
}

func (n *nodeAuthorizer) loadVolumes(ctx context.Context, graph *upstreamnode.Graph) error {
	list := &corev1.PersistentVolumeList{}
	s := kine.NewStorage(n.client, coreCodec, func() runtime.Object { return &corev1.PersistentVolume{} })
	if err := s.GetList(ctx, "/persistentvolumes", storage.ListOptions{Recursive: true, Predicate: storage.Everything}, list); err != nil {
		return err
	}
	for i := range list.Items {
		graph.AddPV(&list.Items[i])
	}
	return nil
}
