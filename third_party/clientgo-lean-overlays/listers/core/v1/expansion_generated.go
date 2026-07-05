// Hand-curated: trimmed to the four types this repo's controllers/
// scheduler actually list (Pod, Node, Service, Endpoints); upstream's
// expansion_generated.go declares eight more marker interfaces for
// core/v1 types this repo doesn't use. See
// third_party/clientgo-lean-overlays/README.md.
package v1

type PodListerExpansion interface{}
type PodNamespaceListerExpansion interface{}
type NodeListerExpansion interface{}
type ServiceListerExpansion interface{}
type ServiceNamespaceListerExpansion interface{}
type EndpointsListerExpansion interface{}
type EndpointsNamespaceListerExpansion interface{}
