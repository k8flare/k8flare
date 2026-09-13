//go:build js && wasm

package main

import (
	authorization "github.com/k8flare/k8flare/packages/apiserver-authorization"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(authorization.GroupVersion) }
