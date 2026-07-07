// Hand-curated: only the expansions this repo's three resource/v1 types
// need; upstream's generated_expansion.go also declares
// ResourceClaimTemplateExpansion for a type this repo doesn't use.
package v1

type DeviceClassExpansion interface{}

type ResourceClaimExpansion interface{}

type ResourceSliceExpansion interface{}
