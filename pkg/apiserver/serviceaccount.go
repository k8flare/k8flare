package apiserver

import (
	"context"
	"errors"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ensureDefaultServiceAccount creates the "default" ServiceAccount in namespace
// if it doesn't already exist, mirroring real Kubernetes' ServiceAccountsController
// (which creates ServiceAccount{ObjectMeta:{Name:"default"}} with no other fields —
// token/Secret provisioning is a separate, deprecated-by-default controller we do
// not replicate). Best-effort and idempotent: AlreadyExists is expected and ignored.
func ensureDefaultServiceAccount(ctx context.Context, saStore *ResourceStore, namespace string) {
	if saStore == nil {
		return
	}
	_, err := saStore.Create(ctx, namespace, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: namespace,
		},
	})
	if err == nil {
		return
	}
	var se *StatusError
	if errors.As(err, &se) && se.Status.Reason == metav1.StatusReasonAlreadyExists {
		return
	}
	log.Printf("failed to auto-provision default ServiceAccount in namespace %q: %v", namespace, err)
}

// ApplyPostCreateEffects performs side effects that must run after a resource is
// successfully persisted (as opposed to ApplyDefaults, which mutates the object
// before it's persisted). Currently: auto-provisioning the default ServiceAccount
// when a Namespace is created, the same duty real Kubernetes' controller-manager
// performs — implemented synchronously here because our apiserver already has
// direct, in-process access to the ServiceAccount store at Namespace-create time.
func ApplyPostCreateEffects(ctx context.Context, stores map[string]*ResourceStore, obj runtime.Object) {
	switch o := obj.(type) {
	case *corev1.Namespace:
		ensureDefaultServiceAccount(ctx, stores["serviceaccounts"], o.Name)
	}
}
