package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func upstreamDiscovery(t *testing.T, gv schema.GroupVersion) map[string]metav1.APIResource {
	t.Helper()
	name := "apis__" + gv.Group + "__" + gv.Version + ".json"
	if gv.Group == "" {
		name = "api__" + gv.Version + ".json"
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".build", "kubernetes-mirror", "api", "discovery", name))
	if err != nil {
		t.Fatal(err)
	}
	var list metav1.APIResourceList
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	byName := map[string]metav1.APIResource{}
	for _, r := range list.APIResources {
		byName[r.Name] = r
	}
	return byName
}

func TestServedResourcesAreUpstreamDiscoveryEntries(t *testing.T) {
	for _, sgv := range Served {
		upstream := upstreamDiscovery(t, sgv.GV)
		for _, got := range sgv.Resources {
			want, ok := upstream[got.Name]
			if !ok {
				t.Errorf("%s: %s is not in upstream discovery", sgv.GV, got.Name)
				continue
			}
			want = metav1.APIResource{Name: want.Name, SingularName: want.SingularName, Namespaced: want.Namespaced, Kind: want.Kind, Verbs: want.Verbs, ShortNames: want.ShortNames, Categories: want.Categories}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: %s\n got %+v\nwant %+v", sgv.GV, got.Name, got, want)
			}
		}
	}
}

func TestRegisteredSubresourcesAreServed(t *testing.T) {
	served := map[string]bool{}
	for _, sgv := range Served {
		for _, res := range sgv.Resources {
			served[res.Name] = true
		}
	}
	for name := range Subresources {
		if !served[name] {
			t.Errorf("%s has an implementation but is not served", name)
		}
	}
}
