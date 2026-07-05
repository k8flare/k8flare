// Hand-curated interface-only extraction. See
// third_party/clientgo-lean-overlays/README.md.
package v1

import (
	rest "k8s.io/client-go/rest"
)

type CoordinationV1Interface interface {
	RESTClient() rest.Interface
	LeasesGetter
}

// CoordinationV1Client is used to interact with features provided by the coordination.k8s.io group.
