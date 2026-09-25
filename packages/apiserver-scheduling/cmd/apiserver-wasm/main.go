//go:build js && wasm

package main

import (
	group "github.com/k8flare/k8flare/packages/apiserver-group"
	scheduling "github.com/k8flare/k8flare/packages/apiserver-scheduling"
)

func main() { group.Serve(scheduling.GroupVersion) }
