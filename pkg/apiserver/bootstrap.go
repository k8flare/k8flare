package apiserver

import (
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var bootstrapOnce sync.Once

// BootstrapCluster creates essential cluster resources if they don't exist.
// Safe to call multiple times — only runs once per Worker instance.
func BootstrapCluster(ctx context.Context, stores map[string]*ResourceStore) {
	bootstrapOnce.Do(func() {
		namespaces := []string{"default", "kube-system", "kube-public", "kube-node-lease"}

		nsStore := stores["namespaces"]
		saStore := stores["serviceaccounts"]
		svcStore := stores["services"]

		for _, ns := range namespaces {
			// Create namespace (ignore AlreadyExists)
			nsStore.Create(ctx, "", &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: ns},
			})

			// Create default ServiceAccount in each namespace
			ensureDefaultServiceAccount(ctx, saStore, ns)
		}

		// Real kube-apiserver bootstraps a "kubernetes" Service in "default"
		// on every start, exposing itself for in-cluster "kubernetes.default"
		// discovery; upstream e2e's own SynchronizedBeforeSuite (not any one
		// conformance test) requires it to exist to determine the cluster's
		// IP family. ClusterIP is the well-known first address of the
		// Service range, already excluded from AllocateNext's pool by
		// addressesReservedForFutureServices (clusterip.go) -- set directly,
		// not through AssignClusterIP, since that address is reserved, not
		// next-in-line.
		svcStore.Create(ctx, "default", &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				ClusterIP:  "10.43.0.1",
				ClusterIPs: []string{"10.43.0.1"},
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(443), Protocol: corev1.ProtocolTCP},
				},
			},
		})
	})
}
