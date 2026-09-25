//go:build js && wasm

package main

import (
	group "github.com/k8flare/k8flare/packages/apiserver-group"
	networking "github.com/k8flare/k8flare/packages/apiserver-networking"
)

func main() { group.Serve(networking.GroupVersion) }
