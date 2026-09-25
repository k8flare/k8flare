//go:build js && wasm

package main

import (
	"github.com/k8flare/k8flare/packages/printers"
	"github.com/k8flare/k8flare/packages/printers-autoscaling/tables"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	_ "k8s.io/kubernetes/pkg/apis/autoscaling/install"
)

func main() {
	bridge.Serve(printers.Handler(tables.AddHandlers))
}
