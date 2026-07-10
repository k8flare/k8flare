//go:build schedwidth

/*
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
*/

// k8flare schedwidth overlay: informers/policy's own Interface narrowed
// to V1 -- the only version pkg/scheduler's real call sites reach (grep
// across pkg/scheduler confirms `.Policy().V1()` only, framework/
// preemption's unconditional PodDisruptionBudget lister). See
// pkg/clientgo-lean-overlays/informers/apps/interface_schedwidth.go's
// doc comment for the general mechanism.

package policy

import (
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	v1 "k8s.io/client-go/informers/policy/v1"
)

// Interface provides access to this group's V1, the only version
// pkg/scheduler's own call sites reach.
type Interface interface {
	// V1 provides access to shared informers for resources in V1.
	V1() v1.Interface
}

type group struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &group{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// V1 returns a new v1.Interface.
func (g *group) V1() v1.Interface {
	return v1.New(g.factory, g.namespace, g.tweakListOptions)
}
