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
	}, nil)
	if err == nil {
		return
	}
	var se *StatusError
	if errors.As(err, &se) && se.Status.Reason == metav1.StatusReasonAlreadyExists {
		return
	}
	log.Printf("failed to auto-provision default ServiceAccount in namespace %q: %v", namespace, err)
}

// ensureRootCAConfigMap creates the "kube-root-ca.crt" ConfigMap in namespace
// if it doesn't already exist, mirroring real Kubernetes' root-ca-cert-publisher
// controller (part of kube-controller-manager). Every e2e test namespace waits
// for this ConfigMap to exist before proceeding (test/e2e/framework/framework.go),
// so without it every namespaced e2e test times out in its BeforeEach, regardless
// of what the test itself checks. Real Kubernetes populates ca.crt with the
// cluster's CA bundle so in-pod clients can verify the apiserver's serving
// cert; k8flare has no equivalent single cluster CA (Cloudflare terminates TLS
// with a publicly-trusted certificate, verified via the system trust store),
// so this is a placeholder value — nothing in this stack reads it back.
// Best-effort and idempotent: AlreadyExists is expected and ignored.
func ensureRootCAConfigMap(ctx context.Context, cmStore *ResourceStore, namespace string) {
	if cmStore == nil {
		return
	}
	_, err := cmStore.Create(ctx, namespace, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-root-ca.crt",
			Namespace: namespace,
		},
		Data: map[string]string{
			"ca.crt": "",
		},
	}, nil)
	if err == nil {
		return
	}
	var se *StatusError
	if errors.As(err, &se) && se.Status.Reason == metav1.StatusReasonAlreadyExists {
		return
	}
	log.Printf("failed to auto-provision kube-root-ca.crt ConfigMap in namespace %q: %v", namespace, err)
}

// ApplyPostCreateEffects performs side effects that must run after a resource is
// successfully persisted (as opposed to ApplyDefaults, which mutates the object
// before it's persisted). Currently: auto-provisioning the default ServiceAccount
// and the kube-root-ca.crt ConfigMap when a Namespace is created, the same duty
// real Kubernetes' controller-manager performs — implemented synchronously here
// because our apiserver already has direct, in-process access to these stores
// at Namespace-create time.
func ApplyPostCreateEffects(ctx context.Context, stores map[string]*ResourceStore, obj runtime.Object) {
	switch o := obj.(type) {
	case *corev1.Namespace:
		ensureDefaultServiceAccount(ctx, stores["serviceaccounts"], o.Name)
		ensureRootCAConfigMap(ctx, stores["configmaps"], o.Name)
	}
}
