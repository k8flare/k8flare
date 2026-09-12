package apiserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// storedTypeMeta reads back the raw bytes under key -- what the TypeScript
// watch layer streams verbatim, which is why these tests assert on the
// stored bytes and not on a GET (Encode stamps GVK on the way out, so a GET
// passed even while watch was broken).
func storedTypeMeta(t *testing.T, kv *fakeKV, key string) (apiVersion, kind string) {
	t.Helper()
	entry, ok := kv.data[key]
	if !ok {
		t.Fatalf("no object stored at %s", key)
	}
	raw, err := base64.StdEncoding.DecodeString(entry.value)
	if err != nil {
		t.Fatalf("decode stored value at %s: %v", key, err)
	}
	var tm metav1.TypeMeta
	if err := json.Unmarshal(raw, &tm); err != nil {
		t.Fatalf("unmarshal stored value at %s: %v", key, err)
	}
	return tm.APIVersion, tm.Kind
}

func requireStoredTypeMeta(t *testing.T, kv *fakeKV, key, wantAPIVersion, wantKind string) {
	t.Helper()
	gotAPIVersion, gotKind := storedTypeMeta(t, kv, key)
	if gotAPIVersion != wantAPIVersion || gotKind != wantKind {
		t.Errorf("stored %s has apiVersion=%q kind=%q, want %q/%q",
			key, gotAPIVersion, gotKind, wantAPIVersion, wantKind)
	}
}

// TestServerConstructedObjectsAreStoredWithTypeMeta covers every object this
// apiserver builds in Go and persists. Without the stamp each one is stored
// with an empty TypeMeta, and a watch event carrying it makes client-go's
// reflector log "Object 'Kind' is missing" and relist.
func TestServerConstructedObjectsAreStoredWithTypeMeta(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	stores := NewResourceStoresForGroupVersion(storage, corev1.SchemeGroupVersion)
	ctx := context.Background()

	// bootstrap.go's namespaces. BootstrapCluster itself is behind a
	// package-level sync.Once, so the object it creates is built here
	// instead of calling it.
	if _, err := stores["namespaces"].Create(ctx, "", &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}, nil); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	requireStoredTypeMeta(t, kv, "/namespaces/default", "v1", "Namespace")

	// bootstrap.go's "kubernetes" Service.
	if _, err := stores["services"].Create(ctx, "default", &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			ClusterIP:  "10.43.0.1",
			ClusterIPs: []string{"10.43.0.1"},
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(443), Protocol: corev1.ProtocolTCP},
			},
		},
	}, nil); err != nil {
		t.Fatalf("create service: %v", err)
	}
	requireStoredTypeMeta(t, kv, "/services/default/kubernetes", "v1", "Service")

	// serviceaccount.go's two per-namespace objects.
	ensureDefaultServiceAccount(ctx, stores["serviceaccounts"], "default")
	requireStoredTypeMeta(t, kv, "/serviceaccounts/default/default", "v1", "ServiceAccount")

	ensureRootCAConfigMap(ctx, stores["configmaps"], "default")
	requireStoredTypeMeta(t, kv, "/configmaps/default/kube-root-ca.crt", "v1", "ConfigMap")
}

// TestTypeMetaLessRequestBodyIsStoredWithTypeMeta is the same defect reached
// from the API rather than from Go: a body with no apiVersion/kind is
// accepted (the route supplies the decode default) but used to be persisted
// exactly as sent. kube-controller-manager's event broadcaster POSTs bodies
// of that shape -- see ResourceStore.decodeDefaults.
func TestTypeMetaLessRequestBodyIsStoredWithTypeMeta(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	cmStore := NewResourceStoresForGroupVersion(storage, corev1.SchemeGroupVersion)["configmaps"]
	ctx := context.Background()

	obj, err := decodeBody([]byte(`{"metadata":{"name":"bare","namespace":"default"},"data":{"a":"b"}}`), cmStore.decodeDefaults())
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, err := cmStore.Create(ctx, "default", obj, nil); err != nil {
		t.Fatalf("create configmap: %v", err)
	}
	requireStoredTypeMeta(t, kv, "/configmaps/default/bare", "v1", "ConfigMap")

	updated := obj.DeepCopyObject()
	updated.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{})
	if _, err := cmStore.Update(ctx, "default", "bare", updated, nil); err != nil {
		t.Fatalf("update configmap: %v", err)
	}
	requireStoredTypeMeta(t, kv, "/configmaps/default/bare", "v1", "ConfigMap")
}

// stripStoredTypeMeta rewrites the object under key the way this apiserver
// persisted it before the stamp, standing in for a cluster whose Durable
// Object already holds Kind-less objects.
func stripStoredTypeMeta(t *testing.T, kv *fakeKV, key string) {
	t.Helper()
	entry, ok := kv.data[key]
	if !ok {
		t.Fatalf("no object stored at %s", key)
	}
	raw, err := base64.StdEncoding.DecodeString(entry.value)
	if err != nil {
		t.Fatalf("decode stored value at %s: %v", key, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal stored value at %s: %v", key, err)
	}
	delete(m, "apiVersion")
	delete(m, "kind")
	legacy, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal stored value at %s: %v", key, err)
	}
	entry.value = base64.StdEncoding.EncodeToString(legacy)
}

// TestStampedTypeMetaDoesNotDefeatNoOpSuppression guards the cost invariant
// that KineStorage.GuaranteedUpdate's byte comparison protects: a rewrite of
// an unchanged object must not produce a new revision. Stamping changes the
// bytes once (a legacy Kind-less object normalizes on its first update) and
// must converge immediately after.
func TestStampedTypeMetaDoesNotDefeatNoOpSuppression(t *testing.T) {
	kv := newFakeKV()
	storage := newTestStorage(kv)
	cmStore := NewResourceStoresForGroupVersion(storage, corev1.SchemeGroupVersion)["configmaps"]
	ctx := context.Background()

	if _, err := cmStore.Create(ctx, "default", &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "quiet", Namespace: "default"},
		Data:       map[string]string{"a": "b"},
	}, nil); err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	stored, err := cmStore.Get(ctx, "default", "quiet")
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	revAfterCreate := kv.revision

	for i := 0; i < 2; i++ {
		if _, err := cmStore.Update(ctx, "default", "quiet", stored.DeepCopyObject(), nil); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}
	if kv.revision != revAfterCreate {
		t.Errorf("identical updates bumped the revision from %d to %d", revAfterCreate, kv.revision)
	}

	stripStoredTypeMeta(t, kv, "/configmaps/default/quiet")
	revBeforeNormalize := kv.revision
	if _, err := cmStore.Update(ctx, "default", "quiet", stored.DeepCopyObject(), nil); err != nil {
		t.Fatalf("normalizing update: %v", err)
	}
	if kv.revision != revBeforeNormalize+1 {
		t.Errorf("normalizing a legacy object moved the revision from %d to %d, want one write",
			revBeforeNormalize, kv.revision)
	}
	requireStoredTypeMeta(t, kv, "/configmaps/default/quiet", "v1", "ConfigMap")

	normalized, err := cmStore.Get(ctx, "default", "quiet")
	if err != nil {
		t.Fatalf("get normalized configmap: %v", err)
	}
	revAfterNormalize := kv.revision
	if _, err := cmStore.Update(ctx, "default", "quiet", normalized.DeepCopyObject(), nil); err != nil {
		t.Fatalf("update after normalizing: %v", err)
	}
	if kv.revision != revAfterNormalize {
		t.Errorf("normalization did not converge: revision moved from %d to %d",
			revAfterNormalize, kv.revision)
	}
}

// TestGetResponseUnchangedByStoredTypeMeta pins the claim that only the
// stored bytes move: Encode already stamped GVK on the way out, so a GET
// reads the same either way.
func TestGetResponseUnchangedByStoredTypeMeta(t *testing.T) {
	ctx := context.Background()

	encodeNamespace := func(t *testing.T, stamped bool) string {
		t.Helper()
		kv := newFakeKV()
		nsStore := NewResourceStoresForGroupVersion(newTestStorage(kv), corev1.SchemeGroupVersion)["namespaces"]
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		}
		if _, err := nsStore.Create(ctx, "", ns, nil); err != nil {
			t.Fatalf("create namespace: %v", err)
		}
		if !stamped {
			stripStoredTypeMeta(t, kv, "/namespaces/default")
		}
		got, err := nsStore.Get(ctx, "", "default")
		if err != nil {
			t.Fatalf("get namespace: %v", err)
		}
		getObjectMeta(got).SetUID("")
		getObjectMeta(got).SetCreationTimestamp(metav1.Time{})
		encoded, err := Encode(got)
		if err != nil {
			t.Fatalf("encode namespace: %v", err)
		}
		return string(encoded)
	}

	if stamped, legacy := encodeNamespace(t, true), encodeNamespace(t, false); stamped != legacy {
		t.Errorf("GET response changed\n stamped: %s\nwithout: %s", stamped, legacy)
	}
}

// TestEveryResourceStampsTypeMeta is the part that makes the fix
// unforgettable: a resource added to apidef.Table inherits the stamp from
// the shared strategy, so no future server-side construction site can
// reintroduce a Kind-less stored object by omitting a call.
func TestEveryResourceStampsTypeMeta(t *testing.T) {
	ctx := context.Background()
	for _, gv := range apidef.GroupVersions() {
		kv := newFakeKV()
		stores := NewResourceStoresForGroupVersion(newTestStorage(kv), gv)
		for _, def := range apidef.ForGroupVersion(gv) {
			store, ok := stores[def.Resource]
			if !ok {
				t.Errorf("%s: no store for resource %q", gv, def.Resource)
				continue
			}
			namespace := ""
			key := "/" + def.Resource + "/probe"
			if def.Namespaced {
				namespace = "probe-ns"
				key = "/" + def.Resource + "/" + namespace + "/probe"
			}
			obj := def.New()
			getObjectMeta(obj).SetName("probe")
			getObjectMeta(obj).SetNamespace(namespace)
			if _, err := store.Create(ctx, namespace, obj, nil); err != nil {
				t.Errorf("%s %s: create: %v", gv, def.Resource, err)
				continue
			}
			requireStoredTypeMeta(t, kv, key, gv.String(), def.Kind)
		}
	}
}
