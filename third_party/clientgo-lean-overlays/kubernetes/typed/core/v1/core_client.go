// Hand-curated interface-only extraction. See
// third_party/clientgo-lean-overlays/README.md.
//
// Narrowed to the four getters this repo's controllers/scheduler actually
// call (Pods, Nodes, Services, Endpoints) -- upstream's CoreV1Interface
// embeds twelve more (ComponentStatuses, ConfigMaps, Events, LimitRanges,
// Namespaces, PersistentVolume[Claim]s, PodTemplates,
// ReplicationControllers, ResourceQuotas, Secrets, ServiceAccounts) that
// this repo's leanclient never implements; dropping them from the
// interface itself (rather than implementing kubernetes.Interface's full
// width with panic stubs, third_party/leanclient-kcm's older approach)
// means their own typed/*.go and applyconfigurations/*.go files never
// need to be mirrored/pruned at all.
package v1

import (
	rest "k8s.io/client-go/rest"
)

type CoreV1Interface interface {
	RESTClient() rest.Interface
	PodsGetter
	NodesGetter
	ServicesGetter
	EndpointsGetter
}
