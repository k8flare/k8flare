package kine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

func configMapResource(hash string) StoredResource {
	return StoredResource{
		Name:  "configmaps",
		Hash:  hash,
		Codec: scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion),
		New:   func() runtime.Object { return &corev1.ConfigMap{} },
	}
}

func seedLegacyConfigMap(m *memKine, name string) {
	m.rev++
	key := "/registry/configmaps/ns/" + name
	m.data[key] = []byte(fmt.Sprintf(`{"kind": "ConfigMap", "apiVersion": "v1", "legacy": true, "metadata": {"name": %q, "namespace": "ns"}, "data": {"k": "v"}}`, name))
	m.revs[key] = m.rev
}

func recordedVersions(t *testing.T, m *memKine) map[string]storageVersionRecord {
	t.Helper()
	records := map[string]storageVersionRecord{}
	if raw, ok := m.data[storageVersionsKey]; ok {
		if err := json.Unmarshal(raw, &records); err != nil {
			t.Fatal(err)
		}
	}
	return records
}

func TestMigrateStorageRewritesObjectsAtTheCurrentEncoding(t *testing.T) {
	mem, hc := newMemKine(t)
	client := &Client{HTTP: hc}
	seedLegacyConfigMap(mem, "a")
	resources := []StoredResource{configMapResource("h1")}
	ctx := context.Background()

	status, err := client.MigrateStorage(ctx, resources, MigrationPassBudget)
	if err != nil {
		t.Fatal(err)
	}
	if status.Rewritten != 1 || !status.Done || len(status.Pending) != 0 {
		t.Fatalf("first pass: %+v", status)
	}
	var stored corev1.ConfigMap
	if _, _, err := resources[0].Codec.Decode(mem.data["/registry/configmaps/ns/a"], nil, &stored); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(mem.data["/registry/configmaps/ns/a"])) != `{"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"a","namespace":"ns"},"data":{"k":"v"}}` {
		t.Fatalf("stored bytes were not re-encoded: %s", mem.data["/registry/configmaps/ns/a"])
	}
	s := NewStorage(client, resources[0].Codec, resources[0].New)
	var got corev1.ConfigMap
	if err := s.Get(ctx, "/configmaps/ns/a", storage.GetOptions{}, &got); err != nil || got.Data["k"] != "v" {
		t.Fatalf("object unreadable after migration: %v %+v", err, got)
	}
	if recordedVersions(t, mem)["configmaps"] != (storageVersionRecord{Hash: "h1"}) {
		t.Fatalf("recorded %+v", recordedVersions(t, mem))
	}

	again, err := client.MigrateStorage(ctx, resources, MigrationPassBudget)
	if err != nil {
		t.Fatal(err)
	}
	if again.Rewritten != 0 || !again.Done {
		t.Fatalf("second pass: %+v", again)
	}
	if mem.revs["/registry/configmaps/ns/a"] != 2 {
		t.Fatalf("second pass rewrote the object: rev %d", mem.revs["/registry/configmaps/ns/a"])
	}
}

func TestMigrateStorageResumesAcrossBudgetedPasses(t *testing.T) {
	mem, hc := newMemKine(t)
	client := &Client{HTTP: hc}
	for _, name := range []string{"a", "b", "c"} {
		seedLegacyConfigMap(mem, name)
	}
	resources := []StoredResource{configMapResource("h1")}
	ctx := context.Background()

	first, err := client.MigrateStorage(ctx, resources, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Rewritten != 1 || first.Done || len(first.Pending) != 1 {
		t.Fatalf("first pass: %+v", first)
	}
	if rec := recordedVersions(t, mem)["configmaps"]; rec.Hash != "h1" || rec.Cursor != "/registry/configmaps/ns/b" {
		t.Fatalf("cursor not recorded: %+v", rec)
	}
	status, err := client.StorageVersionStatus(ctx, resources)
	if err != nil || status.Done || len(status.Pending) != 1 {
		t.Fatalf("status: %+v %v", status, err)
	}

	second, err := client.MigrateStorage(ctx, resources, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Rewritten != 1 || second.Done {
		t.Fatalf("second pass: %+v", second)
	}
	third, err := client.MigrateStorage(ctx, resources, 1)
	if err != nil {
		t.Fatal(err)
	}
	if third.Rewritten != 1 || !third.Done {
		t.Fatalf("third pass: %+v", third)
	}
	for _, name := range []string{"a", "b", "c"} {
		if mem.revs["/registry/configmaps/ns/"+name] <= 3 {
			t.Fatalf("%s was not rewritten", name)
		}
	}
}

func TestMigrateStorageRescansWhenTheStorageVersionHashChanges(t *testing.T) {
	mem, hc := newMemKine(t)
	client := &Client{HTTP: hc}
	seedLegacyConfigMap(mem, "a")
	ctx := context.Background()
	if _, err := client.MigrateStorage(ctx, []StoredResource{configMapResource("h1")}, MigrationPassBudget); err != nil {
		t.Fatal(err)
	}
	seedLegacyConfigMap(mem, "b")
	unchanged, err := client.MigrateStorage(ctx, []StoredResource{configMapResource("h1")}, MigrationPassBudget)
	if err != nil || unchanged.Rewritten != 0 {
		t.Fatalf("same hash rescanned: %+v %v", unchanged, err)
	}
	changed, err := client.MigrateStorage(ctx, []StoredResource{configMapResource("h2")}, MigrationPassBudget)
	if err != nil || changed.Rewritten != 1 || !changed.Done {
		t.Fatalf("new hash: %+v %v", changed, err)
	}
	if recordedVersions(t, mem)["configmaps"].Hash != "h2" {
		t.Fatal(recordedVersions(t, mem))
	}
}

func TestMigrateStorageRecordsUndecodableResourcesAsFailed(t *testing.T) {
	mem, hc := newMemKine(t)
	client := &Client{HTTP: hc}
	mem.rev++
	mem.data["/registry/configmaps/ns/bad"] = []byte("not json")
	mem.revs["/registry/configmaps/ns/bad"] = mem.rev
	seedLegacyConfigMap(mem, "ok")
	ctx := context.Background()
	resources := []StoredResource{configMapResource("h1")}

	status, err := client.MigrateStorage(ctx, resources, MigrationPassBudget)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Done || status.Failed["configmaps"] == "" || status.Rewritten != 0 {
		t.Fatalf("%+v", status)
	}
	again, err := client.MigrateStorage(ctx, resources, MigrationPassBudget)
	if err != nil || again.Rewritten != 0 || again.Failed["configmaps"] == "" {
		t.Fatalf("failed resource retried: %+v %v", again, err)
	}
}
