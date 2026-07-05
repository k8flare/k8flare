//go:build js && wasm

// Command exp-clientset is throwaway forensics scaffolding for the S12 size
// investigation (see ../README.md). Isolates k8s.io/client-go/kubernetes's
// aggregate typed Clientset (NewForConfig constructs all ~54 per-group
// sub-clients unconditionally -- see kubernetes/clientset.go -- so a real
// *call*, not just a blank import, is required for the linker to actually
// retain that constructor's body and everything it reaches).
package main

import (
	"fmt"

	"github.com/syumai/workers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

func main() {
	cs, err := kubernetes.NewForConfig(&restclient.Config{Host: "http://127.0.0.1:1"})
	fmt.Println(cs, err) // keep NewForConfig's result reachable so the call itself can't be dead-code-eliminated
	workers.Ready()
}
