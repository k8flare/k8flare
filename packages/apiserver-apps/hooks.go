package apps

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
	for _, name := range []string{"deployments", "replicasets", "statefulsets"} {
		parent := name
		registry.Subresources[parent+"/scale"] = func(stores map[string]*registry.Store, _ registry.Deps) rest.Storage {
			return registry.NewScaleREST(stores[parent])
		}
	}
}
