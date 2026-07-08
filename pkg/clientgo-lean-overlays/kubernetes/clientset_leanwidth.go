//go:build leanwidth

/*
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
*/

// k8flare leanwidth overlay (see pkg/clientgo-lean-overlays/
// README.md and docs/platform-verification.md's OPEN REGRESSION entry):
// kubernetes.Interface narrowed to the groups this repo's controllers
// actually use, plus SchedulingV1alpha2 (the 1.36 job controller imports
// its PodGroup informer package unconditionally). Merely referencing a
// typed group package links that group's full generated API surface via
// proto-registration init()s, so interface WIDTH -- not implementation
// -- is what decides the wasm binary size: full width measured 105MB
// raw for the KCM binary, this narrow width 78MB (2026-07-05). Only the
// `-tags leanwidth` KCM build against go.wasm.mod ever sees this file;
// the scheduler binary and all host builds use the full-width variant.
//
// A `-tags leanwidth,schedwidth` variant (widening this Interface with
// Storage/Resource/Policy for the scheduler) was attempted and reverted
// 2026-07-07: kubernetes.Interface is one global type, so widening it
// with real methods for those 3 groups made every OTHER, still-unpruned
// sibling API version (Apps V1beta1/V1beta2, Storage V1alpha1/V1beta1,
// Resource V1alpha3/V1beta1, Scheduling V1/V1beta1, Policy V1beta1,
// plus a newly-discovered EventsV1 requirement from
// client-go/tools/events) fail to compile too -- each of those
// informers/<group>/<version> packages' own NewFilteredXInformer
// hardcodes a `client kubernetes.Interface` parameter, so ANY narrowing
// of the type requires pruning literally every sibling version
// transitively reachable, not just the ones this repo calls. See
// docs/platform-verification.md's S8 kube-scheduler-wasm-fork entry for
// the full account; this is exactly the "Phase 10" finding this file's
// own doc comment already predicted, now re-confirmed by attempting it.

package kubernetes

import (
	discovery "k8s.io/client-go/discovery"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	batchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1 "k8s.io/client-go/kubernetes/typed/discovery/v1"
	schedulingv1alpha2 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha2"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	AppsV1() appsv1.AppsV1Interface
	BatchV1() batchv1.BatchV1Interface
	CoordinationV1() coordinationv1.CoordinationV1Interface
	CoreV1() corev1.CoreV1Interface
	DiscoveryV1() discoveryv1.DiscoveryV1Interface
	SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface
}

type Clientset struct {
	*discovery.DiscoveryClient
	appsV1         *appsv1.AppsV1Client
	batchV1        *batchv1.BatchV1Client
	coordinationV1 *coordinationv1.CoordinationV1Client
	coreV1         *corev1.CoreV1Client
	discoveryV1    *discoveryv1.DiscoveryV1Client
}

func (c *Clientset) AppsV1() appsv1.AppsV1Interface                         { return c.appsV1 }
func (c *Clientset) BatchV1() batchv1.BatchV1Interface                      { return c.batchV1 }
func (c *Clientset) CoordinationV1() coordinationv1.CoordinationV1Interface { return c.coordinationV1 }
func (c *Clientset) CoreV1() corev1.CoreV1Interface                         { return c.coreV1 }
func (c *Clientset) DiscoveryV1() discoveryv1.DiscoveryV1Interface          { return c.discoveryV1 }

func (c *Clientset) SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface {
	panic("leanwidth: SchedulingV1alpha2 not implemented")
}

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.UserAgent == "" {
		configShallowCopy.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	httpClient, err := rest.HTTPClientFor(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	var cs Clientset
	if cs.appsV1, err = appsv1.NewForConfigAndClient(&configShallowCopy, httpClient); err != nil {
		return nil, err
	}
	if cs.batchV1, err = batchv1.NewForConfigAndClient(&configShallowCopy, httpClient); err != nil {
		return nil, err
	}
	if cs.coordinationV1, err = coordinationv1.NewForConfigAndClient(&configShallowCopy, httpClient); err != nil {
		return nil, err
	}
	if cs.coreV1, err = corev1.NewForConfigAndClient(&configShallowCopy, httpClient); err != nil {
		return nil, err
	}
	if cs.discoveryV1, err = discoveryv1.NewForConfigAndClient(&configShallowCopy, httpClient); err != nil {
		return nil, err
	}
	if cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfigAndClient(&configShallowCopy, httpClient); err != nil {
		return nil, err
	}
	return &cs, nil
}

func NewForConfigOrDie(c *rest.Config) *Clientset {
	cs, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return cs
}
