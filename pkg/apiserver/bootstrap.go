package apiserver

import (
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var bootstrapOnce sync.Once

// BootstrapCluster creates essential cluster resources if they don't exist.
// Safe to call multiple times — only runs once per Worker instance.
func BootstrapCluster(ctx context.Context, stores map[string]*ResourceStore) {
	bootstrapOnce.Do(func() {
		namespaces := []string{"default", "kube-system", "kube-public", "kube-node-lease"}

		nsStore := stores["namespaces"]
		saStore := stores["serviceaccounts"]

		for _, ns := range namespaces {
			// Create namespace (ignore AlreadyExists)
			nsStore.Create(ctx, "", &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: ns},
			})

			// Create default ServiceAccount in each namespace
			ensureDefaultServiceAccount(ctx, saStore, ns)
		}
	})
}
