package apiserver

import (
	"errors"

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

// isProtectedClusterResource is isClusterResource by group rather than by URL
// prefix, for callers that have parsed request info rather than a raw path.
func isProtectedClusterResource(group, resource string) bool {
	return group == k8flarev1alpha1.GroupName && resource == "clusters"
}

var errManagementClusterUndeletable = errors.New(
	"is the management cluster and cannot be deleted")
