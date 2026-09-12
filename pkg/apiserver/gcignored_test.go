package apiserver

import (
	"reflect"
	"testing"

	"k8s.io/kubernetes/pkg/controller/garbagecollector"
)

func TestGCIgnoredResourcesMatchesUpstream(t *testing.T) {
	if want := garbagecollector.DefaultIgnoredResources(); !reflect.DeepEqual(gcIgnoredResources, want) {
		t.Fatalf("gcIgnoredResources = %v, want %v -- the guards wait for dependents the collector will never orphan", gcIgnoredResources, want)
	}
}
