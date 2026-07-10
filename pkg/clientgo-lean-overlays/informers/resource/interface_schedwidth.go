//go:build schedwidth

/*
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
*/

// k8flare schedwidth overlay: informers/resource's own Interface
// narrowed to V1 and V1beta2 -- the only versions pkg/scheduler's real
// call sites reach (grep across pkg/scheduler confirms `.Resource().V1()`
// for the DRA construction block, unconditional since
// DynamicResourceAllocation is GA/LockToDefault:true, and
// `.Resource().V1beta2()` for scheduler.go's DeviceTaintRules block,
// gated behind the DRADeviceTaintRules feature but still needed to
// compile). V1alpha3/V1beta1 dropped. See pkg/clientgo-lean-overlays/
// informers/apps/interface_schedwidth.go's doc comment for the general
// mechanism.

package resource

import (
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	v1 "k8s.io/client-go/informers/resource/v1"
	v1beta2 "k8s.io/client-go/informers/resource/v1beta2"
)

// Interface provides access to this group's V1 and V1beta2, the only
// versions pkg/scheduler's own call sites reach.
type Interface interface {
	// V1 provides access to shared informers for resources in V1.
	V1() v1.Interface
	// V1beta2 provides access to shared informers for resources in V1beta2.
	V1beta2() v1beta2.Interface
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

// V1beta2 returns a new v1beta2.Interface.
func (g *group) V1beta2() v1beta2.Interface {
	return v1beta2.New(g.factory, g.namespace, g.tweakListOptions)
}
