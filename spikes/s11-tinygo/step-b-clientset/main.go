//go:build js && wasm

// Command step-b-clientset is throwaway TinyGo-viability scaffolding for
// the S11 spike -- see ../README.md. Not a real Worker entrypoint.
//
// Deliberately does NOT import github.com/syumai/workers: step-a already
// showed net/http itself fails to compile under TinyGo's wasm target
// (roundtrip_js.go references an undefined Transport.roundTrip), and
// syumai/workers' handler_js.go imports net/http directly, so any build
// that pulls in syumai/workers inherits that failure regardless of what
// else is in the program. This file isolates the OTHER question the
// spike needs answered independently: does k8s.io/client-go's generated
// typed Clientset itself compile under TinyGo, ignoring the Workers
// runtime glue entirely.
package main

import (
	"fmt"

	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

func main() {
	cfg := &restclient.Config{Host: "https://example.invalid"}
	c, err := clientset.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	fmt.Println(c)
}
