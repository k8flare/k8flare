package customresources

import (
	"reflect"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/registry/customresourcedefinition"
)

func TestCRDDiscoveryUsesUpstreamShortNames(t *testing.T) {
	var rest customresourcedefinition.REST
	got := crdDiscoveryResources()
	if len(got) == 0 || !reflect.DeepEqual(got[0].ShortNames, rest.ShortNames()) {
		t.Fatalf("shortNames=%v want %v", got[0].ShortNames, rest.ShortNames())
	}
	if !reflect.DeepEqual(got[0].Categories, rest.Categories()) {
		t.Fatalf("categories=%v want %v", got[0].Categories, rest.Categories())
	}
}
