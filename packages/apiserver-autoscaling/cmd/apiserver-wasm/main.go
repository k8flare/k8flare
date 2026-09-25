//go:build js && wasm

package main

import (
	autoscaling "github.com/k8flare/k8flare/packages/apiserver-autoscaling"
	"github.com/k8flare/k8flare/packages/apiserver-autoscaling/storageconv"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() {
	storageconv.Install()
	group.Serve(autoscaling.GroupVersion)
}
