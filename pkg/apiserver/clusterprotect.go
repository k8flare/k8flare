package apiserver

import (
	"errors"
	"strings"

	k8flarev1alpha1 "github.com/k8flare/k8flare/pkg/apis/k8flare/v1alpha1"
	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// protectedClusterName is the one Cluster object that must never be
// deleted: "default" is the management cluster itself -- the cluster whose
// storage holds every other Cluster object, whose token vault issues their
// tokens, and (once the cluster-operator lands, P2 of
// docs/cluster-api-design.md) the cluster the operator runs against.
// Deleting it would tear down the control plane that owns the teardown.
const protectedClusterName = "default"

// clusterAPIPrefix is the URL prefix requests for k8flare.com/v1alpha1
// arrive under. Derived from the same apidef machinery that registers the
// route (server.go), so it cannot drift from it.
var clusterAPIPrefix = apidef.APIPrefix(k8flarev1alpha1.SchemeGroupVersion)

// IsProtectedClusterDelete reports whether a DELETE for resource/name
// served under prefix targets the protected "default" Cluster. Callers
// reject with 403 Forbidden.
//
// This is an admission-style check in the handler, not a registry strategy
// hook, for the same reason namespace-lifecycle admission is (handler.go's
// POST case): upstream's rest.RESTDeleteStrategy has no validate-on-delete
// hook to hang it off -- BeforeDelete only mutates deletion options -- so
// the generic delete path offers nowhere else to say "no".
//
// prefix is matched rather than a GroupVersion value because HandleResource
// is routed per-GroupVersion by URL prefix and never receives the
// GroupVersion itself.
//
// A collection delete (DELETE .../clusters with no name) takes the other
// path: it cannot 403 the whole request without making "delete every tenant
// cluster" impossible, so ProtectedClusterCollectionKeep holds "default"
// back and lets the rest through.
func IsProtectedClusterDelete(prefix, resource, name string) bool {
	return isClusterResource(prefix, resource) && name == protectedClusterName
}

// ProtectedClusterCollectionKeep returns the object name a collection
// delete of resource under prefix must NOT delete ("" when nothing is
// protected). Closing P1's known gap: with the cluster-operator in place a
// Cluster delete tears down real infrastructure, so DELETE .../clusters
// reaching "default" would destroy the control plane that owns every other
// cluster's teardown.
func ProtectedClusterCollectionKeep(prefix, resource string) string {
	if isClusterResource(prefix, resource) {
		return protectedClusterName
	}
	return ""
}

func isClusterResource(prefix, resource string) bool {
	return strings.TrimSuffix(prefix, "/") == strings.TrimSuffix(clusterAPIPrefix, "/") &&
		resource == "clusters"
}

// isProtectedClusterResource is isClusterResource by group rather than by URL
// prefix, for callers that have parsed request info rather than a raw path.
func isProtectedClusterResource(group, resource string) bool {
	return group == k8flarev1alpha1.GroupName && resource == "clusters"
}

var errManagementClusterUndeletable = errors.New(
	"is the management cluster and cannot be deleted")
