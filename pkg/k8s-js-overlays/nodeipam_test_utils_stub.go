// Hand-curated: replaces upstream
// pkg/controller/nodeipam/ipam/test/utils.go (a non-_test.go file, so
// it's part of the package's normal build) with an empty stub. Its
// exports are test-only fixtures for *other* packages' _test.go files --
// confirmed unused by any non-test code in pkg/controller/nodeipam
// (grep). The original imports both k8s.io/client-go/kubernetes/fake and
// the aggregate k8s.io/client-go/informers (for a throwaway
// fake-clientset-backed informer factory used only in tests), the latter
// of which pulls in *all* ~54 client-go groups' typed clients and
// applyconfigurations transitively -- exactly the weight
// pkg/clientgo-lean-overlays exists to avoid. Since this file is
// dead code for RunControllerManager's actual path, stubbing it here is
// simpler than mirroring/pruning client-go's fake and aggregate informers
// packages just to keep an unused test fixture compiling.
package test
