package all

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/k8flare/k8flare/packages/workloads"
)

func TestSyncPublishesTheRootCAOnlyIntoActiveNamespaces(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "alive"}, Status: v1.NamespaceStatus{Phase: v1.NamespaceActive}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "going", DeletionTimestamp: &now}, Status: v1.NamespaceStatus{Phase: v1.NamespaceTerminating}},
	)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"namespaces"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ConfigMaps("alive").Get(context.Background(), "kube-root-ca.crt", metav1.GetOptions{}); err != nil {
		t.Fatalf("active namespace has no root CA: %v", err)
	}
	if _, err := client.CoreV1().ConfigMaps("going").Get(context.Background(), "kube-root-ca.crt", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("terminating namespace got a create: %v", err)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "create" && action.GetResource().Resource == "configmaps" && action.GetNamespace() == "going" {
			t.Fatalf("create attempted in the terminating namespace: %v", action)
		}
	}
}
