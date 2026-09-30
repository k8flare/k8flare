package core

import (
	"testing"

	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func TestHeadlessServiceWithoutClusterIPsPassesValidation(t *testing.T) {
	store := coreStore(t, "services", "Service", true)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: metav1.NamespaceDefault},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP, SessionAffinity: corev1.ServiceAffinityNone, InternalTrafficPolicy: ptr.To(corev1.ServiceInternalTrafficPolicyCluster), ClusterIP: corev1.ClusterIPNone,
			Selector: map[string]string{"foo": "bar"},
			Ports:    []corev1.ServicePort{{Name: "http", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt32(80)}},
		},
	}
	if err := registrytest.Create(store, svc); err != nil {
		t.Fatalf("a headless Service created with only clusterIP is rejected: %v", err)
	}
	if len(svc.Spec.ClusterIPs) != 1 || svc.Spec.ClusterIPs[0] != corev1.ClusterIPNone {
		t.Fatalf("clusterIPs=%v", svc.Spec.ClusterIPs)
	}
}

func TestBootstrapKubernetesServicePassesValidation(t *testing.T) {
	store := coreStore(t, "services", "Service", true)
	if err := registrytest.Create(store, bootstrapObject(kubernetesService())); err != nil {
		t.Fatalf("the bootstrap kubernetes Service is rejected: %v", err)
	}
}

func TestBootstrapNamespacesPassValidation(t *testing.T) {
	store := coreStore(t, "namespaces", "Namespace", false)
	for _, name := range systemNamespaces {
		if err := registrytest.Create(store, bootstrapObject(systemNamespace(name))); err != nil {
			t.Fatalf("the bootstrap %s namespace is rejected: %v", name, err)
		}
	}
}
