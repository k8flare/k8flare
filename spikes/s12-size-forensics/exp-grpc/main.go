//go:build js && wasm

// Command exp-grpc is throwaway forensics scaffolding for the S12 size
// investigation (see ../README.md). google.golang.org/grpc showed up in
// both scheduler's and KCM's pure import graphs (intersection) at ~452KiB
// pre-link nm-sum despite neither RunScheduler nor RunControllerManager
// ever making a gRPC call themselves -- this checks whether the real
// linker actually keeps grpc's code (something transitively pulls it in
// and calls into it) or dead-code-eliminates it (it is merely importable,
// never reachable).
package main

import (
	"fmt"

	"github.com/syumai/workers"
	_ "google.golang.org/grpc"
)

func main() {
	fmt.Println("grpc blank-imported only, nothing called")
	workers.Ready()
}
