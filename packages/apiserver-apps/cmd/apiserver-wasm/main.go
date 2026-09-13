//go:build js && wasm

package main

import (
	apps "github.com/k8flare/k8flare/packages/apiserver-apps"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(apps.GroupVersion) }
