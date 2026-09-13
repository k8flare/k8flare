//go:build js && wasm

package main

import (
	group "github.com/k8flare/k8flare/packages/apiserver-group"
	storage "github.com/k8flare/k8flare/packages/apiserver-storage"
)

func main() { group.Serve(storage.GroupVersion) }
