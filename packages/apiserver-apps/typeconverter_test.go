package apps

import (
	"strings"
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
)

func TestTypeConverterKnowsContainersAreKeyedByName(t *testing.T) {
	deployment := validDeployment()
	deployment.APIVersion, deployment.Kind = "apps/v1", "Deployment"
	converter, err := registry.TypeConverter()
	if err != nil {
		t.Fatal(err)
	}
	typed, err := converter.ObjectToTyped(deployment)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := typed.ToFieldSet()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fields.String(), `.spec.template.spec.containers[name="c"]`) {
		t.Fatalf("containers are not keyed by name: %s", fields.String())
	}
}
