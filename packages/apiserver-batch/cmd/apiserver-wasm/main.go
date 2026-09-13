//go:build js && wasm

package main

import (
	batch "github.com/k8flare/k8flare/packages/apiserver-batch"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(batch.GroupVersion) }
