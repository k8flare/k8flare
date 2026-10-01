package scheduling

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schedhelpers "k8s.io/kubernetes/pkg/apis/scheduling/v1"
)

func TestSystemPriorityClassesMatchUpstream(t *testing.T) {
	got := schedhelpers.SystemPriorityClasses()
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	byName := map[string]int32{}
	for _, pc := range got {
		byName[pc.Name] = pc.Value
	}
	if byName["system-node-critical"] != 2000001000 {
		t.Fatalf("system-node-critical = %d", byName["system-node-critical"])
	}
	if byName["system-cluster-critical"] != 2000000000 {
		t.Fatalf("system-cluster-critical = %d", byName["system-cluster-critical"])
	}
}

func TestBootstrapScheduling_SecondLoadSkipsCreates(t *testing.T) {
	cs := registrytest.NewCountingStore(t)
	store := cs.NewStore(t, schedulingv1.SchemeGroupVersion, metav1.APIResource{Name: "priorityclasses", Kind: "PriorityClass", SingularName: "priorityclass"})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	handler1 := bootstrapPriorityClasses(store, next)
	req1 := httptest.NewRequest("GET", "/apis/scheduling.k8s.io/v1/priorityclasses", nil)
	rec1 := httptest.NewRecorder()
	handler1.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first load status = %d", rec1.Code)
	}

	if puts := cs.RegistryPuts.Load(); puts == 0 {
		t.Fatal("first load created no priority classes")
	}
	if _, ok := cs.GetMarker("scheduling"); !ok {
		t.Fatal("first load did not write scheduling marker")
	}

	cs.RegistryPuts.Store(0)
	cs.MarkerGets.Store(0)
	handler2 := bootstrapPriorityClasses(store, next)
	req2 := httptest.NewRequest("GET", "/apis/scheduling.k8s.io/v1/priorityclasses", nil)
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second load status = %d", rec2.Code)
	}

	if puts := cs.RegistryPuts.Load(); puts != 0 {
		t.Fatalf("second load performed %d creates, want 0", puts)
	}
	if gets := cs.MarkerGets.Load(); gets != 1 {
		t.Fatalf("second load performed %d marker reads, want 1", gets)
	}
}
