// Hand-curated: replaces upstream pkg/scheduler/backend/queue/testing.go
// (a non-_test.go file, so it's part of the package's normal build, not
// excluded from `go build`) with an empty stub. Its three exports
// (NewTestQueue/NewTestQueueWithObjects/NewTestQueueWithInformerFactory)
// are test-only helpers for *other* packages' _test.go files -- confirmed
// unused by any non-test code anywhere in pkg/scheduler (grep). The
// original imports k8s.io/client-go/kubernetes/fake, whose
// typed/<group>/<version>/fake subpackages third_party/clientgo-lean-
// overlays' pruning removes for the five groups this repo's Clientset
// implements (see that directory's README.md) -- since this file is dead
// code for RunScheduler's actual path, stubbing it out here is simpler
// than also mirroring/pruning client-go's fake package tree just to keep
// a test helper nothing calls compiling.
package queue
