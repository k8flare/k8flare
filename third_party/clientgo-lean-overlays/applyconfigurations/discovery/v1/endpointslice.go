// Hand-curated: satisfies EndpointSliceInterface's Apply/ApplyStatus method
// signatures with an intentionally empty type. See
// third_party/clientgo-lean-overlays/README.md -- this repo's controllers
// never use Server-Side Apply (confirmed by grep, zero call sites), so no
// real apply-configuration builder is needed; what matters is that the
// package containing EndpointSliceApplyConfiguration has no unrelated
// sibling type files (that's what actually costs ~40MiB, not this type's
// own complexity -- verified empirically, not assumed).
package v1

type EndpointSliceApplyConfiguration struct{}

func (b *EndpointSliceApplyConfiguration) IsApplyConfiguration() {}
