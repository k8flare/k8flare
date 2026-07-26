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
		rs := NewResourceStore(s, def.Resource, def.Namespaced, def.New, def.NewList)
		// Migration to upstream genericregistry.Store (S25 phase 1b),
		// resource by resource: a listed resource routes its
		// single-object verbs through the real upstream registry
		// (ObjectMeta lifecycle, preconditions, finalizer-aware delete)
		// instead of the hand-written store.go path. Grow this set as
		// each resource's behavior is verified equivalent by the suite.
		if upstreamMigrated[gv.String()+"/"+def.Resource] {
			rs.upstream = NewUpstreamStore(s, gv, def.Resource, def.Singular, def.Namespaced, def.New, def.NewList)
		}
		stores[def.Resource] = rs
	}
	return stores
}

// upstreamMigrated lists the resources served by genericregistry.Store.
var upstreamMigrated = map[string]bool{
	"v1/configmaps":                        true,
	"v1/secrets":                           true,
	"v1/serviceaccounts":                   true,
	"v1/limitranges":                       true,
	"v1/resourcequotas":                    true,
	"policy/v1/poddisruptionbudgets":       true,
	"scheduling.k8s.io/v1/priorityclasses": true,
	"coordination.k8s.io/v1/leases":        true,

	"v1/namespaces":                                    true,
	"v1/nodes":                                         true,
	"v1/services":                                      true,
	"v1/endpoints":                                     true,
	"v1/events":                                        true,
	"v1/persistentvolumes":                             true,
	"v1/persistentvolumeclaims":                        true,
	"discovery.k8s.io/v1/endpointslices":               true,
	"networking.k8s.io/v1/ingresses":                   true,
	"networking.k8s.io/v1/ingressclasses":              true,
	"networking.k8s.io/v1/networkpolicies":             true,
	"networking.k8s.io/v1/servicecidrs":                true,
	"storage.k8s.io/v1/storageclasses":                 true,
	"storage.k8s.io/v1/csidrivers":                     true,
	"storage.k8s.io/v1/csinodes":                       true,
	"node.k8s.io/v1/runtimeclasses":                    true,
	"resource.k8s.io/v1/resourceclaims":                true,
	"resource.k8s.io/v1/resourceslices":                true,
	"resource.k8s.io/v1/deviceclasses":                 true,
	"apps/v1/controllerrevisions":                      true,
	"rbac.authorization.k8s.io/v1/roles":               true,
	"rbac.authorization.k8s.io/v1/rolebindings":        true,
	"rbac.authorization.k8s.io/v1/clusterroles":        true,
	"rbac.authorization.k8s.io/v1/clusterrolebindings": true,
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
