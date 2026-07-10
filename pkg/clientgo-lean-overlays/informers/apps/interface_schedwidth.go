//go:build schedwidth

/*
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
*/

// k8flare schedwidth overlay: informers/apps' own Interface narrowed to
// V1 -- the only version pkg/scheduler's real call sites reach (grep
// across pkg/scheduler confirms `.Apps().V1()` only). Dropping
// V1beta1()/V1beta2() here (not just leaving them unreferenced) is what
// keeps informers/apps/v1beta1 and v1beta2 out of the schedwidth binary:
// their own group's concrete `V1beta1()`/`V1beta2()` method bodies, if
// present, would still compile and link even if no caller ever invokes
// them (interface method tables don't get dead-code-eliminated the way a
// genuinely unreferenced function would). See
// pkg/clientgo-lean-overlays/kubernetes/clientset_leanwidth.go's doc
// comment for the general mechanism this mirrors one level up
// (kubernetes.Interface itself).

package apps

import (
	v1 "k8s.io/client-go/informers/apps/v1"
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
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
