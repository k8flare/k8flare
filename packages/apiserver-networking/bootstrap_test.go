package networking

import (
	"net/http"
	"testing"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultServiceCIDRMatchesUpstreamNameAndRange(t *testing.T) {
	obj, ok := defaultServiceCIDR().(*networkingv1.ServiceCIDR)
	if !ok {
		t.Fatalf("type %T", defaultServiceCIDR())
	}
	if obj.Name != defaultServiceCIDRName {
		t.Fatalf("name %q", obj.Name)
	}
	if len(obj.Spec.CIDRs) != 1 || obj.Spec.CIDRs[0] != supervisor.ServiceCIDR.String() {
		t.Fatalf("cidrs %v", obj.Spec.CIDRs)
	}
	if len(obj.Status.Conditions) != 1 || obj.Status.Conditions[0].Type != networkingv1.ServiceCIDRConditionReady || obj.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("status %+v", obj.Status)
	}
}

func TestBootstrapServiceCIDRNilStorePassesThrough(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if bootstrapServiceCIDR(nil, next) == nil {
		t.Fatal("nil handler")
	}
}
