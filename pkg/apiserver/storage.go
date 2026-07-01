package apiserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrKeyExists = errors.New("already exists")
)

// StoredObject represents a key-value entry stored in the KineStore Durable Object.
type StoredObject struct {
	Key            string
	Value          []byte // raw bytes (decoded from base64)
	CreateRevision int64
	ModRevision    int64
}

// Storage is a client that talks to the KineStore Durable Object via HTTP.
type Storage struct {
	doFetch func(req *http.Request) (*http.Response, error)
	prefix  string // "/registry"
}

// NewStorage creates a new Storage client.
// doFetch is the function used to send HTTP requests to the Durable Object
// (typically cloudflare.DurableObjectStub.Fetch).
// prefix is the key prefix for all storage operations (e.g. "/registry").
func NewStorage(doFetch func(req *http.Request) (*http.Response, error), prefix string) *Storage {
	return &Storage{
		doFetch: doFetch,
		prefix:  prefix,
	}
}

const doBaseURL = "http://do.internal"

// kvJSON is the JSON representation of a key-value entry from the DO.
type kvJSON struct {
	Key            string `json:"key"`
	CreateRevision int64  `json:"createRevision"`
	ModRevision    int64  `json:"modRevision"`
	Value          string `json:"value"` // base64-encoded
	Lease          int64  `json:"lease"`
}

// getResponse is the JSON response from GET /key/{key...}.
type getResponse struct {
	Revision int64   `json:"revision"`
	KV       *kvJSON `json:"kv"`
}

// putRequest is the JSON body for PUT /key/{key...}.
type putRequest struct {
	Value    string `json:"value"`    // base64-encoded
	Revision int64  `json:"revision"` // 0 for create, N for CAS update
}

// putResponse is the JSON response from PUT /key/{key...}.
type putResponse struct {
	Revision int64   `json:"revision"`
	KV       *kvJSON `json:"kv,omitempty"`
	Updated  bool    `json:"updated"`
	Error    string  `json:"error,omitempty"`
}

// deleteResponse is the JSON response from DELETE /key/{key...}.
type deleteResponse struct {
	Revision int64   `json:"revision"`
	KV       *kvJSON `json:"kv,omitempty"`
	Deleted  bool    `json:"deleted"`
}

// listResponse is the JSON response from GET /list/{prefix...}.
type listResponse struct {
	Revision int64    `json:"revision"`
	Count    int64    `json:"count"`
	KVs      []kvJSON `json:"kvs"`
}

// revisionResponse is the JSON response from GET /revision.
type revisionResponse struct {
	Revision int64 `json:"revision"`
}

// kvToStoredObject converts a kvJSON to a StoredObject, decoding the base64 value.
func kvToStoredObject(kv *kvJSON) (*StoredObject, error) {
	value, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, fmt.Errorf("decode base64 value: %w", err)
	}
	return &StoredObject{
		Key:            kv.Key,
		Value:          value,
		CreateRevision: kv.CreateRevision,
		ModRevision:    kv.ModRevision,
	}, nil
}

// Get retrieves a single key from the Durable Object.
// Returns ErrNotFound if the key does not exist.
func (s *Storage) Get(ctx context.Context, key string) (*StoredObject, error) {
	url := doBaseURL + "/key" + s.prefix + key
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("storage get: create request: %w", err)
	}

	resp, err := s.doFetch(req)
	if err != nil {
		return nil, fmt.Errorf("storage get: fetch: %w", err)
	}
	defer resp.Body.Close()

	var result getResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("storage get: decode response: %w", err)
	}

	if result.KV == nil {
		return nil, ErrNotFound
	}

	obj, err := kvToStoredObject(result.KV)
	if err != nil {
		return nil, fmt.Errorf("storage get: %w", err)
	}
	return obj, nil
}

// Create stores a new key in the Durable Object.
// Returns ErrKeyExists if the key already exists.
// Returns the new revision on success.
func (s *Storage) Create(ctx context.Context, key string, data []byte) (int64, error) {
	encoded := base64.StdEncoding.EncodeToString(data)
	body, err := json.Marshal(putRequest{
		Value:    encoded,
		Revision: 0, // 0 means create
	})
	if err != nil {
		return 0, fmt.Errorf("storage create: marshal body: %w", err)
	}

	url := doBaseURL + "/key" + s.prefix + key
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("storage create: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.doFetch(req)
	if err != nil {
		return 0, fmt.Errorf("storage create: fetch: %w", err)
	}
	defer resp.Body.Close()

	var result putResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("storage create: decode response: %w", err)
	}

	if resp.StatusCode == http.StatusConflict {
		return 0, ErrKeyExists
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("storage create: unexpected status %d: %s", resp.StatusCode, result.Error)
	}

	return result.Revision, nil
}

// Update performs a CAS (compare-and-swap) update on an existing key.
// The revision must match the current revision of the key.
// Returns the new revision, whether the update was applied, and any error.
// Returns ErrConflict if the revision does not match.
func (s *Storage) Update(ctx context.Context, key string, data []byte, revision int64) (int64, bool, error) {
	encoded := base64.StdEncoding.EncodeToString(data)
	body, err := json.Marshal(putRequest{
		Value:    encoded,
		Revision: revision,
	})
	if err != nil {
		return 0, false, fmt.Errorf("storage update: marshal body: %w", err)
	}

	url := doBaseURL + "/key" + s.prefix + key
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return 0, false, fmt.Errorf("storage update: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.doFetch(req)
	if err != nil {
		return 0, false, fmt.Errorf("storage update: fetch: %w", err)
	}
	defer resp.Body.Close()

	var result putResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, false, fmt.Errorf("storage update: decode response: %w", err)
	}

	if resp.StatusCode == http.StatusConflict {
		return result.Revision, false, ErrConflict
	}
	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("storage update: unexpected status %d: %s", resp.StatusCode, result.Error)
	}

	return result.Revision, result.Updated, nil
}

// Delete removes a key from the Durable Object.
// The revision must match the current revision of the key.
// Returns the revision after deletion.
func (s *Storage) Delete(ctx context.Context, key string, revision int64) (int64, error) {
	url := doBaseURL + "/key" + s.prefix + key + "?revision=" + strconv.FormatInt(revision, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return 0, fmt.Errorf("storage delete: create request: %w", err)
	}

	resp, err := s.doFetch(req)
	if err != nil {
		return 0, fmt.Errorf("storage delete: fetch: %w", err)
	}
	defer resp.Body.Close()

	var result deleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("storage delete: decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("storage delete: unexpected status %d", resp.StatusCode)
	}

	return result.Revision, nil
}

// List retrieves all keys matching the given prefix.
// limit controls the maximum number of results (0 means no limit).
// revision requests a consistent read at a specific revision (0 means latest).
// Returns the list of objects, the current revision, and any error.
func (s *Storage) List(ctx context.Context, prefix string, limit int64, revision int64) ([]*StoredObject, int64, error) {
	url := doBaseURL + "/list" + s.prefix + prefix
	sep := '?'
	if limit > 0 {
		url += string(sep) + "limit=" + strconv.FormatInt(limit, 10)
		sep = '&'
	}
	if revision > 0 {
		url += string(sep) + "revision=" + strconv.FormatInt(revision, 10)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("storage list: create request: %w", err)
	}

	resp, err := s.doFetch(req)
	if err != nil {
		return nil, 0, fmt.Errorf("storage list: fetch: %w", err)
	}
	defer resp.Body.Close()

	var result listResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("storage list: decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("storage list: unexpected status %d", resp.StatusCode)
	}

	objects := make([]*StoredObject, 0, len(result.KVs))
	for i := range result.KVs {
		obj, err := kvToStoredObject(&result.KVs[i])
		if err != nil {
			return nil, 0, fmt.Errorf("storage list: item %d: %w", i, err)
		}
		objects = append(objects, obj)
	}

	return objects, result.Revision, nil
}

// CurrentRevision returns the current revision number from the Durable Object.
func (s *Storage) CurrentRevision(ctx context.Context) (int64, error) {
	url := doBaseURL + "/revision"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("storage current revision: create request: %w", err)
	}

	resp, err := s.doFetch(req)
	if err != nil {
		return 0, fmt.Errorf("storage current revision: fetch: %w", err)
	}
	defer resp.Body.Close()

	var result revisionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("storage current revision: decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("storage current revision: unexpected status %d", resp.StatusCode)
	}

	return result.Revision, nil
}
