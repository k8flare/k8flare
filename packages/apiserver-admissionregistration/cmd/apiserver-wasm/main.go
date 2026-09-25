//go:build js && wasm

package main

import (
	admissionregistration "github.com/k8flare/k8flare/packages/apiserver-admissionregistration"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(admissionregistration.GroupVersion) }
