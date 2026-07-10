//go:build schedwidth

/*
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
*/

// k8flare schedwidth overlay (see docs/platform-verification.md's S19/S21
// entries and pkg/clientgo-lean-overlays/kubernetes/clientset_leanwidth.go's
// doc comment for the prior `-tags leanwidth,schedwidth` attempt this
// supersedes): kubernetes.Interface narrowed for the `-tags schedwidth`
// scheduler wasm build, independent of `-tags leanwidth` (KCM's own
// narrow width -- the two are mutually exclusive, never combined).
//
// Unlike that prior attempt, this narrowing is paired with build-tagged
// per-group informers/<group>/interface_schedwidth.go overlays (apps,
// storage, resource, scheduling, policy) that also drop every sibling
// API version informers/factory.go's own accessors (Apps()/Storage()/
// Resource()/Scheduling()/Policy()) would otherwise force through each
// group's own interface.go -- the exact mechanism that made the prior
// attempt fail (`Apps` V1beta1/V1beta2, `Storage` V1alpha1/V1beta1,
// `Resource` V1alpha3/V1beta1, `Scheduling` V1/V1beta1, `Policy` V1beta1
// all required real methods here otherwise, since every
// informers/<group>/<version> package's own generated code hardcodes a
// `client kubernetes.Interface` parameter and calls the matching
// accessor unconditionally at informer-construction time, not gated by
// which version is actually watched at runtime).
//
// Method set here = exactly what's needed to compile, confirmed by grep
// across pkg/scheduler and pkg/controllers/sched, not guessed:
//   - CoreV1/AppsV1/StorageV1/ResourceV1/PolicyV1: informers/factory.go's
//     6 groups (Core/Apps/Storage/Resource/Scheduling/Policy) each
//     delegate to the matching informers/<group> package, which (now
//     narrowed the same way) only needs the primary version. CoreV1 is
//     also called directly by client-go/tools/events'
//     eventBroadcasterAdapterImpl.coreClient fallback.
//   - ResourceV1beta2: informers/resource's own (narrowed, still 2-wide)
//     interface_schedwidth.go additionally needs this for
//     scheduler.go's DeviceTaintRules construction (gated behind the
//     DRADeviceTaintRules feature at runtime, Beta/Default:false in
//     v1.36.2-k3s1 -- not gated at compile time, so the method must
//     exist regardless).
//   - SchedulingV1alpha2: informers/scheduling's own (narrowed to just
//     this version) interface_schedwidth.go needs it the same way.
//   - EventsV1: client-go/tools/events.NewEventBroadcasterAdapterWithContext
//     calls this unconditionally to build eventsv1Client/Broadcaster,
//     gated at runtime only by a Discovery().ServerResourcesForGroupVersion
//     check, not at compile time.
//   - Discovery: base Interface method, also read (not written) by the
//     same event-broadcaster-adapter call above.
//
// No BatchV1/CoordinationV1/DiscoveryV1: confirmed unused by pkg/scheduler
// and pkg/controllers/sched by grep (no leaderelection in this repo's
// RunScheduler, no direct BatchV1/DiscoveryV1 call site). Real
// implementations for CoreV1/AppsV1/StorageV1/ResourceV1/PolicyV1 come
// from pkg/leanclient/clientset's existing Clientset+SchedulerClientset
// (unchanged, already `!leanwidth`-tagged so it already applies here
// too); ResourceV1beta2/SchedulingV1alpha2/EventsV1/Discovery are
// permanent panic stubs added in pkg/leanclient/clientset/
// stubs_schedwidth.go, matching the same not-exercised-at-runtime
// reasoning `-tags leanwidth`'s SchedulingV1alpha2 stub
// (clientset_leanwidth.go) and this repo's existing Discovery() stub
// (stubs.go, unconditionally panics even in the current shipped
// full-width scheduler build -- a pre-existing, separately-tracked risk,
// not introduced or worsened here) already rely on.

package kubernetes

import (
	discovery "k8s.io/client-go/discovery"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	eventsv1 "k8s.io/client-go/kubernetes/typed/events/v1"
	policyv1 "k8s.io/client-go/kubernetes/typed/policy/v1"
	resourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
	resourcev1beta2 "k8s.io/client-go/kubernetes/typed/resource/v1beta2"
	schedulingv1alpha2 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha2"
	storagev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

// Interface is this build's kubernetes.Interface: only the methods
// something in the schedwidth binary's own call graph actually needs to
// compile against (see this file's doc comment for the accounting).
// Real implementation: pkg/leanclient/clientset.SchedulerClientset (its
// `!leanwidth`-tagged scheduler.go already applies here unchanged).
type Interface interface {
	Discovery() discovery.DiscoveryInterface
	CoreV1() corev1.CoreV1Interface
	AppsV1() appsv1.AppsV1Interface
	StorageV1() storagev1.StorageV1Interface
	ResourceV1() resourcev1.ResourceV1Interface
	ResourceV1beta2() resourcev1beta2.ResourceV1beta2Interface
	SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface
	PolicyV1() policyv1.PolicyV1Interface
	EventsV1() eventsv1.EventsV1Interface
}
