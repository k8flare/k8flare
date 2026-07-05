//go:build js && wasm

// exp-leanscheme-1group isolates whether exp-leanscheme's surprising
// result (5-group own-scheme client landed at ~50.8 MiB, essentially
// identical to both the 54-group aggregate Clientset at 50.71 MiB and the
// shared-scheme single-group client at 50.50 MiB) means "5 groups is
// already too many" or "the cost is fixed machinery independent of scheme
// scope entirely". Registers only core/v1 -- if this is ALSO ~50 MiB, the
// scheme-scope hypothesis is dead regardless of implementation, and the
// fixed cost lives in the generic runtime.Scheme / serializer.CodecFactory
// / rest.RESTClient machinery itself, not in how many types are registered.
package main

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func main() {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	codecs := serializer.NewCodecFactory(scheme)
	negotiated := rest.CodecFactoryForGeneratedClient(scheme, codecs).WithoutConversion()

	coreGV := corev1.SchemeGroupVersion
	coreCfg := &rest.Config{Host: "https://example.invalid", APIPath: "/api"}
	coreCfg.GroupVersion = &coreGV
	coreCfg.NegotiatedSerializer = negotiated
	coreRC, err := rest.RESTClientFor(coreCfg)
	if err != nil {
		panic(err)
	}
	core := corev1client.New(coreRC)
	_ = core.Pods("default")
	_ = core.Nodes()

	println("built")
}
