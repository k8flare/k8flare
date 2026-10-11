package gc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

type storeAPI struct {
	*blockingResourceClient
	store     *fakeKineStore
	resource  string
	namespace string
	refuse    string
}

func (a *storeAPI) Resource(r schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	c := *a
	c.resource = r.Resource
	return &c
}

func (a *storeAPI) Namespace(ns string) dynamic.ResourceInterface {
	c := *a
	c.namespace = ns
	return &c
}

func (a *storeAPI) key(name string) string {
	return "/registry/" + a.resource + "/" + a.namespace + "/" + name
}

func (a *storeAPI) Delete(_ context.Context, name string, opts metav1.DeleteOptions, _ ...string) error {
	if name == a.refuse {
		return errors.New("delete refused")
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	obj, ok := a.read(name)
	if !ok {
		return nil
	}
	meta := obj["metadata"].(map[string]any)
	switch *opts.PropagationPolicy {
	case metav1.DeletePropagationForeground:
		meta["finalizers"] = append(finalizersOf(meta), foregroundFinalizer)
	case metav1.DeletePropagationOrphan:
		meta["finalizers"] = append(finalizersOf(meta), orphanFinalizer)
	}
	if len(finalizersOf(meta)) == 0 {
		delete(a.store.data, a.key(name))
		return nil
	}
	meta["deletionTimestamp"] = metav1.Now().UTC().Format("2006-01-02T15:04:05Z")
	a.write(name, obj)
	return nil
}

func (a *storeAPI) Patch(_ context.Context, name string, _ types.PatchType, data []byte, _ metav1.PatchOptions, _ ...string) (*unstructured.Unstructured, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	obj, ok := a.read(name)
	if !ok {
		return nil, nil
	}
	var patch struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(data, &patch); err != nil {
		return nil, err
	}
	meta := obj["metadata"].(map[string]any)
	for _, field := range []string{"ownerReferences", "finalizers"} {
		if v, ok := patch.Metadata[field]; ok {
			meta[field] = v
		}
	}
	if meta["deletionTimestamp"] != nil && len(finalizersOf(meta)) == 0 {
		delete(a.store.data, a.key(name))
		return nil, nil
	}
	a.write(name, obj)
	return nil, nil
}

func (a *storeAPI) read(name string) (map[string]any, bool) {
	raw, ok := a.store.data[a.key(name)]
	if !ok {
		return nil, false
	}
	decoded, _ := base64.StdEncoding.DecodeString(raw)
	var obj map[string]any
	_ = json.Unmarshal(decoded, &obj)
	return obj, true
}

func (a *storeAPI) write(name string, obj map[string]any) {
	raw, _ := json.Marshal(obj)
	a.store.data[a.key(name)] = base64.StdEncoding.EncodeToString(raw)
}

func finalizersOf(meta map[string]any) []any {
	list, _ := meta["finalizers"].([]any)
	return list
}

func storedItem(data map[string]string, it *item) {
	raw, _ := json.Marshal(it)
	data[it.key] = base64.StdEncoding.EncodeToString(raw)
}

func foregroundRC(data map[string]string, pods int) {
	storedItem(data, &item{TypeMeta: metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"}, ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "ns-default"}, key: "/registry/namespaces/default"})
	now := metav1.Now()
	rc := owned("rc")
	rc.Kind = "ReplicationController"
	rc.key = "/registry/replicationcontrollers/default/rc"
	rc.DeletionTimestamp = &now
	rc.Finalizers = []string{foregroundFinalizer}
	storedItem(data, rc)
	for i := range pods {
		storedItem(data, owned(fmt.Sprintf("pod-%d", i), ref("rc", true)))
	}
}

func collectAgainst(store *fakeKineStore, refuse string) (*Result, error) {
	kubeClient := fakekube.NewSimpleClientset()
	kubeClient.Discovery().(*fake.FakeDiscovery).Resources = []*metav1.APIResourceList{{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod"},
			{Name: "replicationcontrollers", SingularName: "replicationcontroller", Namespaced: true, Kind: "ReplicationController"},
			{Name: "namespaces", SingularName: "namespace", Kind: "Namespace"},
		},
	}}
	api := &storeAPI{blockingResourceClient: &blockingResourceClient{}, store: store, refuse: refuse}
	return Collect(context.Background(), kubeClient, api, &kine.Client{HTTP: &http.Client{Transport: store}})
}

func TestAForegroundDeletionFinishesOnTheWritesItsCollectsMakeAndThenNeedsNoRetry(t *testing.T) {
	store := &fakeKineStore{data: map[string]string{}}
	foregroundRC(store.data, 3)
	for round := 0; ; round++ {
		if round == 8 {
			t.Fatal("collects kept writing after 8 rounds")
		}
		res, err := collectAgainst(store, "")
		if err != nil {
			t.Fatal(err)
		}
		if res.Deleted+res.Patched > 0 {
			continue
		}
		if res.Failed != 0 || res.Remaining != 0 {
			t.Fatalf("result = %+v once nothing was left to write, want no failure and nothing remaining", res)
		}
		break
	}
	var keys []string
	for k := range store.data {
		keys = append(keys, k)
	}
	if !slices.Equal(keys, []string{"/registry/namespaces/default"}) {
		t.Fatalf("left behind %s, want the rc and its pods gone", strings.Join(keys, " "))
	}
}

func TestAForegroundOwnerWaitsForDependentsAlreadyBeingDeletedWithoutWritingToThem(t *testing.T) {
	store := &fakeKineStore{data: map[string]string{}}
	foregroundRC(store.data, 0)
	now := metav1.Now()
	var pods []string
	for i := range 3 {
		pod := owned(fmt.Sprintf("pod-%d", i), ref("rc", true))
		pod.DeletionTimestamp = &now
		storedItem(store.data, pod)
		pods = append(pods, pod.key)
	}
	res, err := collectAgainst(store, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted+res.Patched != 0 {
		t.Fatalf("result = %+v while the rc's pods terminate, want no write: the rc waits for them to be gone", res)
	}
	if _, ok := store.data["/registry/replicationcontrollers/default/rc"]; !ok {
		t.Fatal("rc removed while its pods still exist")
	}
	for _, key := range pods {
		delete(store.data, key)
	}
	if _, err := collectAgainst(store, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.data["/registry/replicationcontrollers/default/rc"]; ok {
		t.Fatal("rc still present after its pods are gone")
	}
}

func TestAForegroundDeletionCycleFinishes(t *testing.T) {
	store := &fakeKineStore{data: map[string]string{}}
	foregroundRC(store.data, 0)
	delete(store.data, "/registry/replicationcontrollers/default/rc")
	now := metav1.Now()
	for _, p := range []*item{owned("p1", ref("p3", true)), owned("p2", ref("p1", true)), owned("p3", ref("p2", true))} {
		p.DeletionTimestamp = &now
		p.Finalizers = []string{foregroundFinalizer}
		storedItem(store.data, p)
	}
	for round := 0; len(store.data) > 1; round++ {
		if round == 8 {
			t.Fatalf("cycle still present after 8 collects: %d keys", len(store.data))
		}
		if _, err := collectAgainst(store, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestACollectCountsADeleteItCouldNotMakeAsFailed(t *testing.T) {
	store := &fakeKineStore{data: map[string]string{}}
	foregroundRC(store.data, 2)
	res, err := collectAgainst(store, "pod-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || res.Deleted != 1 {
		t.Fatalf("result = %+v, want the refused delete counted as failed", res)
	}
}
