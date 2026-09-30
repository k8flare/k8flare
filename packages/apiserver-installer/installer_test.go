package installer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	_ "github.com/k8flare/k8flare/packages/apiserver-rbac"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var rbacV1 = schema.GroupVersion{Group: "rbac.authorization.k8s.io", Version: "v1"}

func discoveredResources(t *testing.T) map[string]metav1.APIResource {
	t.Helper()
	mux := http.NewServeMux()
	deps := registry.Deps{Kine: &kine.Client{HTTP: &http.Client{}}}
	installed, err := Install(mux, deps, rbacV1)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	rec := httptest.NewRecorder()
	installed.Container.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/apis/rbac.authorization.k8s.io/v1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("discovery status %d: %s", rec.Code, rec.Body)
	}
	list := metav1.APIResourceList{}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	out := map[string]metav1.APIResource{}
	for _, r := range list.APIResources {
		out[r.Name] = r
	}
	return out
}

func TestRBACWrappersKeepDiscovery(t *testing.T) {
	wrappers := registry.Wrappers
	registry.Wrappers = nil
	want := discoveredResources(t)
	registry.Wrappers = wrappers

	got := discoveredResources(t)
	for _, name := range []string{"roles", "clusterroles", "rolebindings", "clusterrolebindings"} {
		if len(want[name].Verbs) == 0 {
			t.Fatalf("%s missing from baseline discovery", name)
		}
		if !reflect.DeepEqual(got[name], want[name]) {
			t.Errorf("%s discovery changed:\n got %+v\nwant %+v", name, got[name], want[name])
		}
	}
}
