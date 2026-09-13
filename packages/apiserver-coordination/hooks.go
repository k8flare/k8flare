package coordination

import registry "github.com/k8flare/k8flare/packages/apiserver-registry"

func init() {
	for _, resource := range []string{"leases"} {
		registry.Customizers[resource] = func(store *registry.Store, _ registry.Deps) { registry.PokeControllersOn(store) }
	}
}
