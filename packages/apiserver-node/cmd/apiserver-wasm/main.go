//go:build js && wasm

package main

import (
	group "github.com/k8flare/k8flare/packages/apiserver-group"
	node "github.com/k8flare/k8flare/packages/apiserver-node"
)

func main() { group.Serve(node.GroupVersion) }
