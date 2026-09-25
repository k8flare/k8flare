//go:build js && wasm

package main

import (
	flowcontrol "github.com/k8flare/k8flare/packages/apiserver-flowcontrol"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(flowcontrol.GroupVersion) }
