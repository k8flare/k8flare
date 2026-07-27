package apiserver

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// ResourceStore handles CRUD for a single resource type (e.g. configmaps).
// It bridges between Kubernetes runtime.Object types and the raw byte Storage layer.
type ResourceStore struct {
	storage     *Storage
	resource    string // e.g. "configmaps"
	namespaced  bool
	newFunc     func() runtime.Object // creates a new empty object (e.g. &corev1.ConfigMap{})
	newListFunc func() runtime.Object // creates a new empty list object (e.g. &corev1.ConfigMapList{})
	// upstream serves the single-object verbs: the real
	// genericregistry.Store on top of KineStorage (see
	// upstreamregistry.go). Every resource in apidef.Table goes through
	// it since the S25 migration finished.
	upstream *genericregistry.Store
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
	gv schema.GroupVersion,
	resource, singular string,
	namespaced bool,
	newFunc, newListFunc func() runtime.Object,
) *ResourceStore {
	return &ResourceStore{
		storage:     storage,
		resource:    resource,
		namespaced:  namespaced,
		newFunc:     newFunc,
		newListFunc: newListFunc,
		upstream:    NewUpstreamStore(storage, gv, resource, singular, namespaced, newFunc, newListFunc),
	}
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

// specForGeneration returns obj's top-level Spec field and true when this
// resource is one k8flare maintains metadata.generation for. The gate is
// "declares a status subresource" (apidef), not "has a Spec field via
// reflection": it's the exact spec/status-split set k8s bumps generation
// for, it matches precisely where the status subresource handler preserves
// spec (subresource.go's copyStatus), and it leaves every resource without
// a status subresource (ConfigMap, Secret, Endpoints, ...) with zero
// behavior change. k8s controllers compare generation against
// status.observedGeneration for revision tracking -- real KCM's Deployment
// controller does, which is why the value must actually move.
func specForGeneration(resource string, obj runtime.Object) (reflect.Value, bool) {
	if !apidef.HasSubresource(resource, "status") {
		return reflect.Value{}, false
	}
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	f := v.FieldByName("Spec")
	if !f.IsValid() {
		return reflect.Value{}, false
	}
	return f, true
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

// deleteConflictRetries bounds ResourceStore.Delete's conflict-retry
// loop (see its doc comment). Conflicts are momentary races against
// another writer; a handful of immediate re-read-and-retry passes is
// plenty.
const deleteConflictRetries = 5

// StatusError wraps a metav1.Status as an error, allowing callers to inspect
// the structured Kubernetes status response.
type StatusError struct {
	Status *metav1.Status
}

func (e *StatusError) Error() string {
	return e.Status.Message
}

// Get returns one object via the upstream registry (genericregistry.Store).
func (rs *ResourceStore) Get(ctx context.Context, namespace, name string) (runtime.Object, error) {
	return rs.upstreamGet(ctx, namespace, name)
}

// List returns matching objects via the upstream registry; selector
// semantics (including the per-resource custom field selectors) live in
// upstreamregistry.go's attr funcs.
func (rs *ResourceStore) List(ctx context.Context, namespace string, fieldSelector string, labelSelector string) (runtime.Object, error) {
	return rs.upstreamList(ctx, namespace, fieldSelector, labelSelector)
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
	"spec.unschedulable": true,
}

// selectableFieldsFor extracts the fields.Set of field-selector-queryable
// values for obj. Real kube-apiserver requires this same per-resource-type
// mapping (each resource registers its own SelectableFields upstream, e.g.
// pkg/registry/core/pod/strategy.go's PodToSelectableFields) -- only the
// specific fields real clients embedded in this project actually query are
// implemented: kube-scheduler's Pod informer ("spec.nodeName",
// "status.phase!=Succeeded,status.phase!=Failed"), kube-proxy's Service
// informer ("spec.clusterIP!=None"), and the upstream e2e conformance
// framework's own SynchronizedBeforeSuite, which lists Nodes with
// "spec.unschedulable" in its global (pre-every-test) setup
// (k8s.io/kubernetes/test/e2e/framework/node, NodeToSelectableFields'
// fmt.Sprint(node.Spec.Unschedulable) convention matched exactly below).
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
	case *corev1.Node:
		set["spec.unschedulable"] = fmt.Sprint(o.Spec.Unschedulable)
	}
	return set
}

// Create persists a new object via the upstream registry (UID/
// creationTimestamp/generation stamping, generateName, AlreadyExists --
// all rest.BeforeCreate + genericStrategy).
func (rs *ResourceStore) Create(ctx context.Context, namespace string, obj runtime.Object) (runtime.Object, error) {
	return rs.upstreamCreate(ctx, namespace, obj)
}

// Update persists changes via the upstream registry (immutable-field
// preservation, UID preconditions, generation bump, finalizer-aware
// deletion completion -- rest.BeforeUpdate + genericStrategy).
func (rs *ResourceStore) Update(ctx context.Context, namespace, name string, obj runtime.Object) (runtime.Object, error) {
	return rs.upstreamUpdate(ctx, namespace, name, obj)
}

// Delete removes one object via the upstream registry and returns the
// deleted state.
func (rs *ResourceStore) Delete(ctx context.Context, namespace, name string) (runtime.Object, error) {
	return rs.upstreamDelete(ctx, namespace, name)
}

// DeleteCollection deletes every object of this resource type in namespace
// that matches labelSelector (all objects if labelSelector is empty), and
// returns a typed list of the objects that were deleted. An object named
// keepName is left alone and omitted from the result (empty = delete
// everything); the caller uses it to hold back an undeletable object --
// today only the management Cluster, see clusterprotect.go.
func (rs *ResourceStore) DeleteCollection(ctx context.Context, namespace, labelSelector, keepName string) (runtime.Object, error) {
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
		if keepName != "" && itemMeta.Name == keepName {
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
		// Revision 0 means unconditional delete (see Storage.Delete/
		// handleDelete), so a CAS mismatch can't happen -- but the
		// kine-log event insert can still lose a same-revision race
		// (ErrConflict, the DO's UNIQUE-constraint 409); retry those.
		var err error
		for attempt := 0; attempt < deleteConflictRetries; attempt++ {
			if _, err = rs.storage.Delete(ctx, key, 0); !errors.Is(err, ErrConflict) {
				break
			}
		}
		if err != nil {
			return 0, fmt.Errorf("delete %s: %w", obj.Key, err)
		}
	}
	return len(objs), nil
}
