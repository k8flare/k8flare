//go:build js && wasm

package main

import (
	group "github.com/k8flare/k8flare/packages/apiserver-group"
	policy "github.com/k8flare/k8flare/packages/apiserver-policy"
)

func main() { group.Serve(policy.GroupVersion) }
