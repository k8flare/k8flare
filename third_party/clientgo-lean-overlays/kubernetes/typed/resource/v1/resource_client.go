// Hand-curated interface-only extraction. See
// third_party/clientgo-lean-overlays/README.md.
//
// Narrowed to the three getters this repo actually needs (DeviceClasses,
// ResourceClaims, ResourceSlices) -- upstream's ResourceV1Interface also
// embeds ResourceClaimTemplatesGetter, which is dropped here.
package v1

import (
	rest "k8s.io/client-go/rest"
)

type ResourceV1Interface interface {
	RESTClient() rest.Interface
	DeviceClassesGetter
	ResourceClaimsGetter
	ResourceSlicesGetter
}
