//go:build js && wasm

package main

import (
	coordination "github.com/k8flare/k8flare/packages/apiserver-coordination"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(coordination.GroupVersion) }
