package edgehost

import "testing"

func TestPlanQueue(t *testing.T) {
	got := planQueue([]queueMessage{
		{Kind: "change", Key: "/registry/services/default/empty-sel"},
		{Kind: "change", Key: "/registry/deployments/default/rs-probe"},
		{Kind: "change", Key: "/registry/gateway.networking.k8s.io/gateways/default/edge"},
		{Kind: "retry", Changed: []string{"endpoints", "services"}},
	})
	if len(got.Resources) != 5 || got.Resources[0] != "services" || got.Resources[1] != "deployments" || got.Resources[2] != "gateway.networking.k8s.io" || got.Resources[3] != "gateways" || got.Resources[4] != "endpoints" {
		t.Fatal(got.Resources)
	}
	roles := planQueue([]queueMessage{{Kind: "change", Key: "/registry/rbac.authorization.k8s.io/roles/default/view"}})
	if len(roles.Resources) != 2 || roles.Resources[0] != "rbac.authorization.k8s.io" || roles.Resources[1] != "roles" {
		t.Fatal(roles.Resources)
	}
	if len(got.ServiceKeys) != 1 || len(got.GatewayKeys) != 2 {
		t.Fatal(got.ServiceKeys, got.GatewayKeys)
	}
}

func TestPlanQueueRoutesIngressChangesToTheEdgePass(t *testing.T) {
	got := planQueue([]queueMessage{
		{Kind: "change", Key: "/registry/ingresses/default/web"},
		{Kind: "change", Key: "/registry/ingressclasses/k8flare"},
		{Kind: "change", Key: "/registry/deployments/default/web"},
	})
	if len(got.GatewayKeys) != 2 || len(got.ServiceKeys) != 0 {
		t.Fatal(got.GatewayKeys, got.ServiceKeys)
	}
}
