//go:build js && wasm

package main

import (
	authentication "github.com/k8flare/k8flare/packages/apiserver-authentication"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() { group.Serve(authentication.GroupVersion) }
