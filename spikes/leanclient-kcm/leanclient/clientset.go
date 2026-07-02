//go:build js && wasm

// Package leanclient is a narrow stand-in for k8s.io/client-go/kubernetes's
// generated Clientset + k8s.io/client-go/informers' SharedInformerFactory,
// built to satisfy exactly what workers/controllers' five enabled
// controllers (nodeipam, nodelifecycle, taint-eviction-controller,
// endpoint, endpointslice) actually call, after gzip-size measurements
// (docs/platform-verification.md's Phase 5 section) showed the real
// generated Clientset+informers alone cost 8.87 MiB gzip -- 89% of
// Cloudflare Workers' 10 MiB deploy limit -- before any controller logic.
//
// Root cause, found by isolating each layer with real GOOS=js/wasm builds
// (not assumed -- CLAUDE.md rule 2): every one of kubernetes.Interface's
// ~54 group accessors (CoreV1, AppsV1, BatchV1, ...) is required by the
// interface's exact method set, because upstream controller constructors
// and helpers (e.g. k8s.io/component-helpers/node/util.PatchNodeCIDRs,
// called by nodeipam) hardcode their client parameter's type as the
// literal k8s.io/client-go/kubernetes.Interface -- not a smaller,
// structurally-similar interface. A type can only be assigned there if it
// implements all ~54 methods, so there is no way to avoid declaring them
// all; the size win is in what the *other* ~50 (everything this repo's
// enabled controllers never call) are allowed to cost: nothing, because
// they never construct a real concrete client (see stubs.go) and are
// therefore dead code the linker drops entirely (confirmed empirically:
// a probe binary satisfying kubernetes.Interface with only CoreV1 real
// and the other 53 accessors panic-stubbed measured the same size as one
// using only CoreV1 directly, with no aggregate Clientset in the binary
// at all).
//
// What does NOT reduce size, also measured rather than assumed: swapping
// the four real groups' own NewForConfig (which each independently
// reference the shared, all-54-groups k8s.io/client-go/kubernetes/scheme
// package for content-negotiation setup) for a hand-built narrow
// *runtime.Scheme registering only these four groups made no measurable
// difference (9.591 MiB vs 9.593 MiB gzip for CoreV1 alone, either way).
// The dominant cost is CoreV1 itself (mostly Pod's own large type graph)
// plus client-go/rest's transport/serializer machinery, not how many
// *other* groups happen to be registered in the scheme every typed client
// already shares. So this package uses each real group's plain,
// already-published NewForConfig -- the simplest option, per CLAUDE.md's
// "手書きコードは最小限に" -- rather than a hand-rolled scheme/codec/REST-client
// bypass that would add real hand-written code for no measured size
// benefit.
//
// Every real accessor below (CoreV1, DiscoveryV1, CoordinationV1, AppsV1)
// returns the exact, unmodified upstream generated typed client
// (k8s.io/client-go/kubernetes/typed/<group>/<version>) -- genuine reuse,
// not a reimplementation of REST/CRUD logic (CLAUDE.md inviolable rule
// 3). The only hand-written code in this package is this struct's
// wiring and the ~50 mechanical panic stubs in stubs.go.
package leanclient

import (
	kubernetes "k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1 "k8s.io/client-go/kubernetes/typed/discovery/v1"
	restclient "k8s.io/client-go/rest"
)

// Clientset implements kubernetes.Interface. Only the four accessors
// workers/controllers' enabled controllers call (see this package's doc
// comment) are backed by a real, working client; every other method
// (stubs.go) panics if ever called, so a controller that starts quietly
// depending on an unaudited group fails loudly in testing instead of
// silently no-op'ing.
type Clientset struct {
	core  *corev1.CoreV1Client
	disc  *discoveryv1.DiscoveryV1Client
	coord *coordinationv1.CoordinationV1Client
	apps  *appsv1.AppsV1Client
}

var _ kubernetes.Interface = (*Clientset)(nil)

// NewForConfig builds a Clientset whose four real group clients all share
// cfg's Transport -- the Cloudflare-service-binding-backed RoundTripper
// from restconfig.go's RestConfig, in production use.
func NewForConfig(cfg *restclient.Config) (*Clientset, error) {
	core, err := corev1.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	disc, err := discoveryv1.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	coord, err := coordinationv1.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	apps, err := appsv1.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Clientset{core: core, disc: disc, coord: coord, apps: apps}, nil
}

func (c *Clientset) CoreV1() corev1.CoreV1Interface { return c.core }

func (c *Clientset) DiscoveryV1() discoveryv1.DiscoveryV1Interface { return c.disc }

func (c *Clientset) CoordinationV1() coordinationv1.CoordinationV1Interface { return c.coord }

func (c *Clientset) AppsV1() appsv1.AppsV1Interface { return c.apps }
