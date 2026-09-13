//go:build js && wasm

package main

import (
	discovery "github.com/k8flare/k8flare/packages/apiserver-discovery"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(discovery.GroupVersion) }
