// Hand-curated interface-only extraction. See
// third_party/clientgo-lean-overlays/README.md.
package v1

import (
	rest "k8s.io/client-go/rest"
)

type DiscoveryV1Interface interface {
	RESTClient() rest.Interface
	EndpointSlicesGetter
}

// DiscoveryV1Client is used to interact with features provided by the discovery.k8s.io group.
