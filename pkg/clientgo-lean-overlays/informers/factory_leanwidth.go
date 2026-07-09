// Hand-curated: the `-tags leanwidth` sibling of factory.go. Nothing in
// the leanwidth build (pkg/controllers' KCM/garbage-collector wiring)
// ever calls informers.NewSharedInformerFactory or any of
// SharedInformerFactory's Apps()/Core()/Policy()/Resource()/Scheduling()/
// Storage() accessors -- KCM's RunControllerManager uses its own
// pkg/leanclient/informers.New instead, and the garbage collector
// (pkg/controllers/gc.go) builds its own informerfactory.InformerFactory
// directly against a metadata.Interface. The ONLY reason either needs
// this package at all is that k8s.io/controller-manager/pkg/
// informerfactory.InformerFactory's ForResource method signature is
// fixed to return this exact informers.GenericInformer type -- so this
// file declares just that, none of factory.go's SharedInformerFactory
// machinery (which would otherwise drag in informers/apps,
// informers/resource, informers/scheduling, etc., whose generated code
// calls kubernetes.Interface accessor methods leanwidth's narrowed
// Interface -- kubernetes/clientset_leanwidth.go -- doesn't have).
//go:build leanwidth

package informers

import (
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	cache "k8s.io/client-go/tools/cache"
)

// GenericInformer -- identical shape to factory.go's !leanwidth version
// and to upstream's own generic.go (deleted from this mirror, see
// factory.go's doc comment).
type GenericInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() cache.GenericLister
}

// SharedInformerFactory -- narrowed to exactly the two methods
// k8s.io/controller-manager/pkg/informerfactory.informerFactory (the
// unexported struct backing InformerFactory) calls on its
// typedInformerFactory field: ForResource and Start. That struct type
// itself is still never constructed under leanwidth (nothing here calls
// informerfactory.NewInformerFactory -- pkg/controllers/gc.go implements
// informerfactory.InformerFactory directly instead), but Go still
// type-checks the whole informerfactory package regardless of which of
// its functions are actually called, so this type must exist with (at
// least) this method set for that package to compile at all.
type SharedInformerFactory interface {
	ForResource(resource schema.GroupVersionResource) (GenericInformer, error)
	Start(stopCh <-chan struct{})
}
