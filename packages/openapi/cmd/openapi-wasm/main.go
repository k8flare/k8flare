//go:build js && wasm

package main

import (
	"github.com/k8flare/k8flare/packages/openapi"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func main() {
	handler, err := openapi.Handler()
	if err != nil {
		panic(err)
	}
	bridge.Serve(handler)
}
