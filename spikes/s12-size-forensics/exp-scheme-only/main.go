//go:build js && wasm

// exp-scheme-only isolates the cost of runtime.Scheme + AddToScheme(core/v1)
// + serializer.NewCodecFactory alone, with NO rest.RESTClientFor call and
// no generated typed client wrapping -- to find out whether the ~35MiB
// gap between "core/v1 types only" (15.55 MiB) and "core/v1 types + a
// working 1-group client" (50.65 MiB, exp-leanscheme-1group) lives in the
// scheme/codec/content-negotiation layer or in client-go's REST client
// construction layer (see exp-restclient-only, the complementary probe).
package main

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

func main() {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	codecs := serializer.NewCodecFactory(scheme)
	_ = codecs
	println("built")
}
