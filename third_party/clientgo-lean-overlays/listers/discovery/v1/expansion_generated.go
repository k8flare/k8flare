// Hand-curated: identical to upstream -- discovery/v1 only has
// EndpointSlice, which this repo already uses in full. Kept here (rather
// than left unpruned) only for consistency with the other four groups.
package v1

type EndpointSliceListerExpansion interface{}
type EndpointSliceNamespaceListerExpansion interface{}
