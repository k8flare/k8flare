//go:build js && wasm

package leanclient

import (
	"context"
	"encoding/json"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	restclient "k8s.io/client-go/rest"
)

// objectKinder is satisfied by every k8s.io/api object (via its embedded
// metav1.TypeMeta) -- used below to stamp Kind/APIVersion onto the object
// before marshaling it.
type objectKinder interface {
	GetObjectKind() schema.ObjectKind
}

// setTypeMeta stamps gvk onto obj if it implements objectKinder. This
// package deliberately never routes objects through a runtime.Scheme (see
// leanclient.go's doc comment), so nothing else fills in Kind/APIVersion --
// without this, encoding/json omits both (metav1.TypeMeta's fields are
// `omitempty`, and freshly-constructed controller objects never set them
// themselves, same as real client-go's generated clients, which get GVK
// injected by their scheme-aware encoder instead). The real k8flare
// apiserver's decoder requires both fields and 400s without them --
// confirmed against the ReplicaSet/Event write paths, see
// spikes/s14-loader-external-fetch/FINDINGS.md Part 7.
func setTypeMeta[T any](obj *T, gvk schema.GroupVersionKind) {
	if k, ok := any(obj).(objectKinder); ok {
		k.GetObjectKind().SetGroupVersionKind(gvk)
	}
}

// Get issues a GET for resource/namespace/name and decodes the response
// body directly into T via encoding/json -- no runtime.Scheme involved
// (see leanclient.go's doc comment).
func Get[T any](ctx context.Context, c restclient.Interface, resource, namespace, name string, opts metav1.GetOptions) (*T, error) {
	req := c.Get().Resource(resource).Name(name)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	setGetParams(req, opts)
	return doInto[T](ctx, req)
}

// List issues a GET against resource's collection endpoint and decodes
// into L (a list type, e.g. *corev1.PodList).
func List[L any](ctx context.Context, c restclient.Interface, resource, namespace string, opts metav1.ListOptions) (*L, error) {
	req := c.Get().Resource(resource)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	setListParams(req, opts)
	return doInto[L](ctx, req)
}

// Create issues a POST with obj's JSON encoding as the body. gvk is
// stamped onto obj first (see setTypeMeta).
func Create[T any](ctx context.Context, c restclient.Interface, resource, namespace string, obj *T, gvk schema.GroupVersionKind, opts metav1.CreateOptions) (*T, error) {
	setTypeMeta(obj, gvk)
	body, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	req := c.Post().Resource(resource).Body(body)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	setCreateParams(req, opts)
	return doInto[T](ctx, req)
}

// Update issues a PUT with obj's JSON encoding as the body, matching
// gentype's own convention: name is not taken as a separate parameter,
// obj must already carry its own name via ObjectMeta -- callers get name
// from the object they're updating, so requiring an accessor interface
// here would add generic-constraint complexity for no real benefit.
func Update[T any](ctx context.Context, c restclient.Interface, resource, namespace, name string, obj *T, gvk schema.GroupVersionKind, opts metav1.UpdateOptions) (*T, error) {
	return updateSubresource[T](ctx, c, resource, namespace, name, "", obj, gvk, opts)
}

// UpdateSubresource issues a PUT to resource/namespace/name/subresource
// (e.g. subresource "status" for UpdateStatus).
func UpdateSubresource[T any](ctx context.Context, c restclient.Interface, resource, namespace, name, subresource string, obj *T, gvk schema.GroupVersionKind, opts metav1.UpdateOptions) (*T, error) {
	return updateSubresource[T](ctx, c, resource, namespace, name, subresource, obj, gvk, opts)
}

func updateSubresource[T any](ctx context.Context, c restclient.Interface, resource, namespace, name, subresource string, obj *T, gvk schema.GroupVersionKind, opts metav1.UpdateOptions) (*T, error) {
	setTypeMeta(obj, gvk)
	body, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	req := c.Put().Resource(resource).Name(name).Body(body)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	if subresource != "" {
		req = req.SubResource(subresource)
	}
	setUpdateParams(req, opts)
	return doInto[T](ctx, req)
}

// Delete issues a DELETE for resource/namespace/name.
func Delete(ctx context.Context, c restclient.Interface, resource, namespace, name string, opts metav1.DeleteOptions) error {
	// Stamp TypeMeta: plain encoding/json (unlike client-go's codec path)
	// would otherwise emit a body without kind/apiVersion, which the
	// apiserver's strict decoder rejects with 400 "Object 'Kind' is
	// missing" -- found live when the real replicaset controller's
	// UID-preconditioned pod deletes all failed during scale-down.
	opts.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "DeleteOptions"}
	body, err := json.Marshal(&opts)
	if err != nil {
		return err
	}
	req := c.Delete().Resource(resource).Name(name).Body(body)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	_, err = req.DoRaw(ctx)
	return err
}

// DeleteCollection issues a DELETE against resource's collection endpoint.
func DeleteCollection(ctx context.Context, c restclient.Interface, resource, namespace string, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	opts.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "DeleteOptions"} // same reason as Delete above
	body, err := json.Marshal(&opts)
	if err != nil {
		return err
	}
	req := c.Delete().Resource(resource).Body(body)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	setListParams(req, listOpts)
	_, err = req.DoRaw(ctx)
	return err
}

// Patch issues a PATCH with the given patch type and raw data.
func Patch[T any](ctx context.Context, c restclient.Interface, resource, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*T, error) {
	req := c.Patch(pt).Resource(resource).Name(name).SubResource(subresources...).Body(data)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	setPatchParams(req, opts)
	return doInto[T](ctx, req)
}

func doInto[T any](ctx context.Context, req *restclient.Request) (*T, error) {
	body, err := req.DoRaw(ctx)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func setListParams(req *restclient.Request, opts metav1.ListOptions) {
	if opts.LabelSelector != "" {
		req.Param("labelSelector", opts.LabelSelector)
	}
	if opts.FieldSelector != "" {
		req.Param("fieldSelector", opts.FieldSelector)
	}
	if opts.Watch {
		req.Param("watch", "true")
	}
	if opts.AllowWatchBookmarks {
		req.Param("allowWatchBookmarks", "true")
	}
	if opts.ResourceVersion != "" {
		req.Param("resourceVersion", opts.ResourceVersion)
	}
	if opts.ResourceVersionMatch != "" {
		req.Param("resourceVersionMatch", string(opts.ResourceVersionMatch))
	}
	if opts.TimeoutSeconds != nil {
		req.Param("timeoutSeconds", strconv.FormatInt(*opts.TimeoutSeconds, 10))
	}
	if opts.Limit != 0 {
		req.Param("limit", strconv.FormatInt(opts.Limit, 10))
	}
	if opts.Continue != "" {
		req.Param("continue", opts.Continue)
	}
	if opts.SendInitialEvents != nil {
		req.Param("sendInitialEvents", strconv.FormatBool(*opts.SendInitialEvents))
	}
}

func setGetParams(req *restclient.Request, opts metav1.GetOptions) {
	if opts.ResourceVersion != "" {
		req.Param("resourceVersion", opts.ResourceVersion)
	}
}

func setCreateParams(req *restclient.Request, opts metav1.CreateOptions) {
	for _, dr := range opts.DryRun {
		req.Param("dryRun", dr)
	}
	if opts.FieldManager != "" {
		req.Param("fieldManager", opts.FieldManager)
	}
	if opts.FieldValidation != "" {
		req.Param("fieldValidation", opts.FieldValidation)
	}
}

func setUpdateParams(req *restclient.Request, opts metav1.UpdateOptions) {
	for _, dr := range opts.DryRun {
		req.Param("dryRun", dr)
	}
	if opts.FieldManager != "" {
		req.Param("fieldManager", opts.FieldManager)
	}
	if opts.FieldValidation != "" {
		req.Param("fieldValidation", opts.FieldValidation)
	}
}

func setPatchParams(req *restclient.Request, opts metav1.PatchOptions) {
	for _, dr := range opts.DryRun {
		req.Param("dryRun", dr)
	}
	if opts.Force != nil && *opts.Force {
		req.Param("force", "true")
	}
	if opts.FieldManager != "" {
		req.Param("fieldManager", opts.FieldManager)
	}
	if opts.FieldValidation != "" {
		req.Param("fieldValidation", opts.FieldValidation)
	}
}
