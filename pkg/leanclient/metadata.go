//go:build js && wasm

package leanclient

import (
	"context"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	apimachinerywatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/metadata"
	restclient "k8s.io/client-go/rest"
)

// MetadataClient implements k8s.io/client-go/metadata.Interface -- the
// PartialObjectMetadata-only client the real upstream garbagecollector
// controller needs to build its ownership graph across every resource
// type, not just the ones this package's generated per-group clients
// (pkg/leanclient/gen/*) cover. Same non-Scheme approach as every other
// client in this package (see leanclient.go's doc comment): plain
// encoding/json over rest.Request.DoRaw/.Stream, reusing this package's
// existing Get/List/Delete/DeleteCollection/Patch/Watch generics
// unchanged, instantiated with T = metav1.PartialObjectMetadata.
// Decoding a full JSON object response into that struct naturally keeps
// only its TypeMeta/ObjectMeta fields (encoding/json silently ignores
// unknown fields), so no server-side "as=PartialObjectMetadata" content
// negotiation is needed -- this apiserver always serves full objects.
type MetadataClient struct {
	cfg *restclient.Config

	mu      sync.Mutex
	clients map[schema.GroupVersion]restclient.Interface
}

var _ metadata.Interface = (*MetadataClient)(nil)

// NewMetadataClient builds a MetadataClient sharing cfg's Transport, the
// same *restclient.Config pkg/controllers.RestConfig already builds for
// every other client in this package.
func NewMetadataClient(cfg *restclient.Config) *MetadataClient {
	return &MetadataClient{cfg: cfg, clients: make(map[schema.GroupVersion]restclient.Interface)}
}

// restClientFor lazily builds (and caches) one *rest.RESTClient per
// GroupVersion, matching every generated pkg/leanclient/gen/<group>
// client's RESTClientFor -- apiPath is "/api" for the legacy core group,
// "/apis" for everything else.
func (c *MetadataClient) restClientFor(gv schema.GroupVersion) (restclient.Interface, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rc, ok := c.clients[gv]; ok {
		return rc, nil
	}
	apiPath := "/apis"
	if gv.Group == "" {
		apiPath = "/api"
	}
	rc, err := RESTClientFor(c.cfg, apiPath, gv)
	if err != nil {
		return nil, err
	}
	c.clients[gv] = rc
	return rc, nil
}

func (c *MetadataClient) Resource(resource schema.GroupVersionResource) metadata.Getter {
	return &metadataResource{client: c, gvr: resource}
}

type metadataResource struct {
	client    *MetadataClient
	gvr       schema.GroupVersionResource
	namespace string
}

var _ metadata.Getter = (*metadataResource)(nil)

func (r *metadataResource) Namespace(ns string) metadata.ResourceInterface {
	cp := *r
	cp.namespace = ns
	return &cp
}

func (r *metadataResource) restClient() (restclient.Interface, error) {
	return r.client.restClientFor(r.gvr.GroupVersion())
}

func (r *metadataResource) Get(ctx context.Context, name string, opts metav1.GetOptions, _ ...string) (*metav1.PartialObjectMetadata, error) {
	rc, err := r.restClient()
	if err != nil {
		return nil, err
	}
	return Get[metav1.PartialObjectMetadata](ctx, rc, r.gvr.Resource, r.namespace, name, opts)
}

func (r *metadataResource) List(ctx context.Context, opts metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
	rc, err := r.restClient()
	if err != nil {
		return nil, err
	}
	return List[metav1.PartialObjectMetadataList](ctx, rc, r.gvr.Resource, r.namespace, opts)
}

func (r *metadataResource) Watch(ctx context.Context, opts metav1.ListOptions) (apimachinerywatch.Interface, error) {
	rc, err := r.restClient()
	if err != nil {
		return nil, err
	}
	return Watch(ctx, rc, r.gvr.Resource, r.namespace, opts, func() *metav1.PartialObjectMetadata { return &metav1.PartialObjectMetadata{} })
}

func (r *metadataResource) Delete(ctx context.Context, name string, opts metav1.DeleteOptions, _ ...string) error {
	rc, err := r.restClient()
	if err != nil {
		return err
	}
	return Delete(ctx, rc, r.gvr.Resource, r.namespace, name, opts)
}

func (r *metadataResource) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	rc, err := r.restClient()
	if err != nil {
		return err
	}
	return DeleteCollection(ctx, rc, r.gvr.Resource, r.namespace, opts, listOpts)
}

func (r *metadataResource) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*metav1.PartialObjectMetadata, error) {
	rc, err := r.restClient()
	if err != nil {
		return nil, err
	}
	return Patch[metav1.PartialObjectMetadata](ctx, rc, r.gvr.Resource, r.namespace, name, pt, data, opts, subresources...)
}
