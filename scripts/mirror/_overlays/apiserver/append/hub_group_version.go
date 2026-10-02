package endpoints

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// hubGroupVersionFor is the version a PATCH body is decoded to before the
// merge is applied: the internal version when the scheme has one, the served
// version otherwise. This apiserver registers external types only, and
// upstream hardcodes the internal hub. Added by scripts/mirror.
func hubGroupVersionFor(typer runtime.ObjectTyper, served schema.GroupVersion, kind schema.GroupVersionKind) schema.GroupVersion {
	internal := schema.GroupVersion{Group: kind.Group, Version: runtime.APIVersionInternal}
	if typer != nil && typer.Recognizes(internal.WithKind(kind.Kind)) {
		return internal
	}
	return served
}
