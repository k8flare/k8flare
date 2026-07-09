package apidef

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewRESTMapper builds a static REST mapper covering every resource in
// Table. This project's GVK set is fixed at build time (Table is already
// the single source of truth every other derived artifact comes from --
// see this package's doc comment), so the real discovery-based
// RESTMapper machinery (which polls /apis to learn what's servable, then
// periodically Reset()s to pick up changes) is unnecessary: a
// meta.DefaultRESTMapper populated once from Table already knows the
// complete, unchanging answer. Used by pkg/controllers' garbagecollector
// wiring (the real upstream ownerReferences GC controller needs a
// meta.ResettableRESTMapper -- see resettableRESTMapper below for why
// Reset() is a safe no-op here).
func NewRESTMapper() meta.ResettableRESTMapper {
	seen := make(map[schema.GroupVersion]bool, len(Table))
	for _, r := range Table {
		seen[r.GroupVersion] = true
	}
	gvs := make([]schema.GroupVersion, 0, len(seen))
	for gv := range seen {
		gvs = append(gvs, gv)
	}

	mapper := meta.NewDefaultRESTMapper(gvs)
	for _, r := range Table {
		scope := meta.RESTScopeNamespace
		if !r.Namespaced {
			scope = meta.RESTScopeRoot
		}
		singular := r.Singular
		if singular == "" {
			// Matches upstream's own fallback shape for the rare
			// resource that leaves Singular unset (see ResourceDef's
			// doc comment) -- not authoritative, but this project's
			// garbagecollector wiring never calls
			// ResourceSingularizer, only KindFor/RESTMapping (see
			// pkg/controllers' gc.go), so an approximate singular
			// costs nothing in practice.
			singular = strings.ToLower(r.Kind)
		}
		mapper.AddSpecific(
			r.GroupVersion.WithKind(r.Kind),
			r.GroupVersion.WithResource(r.Resource),
			r.GroupVersion.WithResource(singular),
			scope,
		)
	}
	return &resettableRESTMapper{DefaultRESTMapper: mapper}
}

// resettableRESTMapper adapts *meta.DefaultRESTMapper (which has no
// Reset method) to meta.ResettableRESTMapper. Reset is a no-op: the real
// garbagecollector controller calls it after a "no kind registered for
// GVR" RESTMapping error, on the theory that discovery may have learned
// about a newly installed CRD/aggregated API since the mapper was built.
// This project's resource set never changes at runtime (Table is fixed
// at build time), so there is nothing a Reset could ever pick up -- the
// same RESTMapping error will recur identically, exactly like it would
// if this mapper had genuinely just re-synced against an unchanged
// discovery document.
type resettableRESTMapper struct {
	*meta.DefaultRESTMapper
}

func (m *resettableRESTMapper) Reset() {}
