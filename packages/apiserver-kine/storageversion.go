package kine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
)

const (
	storageVersionsKey  = "/k8flare/storageversions"
	migrationScanLimit  = 500
	MigrationPassBudget = 200
)

type StoredResource struct {
	Name  string
	Hash  string
	Codec runtime.Codec
	New   func() runtime.Object
}

type storageVersionRecord struct {
	Hash   string `json:"hash"`
	Cursor string `json:"cursor,omitempty"`
	Error  string `json:"error,omitempty"`
}

type MigrationStatus struct {
	Pending   []string          `json:"pending"`
	Failed    map[string]string `json:"failed,omitempty"`
	Rewritten int               `json:"rewritten"`
	Done      bool              `json:"done"`
}

func (c *Client) readStorageVersions(ctx context.Context) (map[string]storageVersionRecord, int64, error) {
	records := map[string]storageVersionRecord{}
	kv, _, err := c.Get(ctx, storageVersionsKey)
	if err == ErrNotFound {
		return records, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, 0, err
	}
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, 0, err
	}
	return records, kv.ModRevision, nil
}

func (c *Client) writeStorageVersions(ctx context.Context, records map[string]storageVersionRecord, revision int64) error {
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	_, err = c.Put(ctx, storageVersionsKey, raw, revision)
	if err == ErrConflict {
		return nil
	}
	return err
}

func (r storageVersionRecord) current(res StoredResource) bool {
	return r.Hash == res.Hash && r.Cursor == "" && r.Error == ""
}

func summarize(resources []StoredResource, records map[string]storageVersionRecord) MigrationStatus {
	status := MigrationStatus{Pending: []string{}}
	for _, res := range resources {
		rec := records[res.Name]
		switch {
		case rec.Hash == res.Hash && rec.Error != "":
			if status.Failed == nil {
				status.Failed = map[string]string{}
			}
			status.Failed[res.Name] = rec.Error
		case !rec.current(res):
			status.Pending = append(status.Pending, res.Name)
		}
	}
	status.Done = len(status.Pending) == 0
	return status
}

func (c *Client) StorageVersionStatus(ctx context.Context, resources []StoredResource) (MigrationStatus, error) {
	records, _, err := c.readStorageVersions(ctx)
	if err != nil {
		return MigrationStatus{}, err
	}
	return summarize(resources, records), nil
}

func (c *Client) MigrateStorage(ctx context.Context, resources []StoredResource, budget int) (MigrationStatus, error) {
	records, revision, err := c.readStorageVersions(ctx)
	if err != nil {
		return MigrationStatus{}, err
	}
	sorted := append([]StoredResource(nil), resources...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	rewritten := 0
	for _, res := range sorted {
		rec := records[res.Name]
		if rec.Hash == res.Hash && (rec.current(res) || rec.Error != "") {
			continue
		}
		if rec.Hash != res.Hash {
			rec = storageVersionRecord{Hash: res.Hash}
		}
		n, err := c.rewriteResource(ctx, res, &rec, budget-rewritten)
		rewritten += n
		if err != nil {
			return MigrationStatus{}, err
		}
		records[res.Name] = rec
		if rewritten >= budget {
			break
		}
	}
	if err := c.writeStorageVersions(ctx, records, revision); err != nil {
		return MigrationStatus{}, err
	}
	status := summarize(resources, records)
	status.Rewritten = rewritten
	return status, nil
}

func (c *Client) rewriteResource(ctx context.Context, res StoredResource, rec *storageVersionRecord, budget int) (int, error) {
	prefix := registryPrefix + "/" + res.Name + "/"
	from := rec.Cursor
	rewritten := 0
	for {
		page, _, more, err := c.List(ctx, prefix, from, migrationScanLimit)
		if err != nil {
			return rewritten, err
		}
		for _, kv := range page {
			if rewritten >= budget {
				rec.Cursor = kv.Key
				return rewritten, nil
			}
			changed, err := c.rewriteObject(ctx, res, kv)
			if err != nil {
				rec.Error = kv.Key + ": " + err.Error()
				rec.Cursor = ""
				return rewritten, nil
			}
			if changed {
				rewritten++
			}
		}
		if !more || len(page) == 0 {
			rec.Cursor = ""
			return rewritten, nil
		}
		from = page[len(page)-1].Key + "\x00"
	}
}

func (c *Client) rewriteObject(ctx context.Context, res StoredResource, kv KV) (bool, error) {
	stored, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return false, err
	}
	obj := res.New()
	if _, _, err := res.Codec.Decode(stored, nil, obj); err != nil {
		return false, err
	}
	current, err := runtime.Encode(res.Codec, obj)
	if err != nil {
		return false, err
	}
	if bytes.Equal(stored, current) {
		return false, nil
	}
	_, err = c.Put(ctx, kv.Key, current, kv.ModRevision)
	if err == ErrConflict || err == ErrNotFound {
		return false, nil
	}
	return err == nil, err
}
