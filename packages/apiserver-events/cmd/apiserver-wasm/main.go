//go:build js && wasm

package main

import (
	events "github.com/k8flare/k8flare/packages/apiserver-events"
	"github.com/k8flare/k8flare/packages/apiserver-events/storageconv"
	group "github.com/k8flare/k8flare/packages/apiserver-group"
)

func main() {
	storageconv.Install()
	group.Serve(events.GroupVersion)
}
