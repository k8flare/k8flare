//go:build js && wasm

package main

import nodetunnel "github.com/k8flare/k8flare/packages/node-tunnel"

func main() {
	nodetunnel.Serve()
}
