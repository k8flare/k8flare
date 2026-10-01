package all

import (
	"context"
	"testing"

	"github.com/k8flare/k8flare/packages/workloads"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/controller/endpoint"
)

func endpointsWithoutService(name string, labels map[string]string) *v1.Endpoints {
	return &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: "e1", Labels: labels},
		Subsets: []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{IP: "10.0.0.24"}},
			Ports:     []v1.EndpointPort{{Name: "http", Port: 80, Protocol: v1.ProtocolTCP}},
		}},
	}
}

func TestSyncKeepsEndpointsWrittenByHandWithoutAService(t *testing.T) {
	client := fake.NewSimpleClientset(endpointsWithoutService("testservice", map[string]string{"test-endpoint-static": "true"}))
	for pass := 0; pass < 2; pass++ {
		if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"endpoints"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.CoreV1().Endpoints("default").Get(context.Background(), "testservice", metav1.GetOptions{}); err != nil {
		t.Fatalf("a pass removed Endpoints nobody asked it to manage: %v", err)
	}
}

func TestSyncRemovesEndpointsTheControllerLeftBehindADeletedService(t *testing.T) {
	client := fake.NewSimpleClientset(endpointsWithoutService("gone", map[string]string{endpoint.LabelManagedBy: endpoint.ControllerName}))
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"services"}); err != nil {
		t.Fatal(err)
	}
	_, err := client.CoreV1().Endpoints("default").Get(context.Background(), "gone", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("controller-managed Endpoints outlived their Service: %v", err)
	}
}
