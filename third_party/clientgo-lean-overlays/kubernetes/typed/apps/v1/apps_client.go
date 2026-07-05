// Hand-curated interface-only extraction. See
// third_party/clientgo-lean-overlays/README.md. Narrowed: drops
// StatefulSetsGetter (upstream AppsV1Interface has it; no controller this
// repo enables uses StatefulSets).
package v1

import (
	rest "k8s.io/client-go/rest"
)

type AppsV1Interface interface {
	RESTClient() rest.Interface
	ControllerRevisionsGetter
	DaemonSetsGetter
	DeploymentsGetter
	ReplicaSetsGetter
}
