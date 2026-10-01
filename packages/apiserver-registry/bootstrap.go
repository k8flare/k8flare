package registry

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
)

const bootstrapMarkerPrefix = "/k8flare/bootstrap/"

func StoreClient(store *genericregistry.Store) *kine.Client {
	if store == nil {
		return nil
	}
	if s, ok := store.Storage.Storage.(interface{ Client() *kine.Client }); ok {
		return s.Client()
	}
	return nil
}

func HashObjects(objs ...any) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	for _, obj := range objs {
		_ = enc.Encode(obj)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func RunBootstrap(ctx context.Context, store *genericregistry.Store, name, hash string, ensure func(context.Context) error) error {
	client := StoreClient(store)
	if client == nil {
		return ensure(ctx)
	}
	key := bootstrapMarkerPrefix + name
	kv, _, err := client.Get(ctx, key)
	if err == nil && kv != nil {
		if raw, decErr := base64.StdEncoding.DecodeString(kv.Value); decErr == nil && string(raw) == hash {
			return nil
		}
	}
	if err := ensure(ctx); err != nil {
		return err
	}
	var rev int64
	if kv != nil {
		rev = kv.ModRevision
	}
	if _, err := client.Put(ctx, key, []byte(hash), rev); err != nil && err != kine.ErrConflict {
		return err
	}
	return nil
}
