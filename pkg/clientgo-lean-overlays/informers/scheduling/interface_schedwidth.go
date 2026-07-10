//go:build schedwidth

/*
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
*/

// k8flare schedwidth overlay: informers/scheduling's own Interface
// narrowed to V1alpha2 -- the only version pkg/scheduler's real call
// sites reach (grep across pkg/scheduler confirms `.Scheduling().
// V1alpha2()` only -- PodGroup informers, the same version this repo's
// leanwidth KCM Interface already carries for the 1.36 job controller).
// See pkg/clientgo-lean-overlays/informers/apps/interface_schedwidth.go's
// doc comment for the general mechanism.

package scheduling

import (
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	v1alpha2 "k8s.io/client-go/informers/scheduling/v1alpha2"
)

// Interface provides access to this group's V1alpha2, the only version
// pkg/scheduler's own call sites reach.
type Interface interface {
	// V1alpha2 provides access to shared informers for resources in V1alpha2.
	V1alpha2() v1alpha2.Interface
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

// V1alpha2 returns a new v1alpha2.Interface.
func (g *group) V1alpha2() v1alpha2.Interface {
	return v1alpha2.New(g.factory, g.namespace, g.tweakListOptions)
}
