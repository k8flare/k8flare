package apiserver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/storage/names"
)

// ResourceStore handles CRUD for a single resource type (e.g. configmaps).
// It bridges between Kubernetes runtime.Object types and the raw byte Storage layer.
type ResourceStore struct {
	storage     *Storage
	resource    string // e.g. "configmaps"
	namespaced  bool
	newFunc     func() runtime.Object // creates a new empty object (e.g. &corev1.ConfigMap{})
	newListFunc func() runtime.Object // creates a new empty list object (e.g. &corev1.ConfigMapList{})
}

// NewResourceStore creates a ResourceStore for the given resource type.
// Item insertion into a listed object is handled generically by
// k8s.io/apimachinery/pkg/api/meta.SetList (see List below) via reflection
// on the list's "Items" field -- every k8s API list type has one -- so,
// unlike the ResourceStore this project had before apidef.Table existed,
// there is no per-type "append into the right typed slice" callback to
// supply here.
func NewResourceStore(
	storage *Storage,
	resource string,
	namespaced bool,
	newFunc, newListFunc func() runtime.Object,
) *ResourceStore {
	return &ResourceStore{
		storage:     storage,
		resource:    resource,
		namespaced:  namespaced,
		newFunc:     newFunc,
		newListFunc: newListFunc,
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

// prepareJobForCreate auto-generates spec.selector and injects the
// controller-uid/job-name labels into spec.template.metadata.labels when
// spec.manualSelector isn't true -- matching upstream's
// pkg/registry/batch/job/strategy.go generateSelectorIfNeeded/
// generateSelector, which (unlike the type-defaulting registered on Scheme
// in scheme.go) are unexported and specifically need the object's UID, so
// they can't be reused directly and must run after Create has assigned one.
// Without this, the real Job controller's own AdoptOrphan/ReleaseOrphan pod
// ownership check (comparing each Pod's labels against spec.selector) always
// fails against a nil selector, so it repeatedly disowns and replaces the
// Pods it just created.
func prepareJobForCreate(job *batchv1.Job) {
	if job.Spec.ManualSelector != nil && *job.Spec.ManualSelector {
		return
	}
	if job.Spec.Template.Labels == nil {
		job.Spec.Template.Labels = map[string]string{}
	}
	for _, key := range []string{"job-name", batchv1.JobNameLabel} {
		if _, ok := job.Spec.Template.Labels[key]; !ok {
			job.Spec.Template.Labels[key] = job.Name
		}
	}
	for _, key := range []string{"controller-uid", batchv1.ControllerUidLabel} {
		if _, ok := job.Spec.Template.Labels[key]; !ok {
			job.Spec.Template.Labels[key] = string(job.UID)
		}
	}
	if job.Spec.Selector == nil {
		job.Spec.Selector = &metav1.LabelSelector{}
	}
	if job.Spec.Selector.MatchLabels == nil {
		job.Spec.Selector.MatchLabels = map[string]string{}
	}
	if _, ok := job.Spec.Selector.MatchLabels[batchv1.ControllerUidLabel]; !ok {
		job.Spec.Selector.MatchLabels[batchv1.ControllerUidLabel] = string(job.UID)
	}
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

// immutableFieldStatus returns a metav1.Status indicating a client tried to
// change a server-assigned, immutable metadata field on update.
func immutableFieldStatus(resource, name, field string) *metav1.Status {
	return &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Message:  fmt.Sprintf("%s %q is invalid: metadata.%s: field is immutable", resource, name, field),
		Reason:   metav1.StatusReasonInvalid,
		Code:     422,
	}
}

// badRequestStatus returns a metav1.Status indicating the request itself
// was malformed (e.g. a fieldSelector referencing an unsupported field).
func badRequestStatus(message string) *metav1.Status {
	return &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Message:  message,
		Reason:   metav1.StatusReasonBadRequest,
		Code:     400,
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
// or cluster-scoped resources). Results are filtered by fieldSelector and labelSelector, if
// either is non-empty.
func (rs *ResourceStore) List(ctx context.Context, namespace string, fieldSelector string, labelSelector string) (runtime.Object, error) {
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

	items, err = applyFieldSelector(items, fieldSelector)
	if err != nil {
		return nil, fmt.Errorf("store list: %w", err)
	}
	items, err = applyLabelSelector(items, labelSelector)
	if err != nil {
		return nil, fmt.Errorf("store list: %w", err)
	}

	listObj := rs.newListFunc()
	if err := meta.SetList(listObj, items); err != nil {
		return nil, fmt.Errorf("store list: set items: %w", err)
	}

	// Set list-level resourceVersion for watch continuation
	if accessor, ok := listObj.(metav1.ListMetaAccessor); ok {
		accessor.GetListMeta().SetResourceVersion(strconv.FormatInt(rev, 10))
	}

	return listObj, nil
}

// applyLabelSelector filters a list of runtime.Object by the given Kubernetes label
// selector string (e.g. "env=prod,tier in (web,api)"). An empty selector matches everything.
func applyLabelSelector(items []runtime.Object, labelSelector string) ([]runtime.Object, error) {
	if labelSelector == "" {
		return items, nil
	}
	selector, err := labels.Parse(labelSelector)
	if err != nil {
		return nil, fmt.Errorf("parse label selector %q: %w", labelSelector, err)
	}
	filtered := make([]runtime.Object, 0, len(items))
	for _, item := range items {
		meta := getObjectMeta(item)
		if meta != nil && selector.Matches(labels.Set(meta.Labels)) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// knownSelectableFields is every field path selectableFieldsFor (below)
// ever populates. applyFieldSelector rejects a selector term naming a field
// outside this set (400 Bad Request, matching real kube-apiserver's
// behavior for an unsupported field selector) instead of silently treating
// it as always-matching. That silent-pass-through used to be this
// function's actual behavior (an unrecognized field just fell through a
// switch statement's default case) and caused a real bug: kube-proxy's
// Service informer filters with "spec.clusterIP!=None" to skip headless
// Services, and before spec.clusterIP was added here, that term silently
// matched everything, leaking headless Services into kube-proxy's view
// (see git history, commit 0180b37).
var knownSelectableFields = map[string]bool{
	"metadata.name":      true,
	"metadata.namespace": true,
	"spec.nodeName":      true,
	"status.phase":       true,
	"spec.clusterIP":     true,
}

// selectableFieldsFor extracts the fields.Set of field-selector-queryable
// values for obj. Real kube-apiserver requires this same per-resource-type
// mapping (each resource registers its own SelectableFields upstream, e.g.
// pkg/registry/core/pod/strategy.go's PodToSelectableFields) -- only the
// specific fields real clients embedded in this project actually query are
// implemented: kube-scheduler's Pod informer ("spec.nodeName",
// "status.phase!=Succeeded,status.phase!=Failed") and kube-proxy's Service
// informer ("spec.clusterIP!=None").
func selectableFieldsFor(obj runtime.Object) fields.Set {
	set := fields.Set{}
	if meta := getObjectMeta(obj); meta != nil {
		set["metadata.name"] = meta.Name
		set["metadata.namespace"] = meta.Namespace
	}
	switch o := obj.(type) {
	case *corev1.Pod:
		set["spec.nodeName"] = o.Spec.NodeName
		set["status.phase"] = string(o.Status.Phase)
	case *corev1.Service:
		set["spec.clusterIP"] = o.Spec.ClusterIP
	}
	return set
}

// applyFieldSelector filters items by fieldSelector (real Kubernetes field
// selector syntax, e.g. "status.phase!=Succeeded,status.phase!=Failed" or
// "spec.clusterIP!=None"), using k8s.io/apimachinery/pkg/fields' real
// parser and matcher rather than a hand-rolled one.
func applyFieldSelector(items []runtime.Object, fieldSelector string) ([]runtime.Object, error) {
	if fieldSelector == "" {
		return items, nil
	}
	selector, err := fields.ParseSelector(fieldSelector)
	if err != nil {
		return nil, fmt.Errorf("parse field selector %q: %w", fieldSelector, err)
	}
	for _, req := range selector.Requirements() {
		if !knownSelectableFields[req.Field] {
			return nil, &StatusError{Status: badRequestStatus(fmt.Sprintf("field label not supported: %s", req.Field))}
		}
	}

	filtered := make([]runtime.Object, 0, len(items))
	for _, item := range items {
		if selector.Matches(selectableFieldsFor(item)) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
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
		if meta.GenerateName == "" {
			return nil, fmt.Errorf("store create: name is required")
		}
		// Real controllers (ReplicaSet, Deployment, DaemonSet, Job, ...)
		// create Pods this way rather than picking an explicit name
		// themselves. names.SimpleNameGenerator is the exact upstream
		// apiserver behavior: base + 5 random alphanumerics.
		name = names.SimpleNameGenerator.GenerateName(meta.GenerateName)
		meta.Name = name
	}

	// Set metadata for creation
	meta.UID = uuid.NewUUID()
	meta.CreationTimestamp = metav1.Now()
	if rs.namespaced {
		meta.Namespace = namespace
	}
	// Clear resource version before encoding for storage
	meta.ResourceVersion = ""

	if job, ok := obj.(*batchv1.Job); ok {
		prepareJobForCreate(job)
	}

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

	stored, err := rs.storage.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, &StatusError{Status: notFoundStatus(rs.resource, name)}
		}
		return nil, fmt.Errorf("store update: get current: %w", err)
	}

	oldObj := rs.newFunc()
	if err := DecodeFromStorage(stored.Value, oldObj); err != nil {
		return nil, fmt.Errorf("store update: decode current: %w", err)
	}
	oldMeta := getObjectMeta(oldObj)

	// UID and CreationTimestamp are server-assigned on Create (see above) and
	// immutable afterward. A client that omits one gets the existing value
	// filled in; a client that sends a different value is rejected. Without
	// this, a client PUT that drops these fields would silently overwrite
	// them, breaking anything that compares against the original UID (e.g.
	// ownerReferences).
	if meta.UID == "" {
		meta.UID = oldMeta.UID
	} else if meta.UID != oldMeta.UID {
		return nil, &StatusError{Status: immutableFieldStatus(rs.resource, name, "uid")}
	}
	if meta.CreationTimestamp.IsZero() {
		meta.CreationTimestamp = oldMeta.CreationTimestamp
	} else if !meta.CreationTimestamp.Equal(&oldMeta.CreationTimestamp) {
		return nil, &StatusError{Status: immutableFieldStatus(rs.resource, name, "creationTimestamp")}
	}

	var currentRevision int64
	if meta.ResourceVersion == "" {
		currentRevision = stored.ModRevision
	} else {
		// Parse the resource version from the incoming object for CAS
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

// DeleteCollection deletes every object of this resource type in namespace
// that matches labelSelector (all objects if labelSelector is empty), and
// returns a typed list of the objects that were deleted.
func (rs *ResourceStore) DeleteCollection(ctx context.Context, namespace, labelSelector string) (runtime.Object, error) {
	listObj, err := rs.List(ctx, namespace, "", labelSelector)
	if err != nil {
		return nil, fmt.Errorf("store delete collection: list: %w", err)
	}

	items, err := meta.ExtractList(listObj)
	if err != nil {
		return nil, fmt.Errorf("store delete collection: extract list: %w", err)
	}

	deleted := make([]runtime.Object, 0, len(items))
	for _, item := range items {
		itemMeta := getObjectMeta(item)
		if itemMeta == nil {
			continue
		}
		obj, err := rs.Delete(ctx, namespace, itemMeta.Name)
		if err != nil {
			return nil, fmt.Errorf("store delete collection: delete %s: %w", itemMeta.Name, err)
		}
		deleted = append(deleted, obj)
	}

	resultList := rs.newListFunc()
	if err := meta.SetList(resultList, deleted); err != nil {
		return nil, fmt.Errorf("store delete collection: set items: %w", err)
	}
	return resultList, nil
}

// DeleteAllInNamespace removes every object of this resource type within the
// given namespace. Used by namespace cascading deletion (see
// namespacedelete.go) to sweep dependents before the Namespace object itself
// is removed.
//
// Deletes are unconditional (revision 0), not CAS against the revision seen
// during listing: the intent is "this object's namespace is going away,
// remove whatever is there now," not "abort if it changed since I looked."
func (rs *ResourceStore) DeleteAllInNamespace(ctx context.Context, namespace string) (int, error) {
	if !rs.namespaced {
		return 0, fmt.Errorf("resource %q is not namespaced", rs.resource)
	}
	prefix := rs.storagePrefix(namespace)
	objs, _, err := rs.storage.List(ctx, prefix, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("list %s for cascade delete: %w", rs.resource, err)
	}
	for _, obj := range objs {
		// obj.Key is the full stored key (e.g. "/registry/pods/ns/name"), but
		// Storage.Delete expects a key relative to its own prefix (it
		// re-prepends the prefix itself) — must strip it here or the request
		// double-prefixes and fails to find the key.
		key := strings.TrimPrefix(obj.Key, rs.storage.prefix)
		// Revision 0 means unconditional delete (see Storage.Delete/handleDelete).
		if _, err := rs.storage.Delete(ctx, key, 0); err != nil {
			return 0, fmt.Errorf("delete %s: %w", obj.Key, err)
		}
	}
	return len(objs), nil
}
