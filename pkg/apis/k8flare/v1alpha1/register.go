package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is k8flare's own API group.
const GroupName = "k8flare.com"

// SchemeGroupVersion is the GroupVersion this package's types belong to.
// Named to match upstream's convention for a versioned API package, so
// apidef.Table's entries read the same as every other group's.
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// Resource takes an unqualified resource name and returns it qualified with
// this group. Mirrors upstream's per-group helper of the same name.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// There is deliberately no SchemeBuilder/AddToScheme here: registration
// flows from apidef.Table through pkg/apiserver/scheme.go's init, which
// calls Scheme.AddKnownTypes for every GroupVersion in the table. A second
// registration path would be a way for the two to drift apart -- the exact
// failure apidef.Table exists to prevent.
