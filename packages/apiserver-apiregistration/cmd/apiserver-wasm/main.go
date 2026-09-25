//go:build js && wasm

package main

import (
	apiregistration "github.com/k8flare/k8flare/packages/apiserver-apiregistration"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(apiregistration.GroupVersion) }
