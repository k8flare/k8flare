package apiserver

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// NewResourceStoresForGroupVersion creates a ResourceStore for every
// apidef.Table entry in the given GroupVersion, keyed by resource name.
//
// Replaces what used to be one hand-written NewXStore constructor plus one
// hand-written NewXStores map-builder per API group (resources.go, 617
// lines for 10 groups): adding a resource to apidef.Table is now the only
// step needed for it to gain a working store.
func NewResourceStoresForGroupVersion(s *Storage, gv schema.GroupVersion) map[string]*ResourceStore {
	stores := map[string]*ResourceStore{}
	for _, def := range apidef.ForGroupVersion(gv) {
		stores[def.Resource] = NewResourceStore(s, def.Resource, def.Namespaced, def.New, def.NewList)
	}
	return stores
}

// NamespacedResourceStores collects every ResourceStore that is namespaced
// across however many store maps are passed in, de-duplicating by resource
// name (e.g. "events" may appear in more than one map pointing at the same
// underlying storage).
//
// Derived from the store maps themselves (which are in turn derived from
// apidef.Table) rather than hand-listed, so a future namespaced resource
// type added to the table is automatically swept on namespace deletion
// without a second place to remember to update.
func NamespacedResourceStores(storeMaps ...map[string]*ResourceStore) []*ResourceStore {
	seen := make(map[string]bool)
	var out []*ResourceStore
	for _, m := range storeMaps {
		for _, rs := range m {
			if !rs.namespaced || seen[rs.resource] {
				continue
			}
			seen[rs.resource] = true
			out = append(out, rs)
		}
	}
	return out
}
