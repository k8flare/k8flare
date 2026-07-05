//go:build js && wasm

// Command kcm-imports is throwaway forensics scaffolding for the S12 size
// investigation (see ../README.md). Mirror of ../sched-imports: a package
// graph rooted at exactly the imports pkg/controllers/controllermanager.go
// itself declares, without scheduler.go's imports riding along. See
// ../sched-imports/main.go's doc comment for why this is needed at all.
package main

import (
	_ "k8s.io/client-go/informers"
	_ "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/rest"
	_ "k8s.io/client-go/util/flowcontrol"
	_ "k8s.io/kubernetes/pkg/controller/cronjob"
	_ "k8s.io/kubernetes/pkg/controller/daemon"
	_ "k8s.io/kubernetes/pkg/controller/deployment"
	_ "k8s.io/kubernetes/pkg/controller/endpoint"
	_ "k8s.io/kubernetes/pkg/controller/endpointslice"
	_ "k8s.io/kubernetes/pkg/controller/job"
	_ "k8s.io/kubernetes/pkg/controller/nodeipam"
	_ "k8s.io/kubernetes/pkg/controller/nodeipam/ipam"
	_ "k8s.io/kubernetes/pkg/controller/nodelifecycle"
	_ "k8s.io/kubernetes/pkg/controller/replicaset"
	_ "k8s.io/kubernetes/pkg/controller/tainteviction"
)

func main() {}
