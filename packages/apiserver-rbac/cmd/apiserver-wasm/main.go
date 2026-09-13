//go:build js && wasm

package main

import (
	group "github.com/k8flare/k8flare/packages/apiserver-group"
	rbac "github.com/k8flare/k8flare/packages/apiserver-rbac"
)

func main() { group.Serve(rbac.GroupVersion) }
