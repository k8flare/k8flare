//go:build js && wasm

// Package leanclient replaces k8s.io/client-go/kubernetes's generated
// typed clients for workers/controllers' Go WASM build: real, unmodified
// upstream controller code (pkg/controller/*, pkg/scheduler) requires its
// client parameter to satisfy k8s.io/client-go/kubernetes.Interface's
// exact per-group/per-type interfaces (corev1.PodInterface and friends),
// but client-go's own generated implementations of those interfaces are
// unusable here: they cost ~45MiB of linked GOOS=js/wasm code regardless
// of how many API groups are registered in the runtime.Scheme they share
// (measured, not assumed -- see docs/platform-verification.md's Phase 10
// forensics). The cost is not in the types themselves (~11-16MiB per
// group, unavoidable since upstream controllers use the real
// k8s.io/api/<group>/<version> structs) or in client-go/rest's transport
// machinery (~2MiB) -- it's specifically in routing every object through
// runtime.Scheme + serializer.CodecFactory's generic, reflection-based
// encode/decode machinery, which pulls in code proportional to *what's
// registered in the scheme*, not to what this repo's controllers actually
// call.
//
// This package's verb helpers (verbs.go) never register any type into a
// runtime.Scheme and never route an object through a
// serializer.CodecFactory: they use rest.Request.DoRaw/.Stream (client-go
// primitives that hand back undecoded bytes) and plain encoding/json
// against the caller's own concrete k8s.io/api type. rest.RESTClientFor
// still needs *some* NegotiatedSerializer to construct (RESTClientFor),
// so RESTClientFor below builds one from a deliberately empty Scheme --
// its content negotiation machinery is exercised for content-type
// selection only, never for decoding a real object.
//
// The interfaces this package's generated implementations (cmd/k8flare-gen's
// leanclient step, pkg/leanclient/gen/) satisfy also require Apply/
// ApplyStatus methods (k8s.io/client-go/kubernetes.Interface's exact
// method set, even though this repo's controllers never call Server-Side
// Apply -- confirmed by grep, zero hits). Referencing their parameter type
// (k8s.io/client-go/applyconfigurations/<group>/<version>.XApplyConfiguration)
// is unavoidable to satisfy the interface, but importing that type from
// client-go's own package costs ~40MiB *regardless of the type's own
// complexity* -- isolated (not assumed) to sibling files in the same
// package being linked despite never being referenced: deleting ~200
// unrelated sibling files (Service/ConfigMap/Secret/... apply-configs)
// from a local copy of just k8s.io/client-go/applyconfigurations/core/v1
// dropped an identical stub from 44.25MiB to 2.04MiB with zero code
// changes elsewhere. third_party/clientgo-lean-overlays/ is the
// consequence: a second local module mirror (alongside
// third_party/k8s-js-overlays/ for k8s.io/kubernetes) pruning
// k8s.io/client-go's kubernetes/typed/<group>/<version> and
// applyconfigurations/<group>/<version> packages down to interface-only
// declarations (typed) and empty structs (applyconfigurations) for
// exactly the types this repo's controllers use -- see that directory's
// README.md for the full mechanism and why go build -overlay can't do
// this (GOMODCACHE restriction, same reason third_party/k8s-js-overlays/
// exists).
package leanclient

import (
	restclient "k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// RESTClientFor builds a *rest.RESTClient for group/version at apiPath
// ("/api" for the legacy core group, "/apis" for every other group),
// sharing cfg's Transport (in production, the Cloudflare-service-binding-
// backed RoundTripper from pkg/controllers/restconfig.go's RestConfig).
// The returned client's NegotiatedSerializer is backed by a Scheme with
// nothing registered in it -- see this package's doc comment for why that
// is safe: nothing built on top of this client ever asks the codec to
// decode a real object.
func RESTClientFor(cfg *restclient.Config, apiPath string, gv schema.GroupVersion) (*restclient.RESTClient, error) {
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)

	c := *cfg
	c.APIPath = apiPath
	c.GroupVersion = &gv
	c.NegotiatedSerializer = restclient.CodecFactoryForGeneratedClient(scheme, codecs).WithoutConversion()
	return restclient.RESTClientFor(&c)
}
