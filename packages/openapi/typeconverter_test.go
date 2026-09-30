package openapi

import (
	"fmt"
	"strings"
	"testing"

	_ "github.com/k8flare/k8flare/packages/apiserver-events"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestEveryServedKindHasATypeInTheTypeConverter(t *testing.T) {
	converter, err := registry.TypeConverter()
	if err != nil {
		t.Fatal(err)
	}
	if kind := fmt.Sprintf("%T", converter); strings.Contains(strings.ToLower(kind), "deduced") {
		t.Fatalf("the served definitions are not loaded: %s", kind)
	}
	for _, sgv := range registry.Served {
		for _, res := range sgv.Resources {
			if strings.Contains(res.Name, "/") {
				continue
			}
			gvk := sgv.GV.WithKind(res.Kind)
			obj, err := scheme.Scheme.New(gvk)
			if err != nil {
				t.Errorf("%s: %v", gvk, err)
				continue
			}
			obj.GetObjectKind().SetGroupVersionKind(gvk)
			if _, err := converter.ObjectToTyped(obj); err != nil {
				t.Errorf("%s: %v", gvk, err)
			}
		}
	}
}
