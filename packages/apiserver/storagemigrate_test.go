package apiserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/authentication/user"
)

const legacyHPAKey = "/registry/horizontalpodautoscalers/ns/web"

func storageMigrateMux(t *testing.T) (*http.ServeMux, *kine.Client) {
	t.Helper()
	client := fakeVaultStore()
	if _, err := client.Put(context.Background(), legacyHPAKey, []byte(`{"kind":"HorizontalPodAutoscaler","apiVersion":"autoscaling/v1","metadata":{"name":"web","namespace":"ns"},"spec":{"scaleTargetRef":{"kind":"Deployment","name":"web","apiVersion":"apps/v1"},"maxReplicas":3,"targetCPUUtilizationPercentage":50}}`), 0); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	installStorageMigrate(mux, client)
	return mux, client
}

func callStorageMigrate(t *testing.T, mux http.Handler, method, path string) kine.MigrationStatus {
	t.Helper()
	code, body := callAdmin(mux, method, path, "", user.SystemPrivilegedGroup)
	if code != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, path, code, body)
	}
	var status kine.MigrationStatus
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatalf("%s %s: %v %s", method, path, err, body)
	}
	return status
}

func storedAPIVersion(t *testing.T, client *kine.Client, key string) string {
	t.Helper()
	kv, _, err := client.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	return stored.APIVersion
}

func TestStorageMigrateRoutesAreAdminOnly(t *testing.T) {
	mux, client := storageMigrateMux(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/internal/storage-migrate/status"},
		{http.MethodPost, "/internal/storage-migrate"},
	} {
		if code, _ := callAdmin(mux, c.method, c.path, "", user.AllAuthenticated); code != http.StatusForbidden {
			t.Fatalf("%s %s as non-admin: %d", c.method, c.path, code)
		}
	}
	if got := storedAPIVersion(t, client, legacyHPAKey); got != "autoscaling/v1" {
		t.Fatalf("non-admin request rewrote the object to %q", got)
	}
}

func TestStorageMigrateRewritesStoredObjectsInPlace(t *testing.T) {
	mux, client := storageMigrateMux(t)

	before := callStorageMigrate(t, mux, http.MethodGet, "/internal/storage-migrate/status")
	if before.Done || !slices.Contains(before.Pending, "horizontalpodautoscalers") {
		t.Fatalf("status before the sweep: %+v", before)
	}

	swept := callStorageMigrate(t, mux, http.MethodPost, "/internal/storage-migrate")
	if !swept.Done || swept.Rewritten != 1 || len(swept.Pending) != 0 || len(swept.Failed) != 0 {
		t.Fatalf("sweep: %+v", swept)
	}
	if got := storedAPIVersion(t, client, legacyHPAKey); got != "autoscaling/v2" {
		t.Fatalf("stored at %q after the sweep", got)
	}

	after := callStorageMigrate(t, mux, http.MethodGet, "/internal/storage-migrate/status")
	if !after.Done || len(after.Pending) != 0 {
		t.Fatalf("status after the sweep: %+v", after)
	}
	again := callStorageMigrate(t, mux, http.MethodPost, "/internal/storage-migrate")
	if !again.Done || again.Rewritten != 0 {
		t.Fatalf("second sweep: %+v", again)
	}
}
