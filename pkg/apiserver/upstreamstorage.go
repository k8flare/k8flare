package apiserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
)

// KineStorage adapts this project's kine-protocol Storage client (the
// Cluster DO transport, storage.go) to upstream's storage.Interface, so
// genericregistry.Store can run unmodified on top of the DO (S25 phase 1).
//
// Byte-compatibility is deliberate: objects are written with the same
// EncodeToStorage / read with the same DecodeFromStorage the hand-written
// ResourceStore uses, so a resource can switch between the two layers
// (per-resource, during the migration) without any data conversion.
//
// Watch is NOT served here: watch fan-out lives in the TS layer
// (WatchHub + gateway streaming) and never reaches the Go apiserver --
// genericregistry.Store's non-watch verbs never call it.
type KineStorage struct {
	s         *Storage
	newFunc   func() runtime.Object
	versioner storage.Versioner
}

var _ storage.Interface = (*KineStorage)(nil)

func NewKineStorage(s *Storage, newFunc func() runtime.Object) *KineStorage {
	return &KineStorage{s: s, newFunc: newFunc, versioner: storage.APIObjectVersioner{}}
}

func (k *KineStorage) Versioner() storage.Versioner { return k.versioner }

// Keys arrive registry-relative ("/configmaps/<ns>/<name>" -- the
// Store's KeyFunc is built from the resource prefix); Storage itself
// prepends "/registry", same as ResourceStore.
func (k *KineStorage) decodeInto(data []byte, rev int64, objPtr runtime.Object) error {
	if err := DecodeFromStorage(data, objPtr); err != nil {
		return fmt.Errorf("kinestorage decode: %w", err)
	}
	return k.versioner.UpdateObject(objPtr, uint64(rev))
}

func (k *KineStorage) Create(ctx context.Context, key string, obj, out runtime.Object, _ uint64) error {
	if err := k.versioner.PrepareObjectForStorage(obj); err != nil {
		return err
	}
	data, err := EncodeToStorage(obj)
	if err != nil {
		return err
	}
	rev, err := k.s.Create(ctx, key, data)
	if err != nil {
		if errors.Is(err, ErrKeyExists) {
			return storage.NewKeyExistsError(key, 0)
		}
		return err
	}
	if out != nil {
		return k.decodeInto(data, rev, out)
	}
	return nil
}

func (k *KineStorage) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	stored, err := k.s.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if opts.IgnoreNotFound {
				return runtime.SetZeroValue(objPtr)
			}
			return storage.NewKeyNotFoundError(key, 0)
		}
		return err
	}
	return k.decodeInto(stored.Value, stored.ModRevision, objPtr)
}

func (k *KineStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	if !opts.Recursive {
		// A list whose predicate pins metadata.name (MatchesSingle) is
		// turned by genericregistry.Store into a NON-recursive GetList on
		// that one object's key -- treating it as a prefix instead
		// silently returns nothing, which is what a
		// "?fieldSelector=metadata.name=..." list did when this migration
		// first ran. etcd3's store makes the same distinction.
		return k.getListSingle(ctx, key, opts, listObj)
	}
	prefix := key
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	objs, rev, err := k.s.List(ctx, prefix, 0, 0)
	if err != nil {
		return err
	}
	items := make([]runtime.Object, 0, len(objs))
	for _, so := range objs {
		obj := k.newFunc()
		if err := k.decodeInto(so.Value, so.ModRevision, obj); err != nil {
			return err
		}
		ok, err := opts.Predicate.Matches(obj)
		if err != nil {
			return err
		}
		if ok {
			items = append(items, obj)
		}
	}
	if err := meta.SetList(listObj, items); err != nil {
		return err
	}
	return k.updateListRevision(ctx, listObj, rev)
}

// updateListRevision stamps the list's resourceVersion. A facet that has
// never been written to reports revision 0, and the upstream versioner
// rejects that as an illegal list resourceVersion ("illegal resource
// version from storage: 0", hit by the orphan sweep listing events in a
// fresh namespace); the cluster's global revision is the right answer,
// since an empty listing is still current as of now.
func (k *KineStorage) updateListRevision(ctx context.Context, listObj runtime.Object, rev int64) error {
	if rev == 0 {
		cur, err := k.s.CurrentRevision(ctx)
		if err != nil {
			return err
		}
		rev = cur
	}
	return k.versioner.UpdateList(listObj, uint64(rev), "", nil)
}

// getListSingle serves the non-recursive GetList: at most one item, the
// object stored under key itself. A missing key is an empty list, not an
// error.
func (k *KineStorage) getListSingle(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	var items []runtime.Object
	rev := int64(0)
	stored, err := k.s.Get(ctx, key)
	switch {
	case err == nil:
		obj := k.newFunc()
		if err := k.decodeInto(stored.Value, stored.ModRevision, obj); err != nil {
			return err
		}
		ok, err := opts.Predicate.Matches(obj)
		if err != nil {
			return err
		}
		if ok {
			items = append(items, obj)
		}
		rev = stored.ModRevision
	case errors.Is(err, ErrNotFound):
	default:
		return err
	}
	if err := meta.SetList(listObj, items); err != nil {
		return err
	}
	return k.updateListRevision(ctx, listObj, rev)
}

func (k *KineStorage) Delete(ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, _ runtime.Object, _ storage.DeleteOptions) error {
	for attempt := 0; attempt < deleteConflictRetries; attempt++ {
		stored, err := k.s.Get(ctx, key)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return storage.NewKeyNotFoundError(key, 0)
			}
			return err
		}
		obj := k.newFunc()
		if err := k.decodeInto(stored.Value, stored.ModRevision, obj); err != nil {
			return err
		}
		if preconditions != nil {
			if err := preconditions.Check(key, obj); err != nil {
				return err
			}
		}
		if validateDeletion != nil {
			if err := validateDeletion(ctx, obj); err != nil {
				return err
			}
		}
		if _, err := k.s.Delete(ctx, key, stored.ModRevision); err != nil {
			if errors.Is(err, ErrConflict) {
				continue
			}
			return err
		}
		if out != nil {
			return k.decodeInto(stored.Value, stored.ModRevision, out)
		}
		return nil
	}
	return storage.NewResourceVersionConflictsError(key, 0)
}

func (k *KineStorage) GuaranteedUpdate(ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, _ runtime.Object) error {
	// Same bound as ResourceStore.Delete's conflict retry (store.go).
	for attempt := 0; attempt < deleteConflictRetries; attempt++ {
		var current runtime.Object
		var currentRev int64
		stored, err := k.s.Get(ctx, key)
		switch {
		case err == nil:
			current = k.newFunc()
			if err := k.decodeInto(stored.Value, stored.ModRevision, current); err != nil {
				return err
			}
			currentRev = stored.ModRevision
		case errors.Is(err, ErrNotFound):
			if !ignoreNotFound {
				return storage.NewKeyNotFoundError(key, 0)
			}
			current = k.newFunc()
		default:
			return err
		}

		if preconditions != nil {
			if err := preconditions.Check(key, current); err != nil {
				return err
			}
		}
		updated, _, err := tryUpdate(current, storage.ResponseMeta{ResourceVersion: uint64(currentRev)})
		if err != nil {
			return err
		}
		if err := k.versioner.PrepareObjectForStorage(updated); err != nil {
			return err
		}
		data, err := EncodeToStorage(updated)
		if err != nil {
			return err
		}

		var newRev int64
		if currentRev == 0 {
			newRev, err = k.s.Create(ctx, key, data)
			if errors.Is(err, ErrKeyExists) {
				continue
			}
		} else {
			var ok bool
			newRev, ok, err = k.s.Update(ctx, key, data, currentRev)
			if err == nil && !ok {
				continue
			}
		}
		if err != nil {
			if errors.Is(err, ErrConflict) {
				continue
			}
			return err
		}
		return k.decodeInto(data, newRev, destination)
	}
	return storage.NewResourceVersionConflictsError(key, 0)
}

func (k *KineStorage) Count(_ string) (int64, error) {
	// Only the APF object-count tracker calls Count, and it isn't wired
	// on this platform.
	return 0, fmt.Errorf("kinestorage: Count is not supported")
}

func (k *KineStorage) Watch(_ context.Context, key string, _ storage.ListOptions) (watch.Interface, error) {
	return nil, fmt.Errorf("kinestorage: Watch is served by the TS watch layer, not storage.Interface (key %q)", key)
}

func (k *KineStorage) Stats(_ context.Context) (storage.Stats, error) {
	return storage.Stats{}, fmt.Errorf("kinestorage: Stats is not supported")
}

func (k *KineStorage) ReadinessCheck() error { return nil }

func (k *KineStorage) RequestWatchProgress(_ context.Context) error { return nil }

func (k *KineStorage) GetCurrentResourceVersion(ctx context.Context) (uint64, error) {
	// The DO's revision counter is global (kine semantics); an empty-
	// prefix list returns it without decoding any rows.
	_, rev, err := k.s.List(ctx, "/", 1, 0)
	if err != nil {
		return 0, err
	}
	return uint64(rev), nil
}

func (k *KineStorage) EnableResourceSizeEstimation(_ storage.KeysFunc) error {
	return fmt.Errorf("kinestorage: resource size estimation is not supported")
}

func (k *KineStorage) CompactRevision() int64 { return 0 }
