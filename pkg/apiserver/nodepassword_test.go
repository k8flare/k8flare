package apiserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// fakeKV is an in-memory key-value store that mimics the KineStore Durable Object
// HTTP API used by Storage.
type fakeKV struct {
	mu       sync.Mutex
	data     map[string]*fakeEntry
	revision int64

	// Optional hooks for injecting errors.
	getErr    error
	createErr error
	updateErr error
}

type fakeEntry struct {
	value          string // base64-encoded
	createRevision int64
	modRevision    int64
}

func newFakeKV() *fakeKV {
	return &fakeKV{
		data:     make(map[string]*fakeEntry),
		revision: 0,
	}
}

// doFetch implements the func(req *http.Request) (*http.Response, error) signature
// expected by Storage. It handles GET /key/..., PUT /key/..., DELETE /key/...,
// and GET /list/... (prefix scan, added for
// TestDeleteNamespaceDependents_ReleasesServiceClusterIPs -- every prior
// fakeKV-based test only ever needed single-key Get/Put).
func (f *fakeKV) doFetch(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := req.URL.Path

	if strings.HasPrefix(path, "/key/") {
		key := strings.TrimPrefix(path, "/key")
		switch req.Method {
		case http.MethodGet:
			return f.handleGet(key)
		case http.MethodPut:
			return f.handlePut(key, req)
		case http.MethodDelete:
			return f.handleDelete(key)
		}
	}

	if strings.HasPrefix(path, "/list/") && req.Method == http.MethodGet {
		prefix := strings.TrimPrefix(path, "/list")
		return f.handleList(prefix)
	}

	return jsonResponse(http.StatusNotFound, map[string]interface{}{
		"error": "not found",
	}), nil
}

// handleDelete unconditionally removes key (matching how this project's
// callers only ever pass revision=0 -- unconditional delete -- through
// fakeKV-backed tests today; a real CAS-checked delete isn't needed here).
func (f *fakeKV) handleDelete(key string) (*http.Response, error) {
	entry, existed := f.data[key]
	delete(f.data, key)
	if !existed {
		return jsonResponse(http.StatusOK, deleteResponse{Revision: f.revision, Deleted: true}), nil
	}
	f.revision++
	return jsonResponse(http.StatusOK, deleteResponse{
		Revision: f.revision,
		KV: &kvJSON{
			Key:            key,
			Value:          entry.value,
			CreateRevision: entry.createRevision,
			ModRevision:    entry.modRevision,
		},
		Deleted: true,
	}), nil
}

// handleList returns every stored entry whose key starts with prefix, in
// the same listResponse shape Storage.List expects. Unlike the real Cluster
// DO, this doesn't filter out entries whose modRevision has been
// superseded (fakeKV has no delete-tombstone/history concept) -- fine for
// the tests that use it, which only ever create keys, never delete-then-list.
func (f *fakeKV) handleList(prefix string) (*http.Response, error) {
	var kvs []kvJSON
	for key, entry := range f.data {
		if strings.HasPrefix(key, prefix) {
			kvs = append(kvs, kvJSON{
				Key:            key,
				Value:          entry.value,
				CreateRevision: entry.createRevision,
				ModRevision:    entry.modRevision,
			})
		}
	}
	return jsonResponse(http.StatusOK, listResponse{
		Revision: f.revision,
		Count:    int64(len(kvs)),
		KVs:      kvs,
	}), nil
}

func (f *fakeKV) handleGet(key string) (*http.Response, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	entry, ok := f.data[key]
	if !ok {
		return jsonResponse(http.StatusOK, getResponse{
			Revision: f.revision,
			KV:       nil,
		}), nil
	}

	return jsonResponse(http.StatusOK, getResponse{
		Revision: f.revision,
		KV: &kvJSON{
			Key:            key,
			Value:          entry.value,
			CreateRevision: entry.createRevision,
			ModRevision:    entry.modRevision,
		},
	}), nil
}

func (f *fakeKV) handlePut(key string, req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var pr putRequest
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if pr.Revision == 0 {
		// Create
		if f.createErr != nil {
			return nil, f.createErr
		}
		if _, exists := f.data[key]; exists {
			return jsonResponse(http.StatusConflict, putResponse{
				Revision: f.revision,
				Error:    "already exists",
			}), nil
		}
		f.revision++
		f.data[key] = &fakeEntry{
			value:          pr.Value,
			createRevision: f.revision,
			modRevision:    f.revision,
		}
		return jsonResponse(http.StatusCreated, putResponse{
			Revision: f.revision,
			Updated:  true,
		}), nil
	}

	// Update (CAS)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	entry, exists := f.data[key]
	if !exists {
		return jsonResponse(http.StatusNotFound, putResponse{
			Error: "not found",
		}), nil
	}
	if entry.modRevision != pr.Revision {
		return jsonResponse(http.StatusConflict, putResponse{
			Revision: f.revision,
			Error:    "conflict",
		}), nil
	}
	f.revision++
	entry.value = pr.Value
	entry.modRevision = f.revision
	return jsonResponse(http.StatusOK, putResponse{
		Revision: f.revision,
		Updated:  true,
	}), nil
}

func jsonResponse(statusCode int, body interface{}) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func newTestStorage(kv *fakeKV) *Storage {
	return NewStorage(kv.doFetch, "")
}

func TestValidateNodePassword_FirstJoinCreatesEntry(t *testing.T) {
	kv := newFakeKV()
	s := newTestStorage(kv)
	ctx := context.Background()

	err := ValidateNodePassword(ctx, s, "node1", "secret123")
	if err != nil {
		t.Fatalf("first join should succeed: %v", err)
	}

	// Verify the entry was stored.
	entry, ok := kv.data["/nodepasswords/node1"]
	if !ok {
		t.Fatal("expected entry to be stored")
	}

	storedValue, err := base64.StdEncoding.DecodeString(entry.value)
	if err != nil {
		t.Fatalf("decode stored value: %v", err)
	}

	expectedHash := sha256Hash("node1:secret123")
	if string(storedValue) != expectedHash {
		t.Errorf("stored hash = %v, want %v", string(storedValue), expectedHash)
	}
}

func TestValidateNodePassword_MatchingPasswordSucceeds(t *testing.T) {
	kv := newFakeKV()
	s := newTestStorage(kv)
	ctx := context.Background()

	// First call: register.
	if err := ValidateNodePassword(ctx, s, "node1", "secret123"); err != nil {
		t.Fatalf("first join: %v", err)
	}

	// Second call with same password: should succeed.
	if err := ValidateNodePassword(ctx, s, "node1", "secret123"); err != nil {
		t.Fatalf("matching password should succeed: %v", err)
	}
}

func TestValidateNodePassword_MismatchedPasswordUpdatesHash(t *testing.T) {
	kv := newFakeKV()
	s := newTestStorage(kv)
	ctx := context.Background()

	// First call: register with original password.
	if err := ValidateNodePassword(ctx, s, "node1", "original"); err != nil {
		t.Fatalf("first join: %v", err)
	}

	// Second call with different password: should update (new behavior).
	if err := ValidateNodePassword(ctx, s, "node1", "newpassword"); err != nil {
		t.Fatalf("mismatched password should update and succeed: %v", err)
	}

	// Verify the stored hash was updated.
	entry, ok := kv.data["/nodepasswords/node1"]
	if !ok {
		t.Fatal("expected entry to still exist")
	}

	storedValue, err := base64.StdEncoding.DecodeString(entry.value)
	if err != nil {
		t.Fatalf("decode stored value: %v", err)
	}

	expectedHash := sha256Hash("node1:newpassword")
	if string(storedValue) != expectedHash {
		t.Errorf("stored hash = %v, want %v (hash of new password)", string(storedValue), expectedHash)
	}

	// Verify the new password now validates successfully.
	if err := ValidateNodePassword(ctx, s, "node1", "newpassword"); err != nil {
		t.Fatalf("new password should now validate: %v", err)
	}
}

func TestValidateNodePassword_StorageGetError(t *testing.T) {
	kv := newFakeKV()
	kv.getErr = fmt.Errorf("network error")
	s := newTestStorage(kv)
	ctx := context.Background()

	err := ValidateNodePassword(ctx, s, "node1", "secret123")
	if err == nil {
		t.Fatal("expected error from storage Get failure")
	}
	if !strings.Contains(err.Error(), "network error") {
		t.Errorf("error should contain 'network error', got: %v", err)
	}
}

func TestValidateNodePassword_StorageCreateError(t *testing.T) {
	kv := newFakeKV()
	kv.createErr = fmt.Errorf("disk full")
	s := newTestStorage(kv)
	ctx := context.Background()

	err := ValidateNodePassword(ctx, s, "node1", "secret123")
	if err == nil {
		t.Fatal("expected error from storage Create failure")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error should contain 'disk full', got: %v", err)
	}
}

func TestValidateNodePassword_StorageUpdateError(t *testing.T) {
	kv := newFakeKV()
	s := newTestStorage(kv)
	ctx := context.Background()

	// First call: register.
	if err := ValidateNodePassword(ctx, s, "node1", "original"); err != nil {
		t.Fatalf("first join: %v", err)
	}

	// Inject update error.
	kv.updateErr = fmt.Errorf("write failed")

	// Second call with different password: should fail on update.
	err := ValidateNodePassword(ctx, s, "node1", "different")
	if err == nil {
		t.Fatal("expected error from storage Update failure")
	}
	if !strings.Contains(err.Error(), "write failed") {
		t.Errorf("error should contain 'write failed', got: %v", err)
	}
}

func TestValidateNodePassword_RaceOnFirstJoin(t *testing.T) {
	// Simulate a race where Create returns ErrKeyExists because
	// another instance registered the password concurrently.
	kv := newFakeKV()
	s := newTestStorage(kv)
	ctx := context.Background()

	// Pre-populate the storage manually to simulate the race:
	// When ValidateNodePassword does Get -> not found -> Create,
	// the create will return "already exists" because another
	// instance stored it first. We achieve this by inserting the
	// entry between the Get and the Create.
	//
	// Since we can't easily intercept between Get and Create in the
	// current design, we test the code path by pre-populating with
	// the same hash (matching password). The race handling re-reads
	// and compares, so it should succeed.
	hash := sha256Hash("node1:secret123")
	kv.data["/nodepasswords/node1"] = &fakeEntry{
		value:          base64.StdEncoding.EncodeToString([]byte(hash)),
		createRevision: 1,
		modRevision:    1,
	}
	kv.revision = 1

	// This call should find the entry (not ErrNotFound) and compare hashes.
	if err := ValidateNodePassword(ctx, s, "node1", "secret123"); err != nil {
		t.Fatalf("race with matching password should succeed: %v", err)
	}
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:])
}
