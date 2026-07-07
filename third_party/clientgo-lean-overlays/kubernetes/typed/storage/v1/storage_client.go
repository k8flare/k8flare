// Hand-curated interface-only extraction. See
// third_party/clientgo-lean-overlays/README.md.
//
// Narrowed to the five getters this repo actually needs (CSIDrivers,
// CSINodes, CSIStorageCapacities, StorageClasses, VolumeAttachments) --
// upstream's StorageV1Interface also embeds VolumeAttributesClassesGetter,
// which is dropped here.
package v1

import (
	rest "k8s.io/client-go/rest"
)

type StorageV1Interface interface {
	RESTClient() rest.Interface
	CSIDriversGetter
	CSINodesGetter
	CSIStorageCapacitiesGetter
	StorageClassesGetter
	VolumeAttachmentsGetter
}
