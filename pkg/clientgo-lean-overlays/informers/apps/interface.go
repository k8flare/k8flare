//go:build !schedwidth

/*
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
*/

// k8flare schedwidth overlay (see pkg/clientgo-lean-overlays/README.md
// and docs/platform-verification.md's S19/S21 entries): byte-identical
// to upstream's informers/apps/interface.go, just build-tagged so the
// `-tags schedwidth` scheduler build picks interface_schedwidth.go
// instead (V1 only -- the only version pkg/scheduler's own call sites
// reach, see that file's doc comment). Every OTHER build (plain,
// `-tags leanwidth`) is unaffected: this file's shape is identical to
// what gen-clientgo-lean-mirror.ts would otherwise copy straight from
// the upstream module.

package apps

import (
	v1 "k8s.io/client-go/informers/apps/v1"
	v1beta1 "k8s.io/client-go/informers/apps/v1beta1"
	v1beta2 "k8s.io/client-go/informers/apps/v1beta2"
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
)

// Interface provides access to each of this group's versions.
type Interface interface {
	// V1 provides access to shared informers for resources in V1.
	V1() v1.Interface
	// V1beta1 provides access to shared informers for resources in V1beta1.
	V1beta1() v1beta1.Interface
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

// V1beta1 returns a new v1beta1.Interface.
func (g *group) V1beta1() v1beta1.Interface {
	return v1beta1.New(g.factory, g.namespace, g.tweakListOptions)
}

// V1beta2 returns a new v1beta2.Interface.
func (g *group) V1beta2() v1beta2.Interface {
	return v1beta2.New(g.factory, g.namespace, g.tweakListOptions)
}
