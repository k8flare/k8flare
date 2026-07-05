//go:build js && wasm

// exp-restclient-only is the complementary probe to exp-scheme-only: an
// empty runtime.Scheme (zero AddToScheme calls -- no core/v1, nothing) fed
// into rest.RESTClientFor, isolating client-go's REST client construction
// machinery (transport/auth/TLS/backoff) from any API type registration
// at all.
package main

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

func main() {
	scheme := runtime.NewScheme() // deliberately empty: no AddToScheme calls
	codecs := serializer.NewCodecFactory(scheme)
	negotiated := rest.CodecFactoryForGeneratedClient(scheme, codecs).WithoutConversion()

	gv := schema.GroupVersion{Group: "", Version: "v1"}
	cfg := &rest.Config{Host: "https://example.invalid", APIPath: "/api"}
	cfg.GroupVersion = &gv
	cfg.NegotiatedSerializer = negotiated
	rc, err := rest.RESTClientFor(cfg)
	if err != nil {
		panic(err)
	}
	_ = rc
	println("built")
}
