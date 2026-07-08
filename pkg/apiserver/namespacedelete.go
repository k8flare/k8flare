package apiserver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
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
// apiserver's ResourceStore maps -- a future generic CRD mechanism
// (apiextensions.k8s.io/v1 CustomResourceDefinition) would need its own
// cascade-delete coverage.
func DeleteNamespaceDependents(ctx context.Context, namespacedStores []*ResourceStore, namespace string) error {
	for _, rs := range namespacedStores {
		// ResourceStore.DeleteAllInNamespace works on raw stored bytes (by
		// design: it's resource-type-agnostic, no decode needed for a plain
		// bulk delete) -- which means it never runs ReleaseClusterIP the way
		// the single/collection Service DELETE paths in handler.go do.
		// Release explicitly here, first, for the one resource type that
		// needs it, so `kubectl delete namespace` doesn't leak every
		// ClusterIP that was allocated to a Service inside it (found by
		// review; verified via TestNamespaceDeleteReleasesServiceClusterIPs).
		if rs.resource == "services" {
			if err := releaseNamespaceServiceClusterIPs(ctx, rs, namespace); err != nil {
				return fmt.Errorf("release ClusterIPs for services in namespace %q: %w", namespace, err)
			}
		}
		if _, err := rs.DeleteAllInNamespace(ctx, namespace); err != nil {
			return fmt.Errorf("sweep %s in namespace %q: %w", rs.resource, namespace, err)
		}
	}
	return nil
}

// releaseNamespaceServiceClusterIPs releases the ClusterIP of every Service
// in namespace back to the pool, best-effort (see ReleaseClusterIP).
func releaseNamespaceServiceClusterIPs(ctx context.Context, rs *ResourceStore, namespace string) error {
	listObj, err := rs.List(ctx, namespace, "", "")
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	svcList, ok := listObj.(*corev1.ServiceList)
	if !ok {
		return fmt.Errorf("services store returned %T, expected *corev1.ServiceList", listObj)
	}
	for i := range svcList.Items {
		ReleaseClusterIP(ctx, rs.storage, &svcList.Items[i])
	}
	return nil
}
