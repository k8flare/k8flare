//go:build js && wasm && leanwidth

// leanwidth build: the KCM WASM binary is built with -tags leanwidth
// against go.wasm.mod, whose k8s.io/client-go replace points at the
// width-pruned .build/clientgo-lean-mirror (kubernetes.Interface has
// only this repo's five real groups plus Discovery -- see
// pkg/clientgo-lean-overlays/kubernetes/clientset_leanwidth.go).
// Merely importing a typed group package links that group's entire
// generated API surface via its proto-registration init()s, so the wide
// stubs.go (which imports all ~50 other groups to satisfy the full-width
// Interface) is exactly what this tag exists to exclude. Only
// Discovery() remains to stub here.
package clientset

import (
	discovery "k8s.io/client-go/discovery"
	schedulingv1alpha2 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha2"
)

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	panic("leanclient: Discovery not implemented (unused by this repo's controllers)")
}

func (c *Clientset) SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface {
	panic("leanclient: SchedulingV1alpha2 not implemented (job controller's PodGroup informer is never started by this repo's controller set)")
}
