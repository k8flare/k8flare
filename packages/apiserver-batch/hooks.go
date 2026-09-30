package batch

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
)

func init() {
	registry.Customizers["jobs"] = func(store *registry.Store, _ registry.Deps) {
		store.DeleteStrategy = registry.OrphanByDefault{RESTDeleteStrategy: store.DeleteStrategy}
	}

}
