package apiserver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ResourceStore handles CRUD for a single resource type (e.g. configmaps).
// It bridges between Kubernetes runtime.Object types and the raw byte Storage layer.
type ResourceStore struct {
	storage      *Storage
	resource     string // e.g. "configmaps"
	namespaced   bool
	newFunc      func() runtime.Object // creates a new empty object (e.g. &corev1.ConfigMap{})
	newListFunc  func() runtime.Object // creates a new empty list object (e.g. &corev1.ConfigMapList{})
	setItemsFunc func(list runtime.Object, items []runtime.Object)
}

// NewResourceStore creates a ResourceStore for the given resource type.
func NewResourceStore(
	storage *Storage,
	resource string,
	namespaced bool,
	newFunc, newListFunc func() runtime.Object,
	setItemsFunc func(list runtime.Object, items []runtime.Object),
) *ResourceStore {
	return &ResourceStore{
		storage:      storage,
		resource:     resource,
		namespaced:   namespaced,
		newFunc:      newFunc,
		newListFunc:  newListFunc,
		setItemsFunc: setItemsFunc,
	}
}

// storageKey builds the storage key for the given resource.
// Namespaced: /{resource}/{namespace}/{name}
// Cluster-scoped: /{resource}/{name}
func (rs *ResourceStore) storageKey(namespace, name string) string {
	if rs.namespaced {
		return "/" + rs.resource + "/" + namespace + "/" + name
	}
	return "/" + rs.resource + "/" + name
}

// storagePrefix builds the storage prefix for listing resources.
// Namespaced with namespace: /{resource}/{namespace}/
// Namespaced without namespace (all namespaces): /{resource}/
// Cluster-scoped: /{resource}/
func (rs *ResourceStore) storagePrefix(namespace string) string {
	if rs.namespaced && namespace != "" {
		return "/" + rs.resource + "/" + namespace + "/"
	}
	return "/" + rs.resource + "/"
}

// generateUID produces a simple UID string. This is not RFC 4122 compliant
// but is sufficient for this minimal implementation.
func generateUID() types.UID {
	return types.UID(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Int31(),
		rand.Int31n(0xffff),
		rand.Int31n(0xffff),
		rand.Int31n(0xffff),
		rand.Int63n(0xffffffffffff),
	))
}

// getObjectMeta extracts the ObjectMeta from a runtime.Object via the ObjectMetaAccessor interface.
func getObjectMeta(obj runtime.Object) *metav1.ObjectMeta {
	accessor, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		return nil
	}
	return accessor.GetObjectMeta().(*metav1.ObjectMeta)
}

// setResourceVersion sets the ResourceVersion field on the object to the given revision.
func setResourceVersion(obj runtime.Object, revision int64) {
	meta := getObjectMeta(obj)
	if meta != nil {
		meta.ResourceVersion = strconv.FormatInt(revision, 10)
	}
}

// notFoundStatus returns a metav1.Status indicating the resource was not found.
func notFoundStatus(resource, name string) *metav1.Status {
	return &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Message:  fmt.Sprintf("%s %q not found", resource, name),
		Reason:   metav1.StatusReasonNotFound,
		Code:     404,
	}
}

// conflictStatus returns a metav1.Status indicating a resource version conflict.
func conflictStatus(resource, name string) *metav1.Status {
	return &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Message:  fmt.Sprintf("operation cannot be fulfilled on %s %q: the object has been modified", resource, name),
		Reason:   metav1.StatusReasonConflict,
		Code:     409,
	}
}

// alreadyExistsStatus returns a metav1.Status indicating the resource already exists.
func alreadyExistsStatus(resource, name string) *metav1.Status {
	return &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Message:  fmt.Sprintf("%s %q already exists", resource, name),
		Reason:   metav1.StatusReasonAlreadyExists,
		Code:     409,
	}
}

// StatusError wraps a metav1.Status as an error, allowing callers to inspect
// the structured Kubernetes status response.
type StatusError struct {
	Status *metav1.Status
}

func (e *StatusError) Error() string {
	return e.Status.Message
}

// Get retrieves a single resource from storage by namespace and name.
func (rs *ResourceStore) Get(ctx context.Context, namespace, name string) (runtime.Object, error) {
	key := rs.storageKey(namespace, name)
	stored, err := rs.storage.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, &StatusError{Status: notFoundStatus(rs.resource, name)}
		}
		return nil, fmt.Errorf("store get: %w", err)
	}

	obj := rs.newFunc()
	if err := DecodeFromStorage(stored.Value, obj); err != nil {
		return nil, fmt.Errorf("store get: decode: %w", err)
	}
	setResourceVersion(obj, stored.ModRevision)
	return obj, nil
}

// List retrieves all resources matching the given namespace (empty string for all namespaces
// or cluster-scoped resources). Results are filtered by fieldSelector if non-empty.
func (rs *ResourceStore) List(ctx context.Context, namespace string, fieldSelector string) (runtime.Object, error) {
	prefix := rs.storagePrefix(namespace)
	storedObjects, rev, err := rs.storage.List(ctx, prefix, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("store list: %w", err)
	}

	items := make([]runtime.Object, 0, len(storedObjects))
	for _, stored := range storedObjects {
		obj := rs.newFunc()
		if err := DecodeFromStorage(stored.Value, obj); err != nil {
			return nil, fmt.Errorf("store list: decode item: %w", err)
		}
		setResourceVersion(obj, stored.ModRevision)
		items = append(items, obj)
	}

	items = applyFieldSelector(items, fieldSelector)

	listObj := rs.newListFunc()
	rs.setItemsFunc(listObj, items)

	// Set list-level resourceVersion for watch continuation
	if accessor, ok := listObj.(metav1.ListMetaAccessor); ok {
		accessor.GetListMeta().SetResourceVersion(strconv.FormatInt(rev, 10))
	}

	return listObj, nil
}

// applyFieldSelector filters a list of runtime.Object by the given fieldSelector string.
// The fieldSelector format is "field1=value1,field2=value2".
func applyFieldSelector(items []runtime.Object, fieldSelector string) []runtime.Object {
	if fieldSelector == "" {
		return items
	}
	selectors := strings.Split(fieldSelector, ",")
	var filtered []runtime.Object
	for _, item := range items {
		if matchesFieldSelector(item, selectors) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// matchesFieldSelector checks whether a single object matches all field selectors.
// Returns false if any selector does not match.
func matchesFieldSelector(obj runtime.Object, selectors []string) bool {
	meta := getObjectMeta(obj)
	for _, sel := range selectors {
		parts := strings.SplitN(sel, "=", 2)
		if len(parts) != 2 {
			continue
		}
		field, value := parts[0], parts[1]
		switch field {
		case "metadata.name":
			if meta != nil && meta.Name != value {
				return false
			}
		case "metadata.namespace":
			if meta != nil && meta.Namespace != value {
				return false
			}
		case "spec.nodeName":
			if pod, ok := obj.(*corev1.Pod); ok {
				if pod.Spec.NodeName != value {
					return false
				}
			}
		case "status.phase":
			if pod, ok := obj.(*corev1.Pod); ok {
				if string(pod.Status.Phase) != value {
					return false
				}
			}
		}
	}
	return true
}

// Create stores a new resource in storage. It sets UID, creation timestamp,
// namespace, and resource version on the object.
func (rs *ResourceStore) Create(ctx context.Context, namespace string, obj runtime.Object) (runtime.Object, error) {
	meta := getObjectMeta(obj)
	if meta == nil {
		return nil, fmt.Errorf("store create: object does not implement ObjectMetaAccessor")
	}

	name := meta.Name
	if name == "" {
		return nil, fmt.Errorf("store create: name is required")
	}

	// Set metadata for creation
	meta.UID = generateUID()
	meta.CreationTimestamp = metav1.Now()
	if rs.namespaced {
		meta.Namespace = namespace
	}
	// Clear resource version before encoding for storage
	meta.ResourceVersion = ""

	data, err := EncodeToStorage(obj)
	if err != nil {
		return nil, fmt.Errorf("store create: encode: %w", err)
	}

	key := rs.storageKey(namespace, name)
	revision, err := rs.storage.Create(ctx, key, data)
	if err != nil {
		if errors.Is(err, ErrKeyExists) {
			return nil, &StatusError{Status: alreadyExistsStatus(rs.resource, name)}
		}
		return nil, fmt.Errorf("store create: %w", err)
	}

	setResourceVersion(obj, revision)
	return obj, nil
}

// Update performs a compare-and-swap update on an existing resource.
// If the object's ResourceVersion is set, it must match the current stored
// revision. An empty ResourceVersion means an unconditional update, matching
// Kubernetes' behavior for updates that omit resourceVersion: the current
// stored revision is fetched and used as the CAS token.
func (rs *ResourceStore) Update(ctx context.Context, namespace, name string, obj runtime.Object) (runtime.Object, error) {
	meta := getObjectMeta(obj)
	if meta == nil {
		return nil, fmt.Errorf("store update: object does not implement ObjectMetaAccessor")
	}

	key := rs.storageKey(namespace, name)

	var currentRevision int64
	if meta.ResourceVersion == "" {
		stored, err := rs.storage.Get(ctx, key)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, &StatusError{Status: notFoundStatus(rs.resource, name)}
			}
			return nil, fmt.Errorf("store update: get current: %w", err)
		}
		currentRevision = stored.ModRevision
	} else {
		// Parse the resource version from the incoming object for CAS
		var err error
		currentRevision, err = strconv.ParseInt(meta.ResourceVersion, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("store update: invalid resource version %q: %w", meta.ResourceVersion, err)
		}
	}

	// Clear resource version before encoding for storage
	meta.ResourceVersion = ""

	data, err := EncodeToStorage(obj)
	if err != nil {
		return nil, fmt.Errorf("store update: encode: %w", err)
	}

	newRevision, updated, err := rs.storage.Update(ctx, key, data, currentRevision)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil, &StatusError{Status: conflictStatus(rs.resource, name)}
		}
		return nil, fmt.Errorf("store update: %w", err)
	}

	if !updated {
		return nil, &StatusError{Status: conflictStatus(rs.resource, name)}
	}

	setResourceVersion(obj, newRevision)
	return obj, nil
}

// Delete removes a resource from storage and returns the deleted object.
func (rs *ResourceStore) Delete(ctx context.Context, namespace, name string) (runtime.Object, error) {
	key := rs.storageKey(namespace, name)

	// Get the current object so we can return it and obtain its revision
	stored, err := rs.storage.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, &StatusError{Status: notFoundStatus(rs.resource, name)}
		}
		return nil, fmt.Errorf("store delete: get current: %w", err)
	}

	// Decode the current object to return it
	obj := rs.newFunc()
	if err := DecodeFromStorage(stored.Value, obj); err != nil {
		return nil, fmt.Errorf("store delete: decode: %w", err)
	}
	setResourceVersion(obj, stored.ModRevision)

	// Delete from storage using the current revision
	_, err = rs.storage.Delete(ctx, key, stored.ModRevision)
	if err != nil {
		return nil, fmt.Errorf("store delete: %w", err)
	}

	return obj, nil
}
