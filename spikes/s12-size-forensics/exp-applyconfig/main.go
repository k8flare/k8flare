//go:build js && wasm

// Command exp-applyconfig is throwaway forensics scaffolding for the S12
// size investigation (see ../README.md). Isolates
// k8s.io/client-go/applyconfigurations/core/v1's generated builder type,
// with no aggregate Clientset involved, to see whether the
// applyconfigurations tree's cost is really tied to the Clientset or
// stands on its own.
package main

import (
	"fmt"

	"github.com/syumai/workers"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
)

func main() {
	cfg := applycorev1.Pod("name", "namespace")
	fmt.Println(cfg)
	workers.Ready()
}
