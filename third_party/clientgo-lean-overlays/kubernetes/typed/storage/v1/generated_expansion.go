// Hand-curated: only the expansions this repo's five storage/v1 types
// need; upstream's generated_expansion.go also declares
// VolumeAttributesClassExpansion for a type this repo doesn't use.
package v1

type CSIDriverExpansion interface{}

type CSINodeExpansion interface{}

type CSIStorageCapacityExpansion interface{}

type StorageClassExpansion interface{}

type VolumeAttachmentExpansion interface{}
