package batch

import registry "github.com/k8flare/k8flare/packages/apiserver-registry"

func init() {
	for _, resource := range []string{"jobs", "cronjobs"} {
		registry.Customizers[resource] = func(store *registry.Store, _ registry.Deps) { registry.PokeControllersOn(store) }
	}
}
