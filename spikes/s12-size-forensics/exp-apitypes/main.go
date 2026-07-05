//go:build js && wasm

// Command exp-apitypes is throwaway forensics scaffolding for the S12 size
// investigation (see ../README.md). Isolates k8s.io/api/core/v1's own
// generated code (struct defs + DeepCopy + protobuf Marshal/Unmarshal) with
// no client-go/informer machinery at all, to separate "the API type graph
// itself is expensive" from "the typed Clientset wrapper is expensive".
package main

import (
	"fmt"

	"github.com/syumai/workers"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	pod := &corev1.Pod{}
	data, err := pod.Marshal()
	cp := pod.DeepCopy()
	fmt.Println(len(data), err, cp) // keep reachable
	workers.Ready()
}
