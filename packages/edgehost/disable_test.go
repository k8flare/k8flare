package edgehost

import (
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func useDisabled(t *testing.T, list string) {
	t.Helper()
	SetDisabled(list)
	t.Cleanup(func() { SetDisabled("") })
}

func TestSetDisabledReplacesTheList(t *testing.T) {
	useDisabled(t, " metrics-server, servicelb ,,")
	if !Disabled(MetricsServer) || !Disabled(ServiceLB) || Disabled(EdgeRouting) {
		t.Fatal(disabled)
	}
	SetDisabled("edge-routing")
	if Disabled(MetricsServer) || Disabled(ServiceLB) || !Disabled(EdgeRouting) {
		t.Fatal(disabled)
	}
}

func TestFollowUpMetricsSendsNothingWhenMetricsServerIsDisabled(t *testing.T) {
	useDisabled(t, "metrics-server")
	if got := followUp(followIn{Target: "metrics"}); len(got.Sends) != 0 {
		t.Fatal(got)
	}
}

func TestFollowUpMetricsKeepsScrapingWhenOtherAddonsAreDisabled(t *testing.T) {
	useDisabled(t, "servicelb,edge-routing,coredns")
	if got := followUp(followIn{Target: "metrics"}); len(got.Sends) != 2 {
		t.Fatal(got)
	}
}

func edgeChanges() []queueMessage {
	return []queueMessage{
		{Kind: "change", Key: "/registry/services/default/web"},
		{Kind: "change", Key: "/registry/ingresses/default/web"},
		{Kind: "change", Key: "/registry/gateway.networking.k8s.io/gateways/default/edge"},
	}
}

func TestPlanQueueProvisionsNoHostnamesWhenServiceLBIsDisabled(t *testing.T) {
	useDisabled(t, "servicelb")
	got := planQueue(edgeChanges())
	if len(got.ServiceKeys) != 0 || len(got.GatewayKeys) != 3 {
		t.Fatal(got.ServiceKeys, got.GatewayKeys)
	}
	if len(got.Resources) != 4 || got.Resources[0] != "services" {
		t.Fatal(got.Resources)
	}
}

func TestPlanQueueBuildsNoRouteTableWhenEdgeRoutingIsDisabled(t *testing.T) {
	useDisabled(t, "edge-routing")
	got := planQueue(edgeChanges())
	if len(got.ServiceKeys) != 1 || len(got.GatewayKeys) != 0 {
		t.Fatal(got.ServiceKeys, got.GatewayKeys)
	}
	if len(got.Resources) != 4 || got.Resources[1] != "ingresses" {
		t.Fatal(got.Resources)
	}
}

func loadBalancerFixture(rules ...Rule) *countingStore {
	store := webFixture(rules...)
	store.values["/registry/services/default/web"] = mustJSON(corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Ports: []corev1.ServicePort{{Name: "http", Port: 80}}}})
	return store
}

func shopRule() Rule {
	return Rule{
		Source: "Ingress/default/web", Host: "shop.example.com",
		Path:     &pathMatch{Type: "PathPrefix", Value: "/"},
		Backends: []Backend{{Namespace: "default", Name: "web", PortName: "http", Weight: 1}},
	}
}

func TestProxyLeavesLoadBalancerHostsAloneWhenServiceLBIsDisabled(t *testing.T) {
	useClock(t)
	useDisabled(t, "servicelb")
	store := loadBalancerFixture(shopRule())
	tun := &tunnelLog{}
	r := httptest.NewRequest("GET", "https://web--default.k8flare.com/", nil)
	if Proxy(httptest.NewRecorder(), r, store.client(), tun.client()) {
		t.Fatal("load balancer host served")
	}
	if store.calls.Load() != 0 || tun.n != 0 {
		t.Fatal(store.calls.Load(), tun.n)
	}
	r = httptest.NewRequest("GET", "https://shop.example.com/", nil)
	if !Proxy(httptest.NewRecorder(), r, store.client(), tun.client()) || tun.n != 1 {
		t.Fatal("ingress host no longer served", tun.n)
	}
}

func TestProxyReadsNoRouteTableWhenEdgeRoutingIsDisabled(t *testing.T) {
	useClock(t)
	useDisabled(t, "edge-routing")
	store := loadBalancerFixture(shopRule())
	tun := &tunnelLog{}
	r := httptest.NewRequest("GET", "https://shop.example.com/", nil)
	if Proxy(httptest.NewRecorder(), r, store.client(), tun.client()) {
		t.Fatal("ingress host served")
	}
	if store.calls.Load() != 0 || tun.n != 0 {
		t.Fatal(store.calls.Load(), tun.n)
	}
	r = httptest.NewRequest("GET", "https://web--default.k8flare.com/", nil)
	if !Proxy(httptest.NewRecorder(), r, store.client(), tun.client()) || tun.n != 1 {
		t.Fatal("load balancer host no longer served", tun.n)
	}
}
