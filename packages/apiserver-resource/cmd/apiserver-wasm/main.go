//go:build js && wasm

package main

import (
	group "github.com/k8flare/k8flare/packages/apiserver-group"
	resource "github.com/k8flare/k8flare/packages/apiserver-resource"
)

func main() { group.Serve(resource.GroupVersion) }
