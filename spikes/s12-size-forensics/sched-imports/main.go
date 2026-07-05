//go:build js && wasm

// Command sched-imports is throwaway forensics scaffolding for the S12 size
// investigation (see ../README.md). It exists only so `go list -deps` can
// see a package graph rooted at exactly the imports
// pkg/controllers/scheduler.go itself declares, without pulling in
// controllermanager.go's imports too -- which is unavoidable if you `go
// list -deps` anything that imports the real github.com/k8flare/k8flare/pkg/controllers
// package, since Go resolves imports per-package (both files share one
// `package controllers`), not per-function. Blank imports are enough:
// `go list -deps` walks the import graph, it does not care whether any
// symbol is referenced.
package main

import (
	_ "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/rest"
	_ "k8s.io/client-go/tools/events"
	_ "k8s.io/kubernetes/pkg/scheduler"
	_ "k8s.io/kubernetes/pkg/scheduler/apis/config"
	_ "k8s.io/kubernetes/pkg/scheduler/apis/config/latest"
)

func main() {}
