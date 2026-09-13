//go:build js && wasm

package main

import (
	core "github.com/k8flare/k8flare/packages/apiserver-core"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(core.GroupVersion) }
