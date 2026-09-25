package core

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceProxyDial(t *testing.T) {
	node := "n1"
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
			{Name: "http", Port: 80},
			{Name: "alt", Port: 8080},
		}},
	}
	eps := &corev1.Endpoints{
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.42.0.9", NodeName: &node}},
			Ports:     []corev1.EndpointPort{{Name: "http", Port: 8080}, {Name: "alt", Port: 9090}},
		}},
	}
	nodeOf := func(addr corev1.EndpointAddress) string {
		if addr.NodeName != nil {
			return *addr.NodeName
		}
		return ""
	}
	d, err := serviceProxyDial(svc, eps, "80", nodeOf)
	if err != nil {
		t.Fatal(err)
	}
	if d != (serviceDial{node: "n1", host: "10.42.0.9", port: "8080"}) {
		t.Fatalf("got %+v", d)
	}
	d, err = serviceProxyDial(svc, eps, "", nodeOf)
	if err != nil {
		t.Fatal(err)
	}
	if d.port != "8080" {
		t.Fatalf("default port %s", d.port)
	}
	if _, err := serviceProxyDial(svc, eps, "81", nodeOf); err == nil {
		t.Fatal("expected missing service port")
	}
}

func TestServiceProxyDialNoEndpoints(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}}
	if _, err := serviceProxyDial(svc, &corev1.Endpoints{}, "", func(corev1.EndpointAddress) string { return "n1" }); err == nil {
		t.Fatal("expected no endpoints")
	}
}
