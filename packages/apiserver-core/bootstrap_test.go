package core

import (
	"testing"

	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
)

func TestBootstrapKubernetesServicePassesValidation(t *testing.T) {
	store := coreStore(t, "services", "Service", true)
	if err := registrytest.Create(store, bootstrapObject(kubernetesService())); err != nil {
		t.Fatalf("the bootstrap kubernetes Service is rejected: %v", err)
	}
}

func TestBootstrapNamespacesPassValidation(t *testing.T) {
	store := coreStore(t, "namespaces", "Namespace", false)
	for _, name := range systemNamespaces {
		if err := registrytest.Create(store, bootstrapObject(systemNamespace(name))); err != nil {
			t.Fatalf("the bootstrap %s namespace is rejected: %v", name, err)
		}
	}
}
