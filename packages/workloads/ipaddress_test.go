package workloads

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureServiceIPAddressesCreatesAndDeletes(t *testing.T) {
	client := fake.NewSimpleClientset(
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}, Spec: v1.ServiceSpec{ClusterIP: "10.43.0.5", ClusterIPs: []string{"10.43.0.5"}}},
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "headless", Namespace: "default"}, Spec: v1.ServiceSpec{ClusterIP: v1.ClusterIPNone}},
		&networkingv1.IPAddress{ObjectMeta: metav1.ObjectMeta{Name: "10.43.0.9"}, Spec: networkingv1.IPAddressSpec{ParentRef: &networkingv1.ParentReference{Resource: "services", Namespace: "default", Name: "gone"}}},
	)
	if err := ensureServiceIPAddresses(context.Background(), client, nil); err != nil {
		t.Fatal(err)
	}
	got, err := client.NetworkingV1().IPAddresses().Get(context.Background(), "10.43.0.5", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.ParentRef == nil || got.Spec.ParentRef.Name != "web" || got.Spec.ParentRef.Namespace != "default" {
		t.Fatalf("parent=%+v", got.Spec.ParentRef)
	}
	if _, err := client.NetworkingV1().IPAddresses().Get(context.Background(), "10.43.0.9", metav1.GetOptions{}); err == nil {
		t.Fatal("orphan ipaddress remains")
	}
	list, err := client.NetworkingV1().IPAddresses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range list.Items {
		if list.Items[i].Name == "None" || list.Items[i].Spec.ParentRef != nil && list.Items[i].Spec.ParentRef.Name == "headless" {
			t.Fatalf("headless address %s", list.Items[i].Name)
		}
	}
}
