//go:build js && wasm

// Command exp-narrowclient is throwaway forensics scaffolding for the S12
// size investigation (see ../README.md). Bypasses the aggregate
// k8s.io/client-go/kubernetes.Clientset entirely and constructs only the
// single typed/core/v1 client directly, mirroring the "leanclient"
// experiment docs/platform-verification.md already ran for gzip size --
// this repeats it for raw size, to see whether avoiding the
// all-~54-groups aggregate Clientset helps more or less once measured
// without gzip smoothing things over.
package main

import (
	"fmt"

	"github.com/syumai/workers"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
)

func main() {
	cs, err := corev1client.NewForConfig(&restclient.Config{Host: "http://127.0.0.1:1"})
	fmt.Println(cs, err)
	workers.Ready()
}
