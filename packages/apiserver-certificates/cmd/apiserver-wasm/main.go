//go:build js && wasm

package main

import (
	certificates "github.com/k8flare/k8flare/packages/apiserver-certificates"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(certificates.GroupVersion) }
