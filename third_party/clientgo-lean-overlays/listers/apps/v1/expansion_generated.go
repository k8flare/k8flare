// Hand-curated: trimmed to ControllerRevision/Deployment (the two of this
// repo's four apps/v1 types with no bespoke *_expansion.go of their own --
// ReplicaSet/DaemonSet's real GetPodReplicaSets/GetPodDaemonSets methods
// live in their own upstream files, kept unmodified). See
// third_party/clientgo-lean-overlays/README.md.
package v1

type ControllerRevisionListerExpansion interface{}
type ControllerRevisionNamespaceListerExpansion interface{}
type DeploymentListerExpansion interface{}
type DeploymentNamespaceListerExpansion interface{}
