package apiserver

import (
	"context"
	"fmt"
)

// DeleteNamespaceDependents deletes every object across every namespaced
// resource type within the given namespace. It does NOT delete the Namespace
// object itself — callers must do that separately, and only after this
// succeeds (see handler.go's namespaces DELETE case).
//
// Scope: this implements namespace-triggered cascading deletion only, not
// general owner-reference garbage collection (no workload types like
// ReplicaSet exist yet that would need that), and does not implement a
// Terminating phase or finalizers — a synchronous sweep-then-delete within
// the single DELETE request is an acceptable simplification at this
// project's current scale: if the sweep fails partway, the Namespace object
// is still there, so a client retry of the same DELETE is the correct
// recovery path (every step is idempotent).
//
// Known gap: this only covers resource types registered in the Go
// apiserver's ResourceStore maps. Namespaced CRDs (DynamicWorker,
// WorkerTrigger) live entirely in the TypeScript packages/crd +
// packages/worker storage layer and are NOT covered here.
func DeleteNamespaceDependents(ctx context.Context, namespacedStores []*ResourceStore, namespace string) error {
	for _, rs := range namespacedStores {
		if _, err := rs.DeleteAllInNamespace(ctx, namespace); err != nil {
			return fmt.Errorf("sweep %s in namespace %q: %w", rs.resource, namespace, err)
		}
	}
	return nil
}
