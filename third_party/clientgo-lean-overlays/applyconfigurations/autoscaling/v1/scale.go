// Hand-curated: satisfies ReplicaSetInterface/DeploymentInterface's
// ApplyScale method signature with an intentionally empty type (this
// repo's controllers never touch the /scale subresource -- see
// README.md's "Deployment/ReplicaSet basics" note on /scale being
// unimplemented). See third_party/clientgo-lean-overlays/README.md for
// why the package must also have no unrelated sibling files
// (HorizontalPodAutoscaler and friends, in the same package upstream).
package v1

type ScaleApplyConfiguration struct{}

func (b *ScaleApplyConfiguration) IsApplyConfiguration() {}
