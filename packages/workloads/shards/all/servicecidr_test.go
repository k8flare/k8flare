package all

import (
	"context"
	"slices"
	"testing"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/controller/servicecidrs"

	"github.com/k8flare/k8flare/packages/workloads"
)

func serviceCIDRWrites(client *fake.Clientset) int {
	n := 0
	for _, a := range client.Actions() {
		if a.GetResource().Resource == "servicecidrs" && a.GetVerb() != "list" && a.GetVerb() != "get" {
			n++
		}
	}
	return n
}

func TestSyncProtectsANewServiceCIDRAndMarksItReady(t *testing.T) {
	ctx := context.Background()
	cidr := &networkingv1.ServiceCIDR{
		ObjectMeta: metav1.ObjectMeta{Name: "extra"},
		Spec:       networkingv1.ServiceCIDRSpec{CIDRs: []string{"10.90.0.0/24"}},
	}
	client := fake.NewClientset(cidr)
	if _, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"servicecidrs"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.NetworkingV1().ServiceCIDRs().Get(ctx, "extra", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Finalizers, servicecidrs.ServiceCIDRProtectionFinalizer) {
		t.Fatalf("finalizers = %v", got.Finalizers)
	}
	if !meta.IsStatusConditionTrue(got.Status.Conditions, networkingv1.ServiceCIDRConditionReady) {
		t.Fatalf("conditions = %v", got.Status.Conditions)
	}
	client.ClearActions()
	if _, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"servicecidrs"}); err != nil {
		t.Fatal(err)
	}
	if n := serviceCIDRWrites(client); n != 0 {
		t.Fatalf("a settled ServiceCIDR was written %d more times: %v", n, client.Actions())
	}
}

func TestSyncSettlesTheBootstrappedServiceCIDRInOnePass(t *testing.T) {
	ctx := context.Background()
	cidr := &networkingv1.ServiceCIDR{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernetes"},
		Spec:       networkingv1.ServiceCIDRSpec{CIDRs: []string{"10.43.0.0/16"}},
		Status: networkingv1.ServiceCIDRStatus{Conditions: []metav1.Condition{{
			Type:               networkingv1.ServiceCIDRConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             "Ready",
			Message:            "Kubernetes default Service CIDR is ready",
			LastTransitionTime: metav1.Now(),
		}}},
	}
	client := fake.NewClientset(cidr)
	if _, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"servicecidrs"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.NetworkingV1().ServiceCIDRs().Get(ctx, "kubernetes", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Finalizers, servicecidrs.ServiceCIDRProtectionFinalizer) {
		t.Fatalf("finalizers = %v", got.Finalizers)
	}
	if !meta.IsStatusConditionTrue(got.Status.Conditions, networkingv1.ServiceCIDRConditionReady) {
		t.Fatalf("conditions = %v", got.Status.Conditions)
	}
	client.ClearActions()
	if _, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"servicecidrs"}); err != nil {
		t.Fatal(err)
	}
	if n := serviceCIDRWrites(client); n != 0 {
		t.Fatalf("the default ServiceCIDR was written %d more times: %v", n, client.Actions())
	}
}

func TestSyncBooksTheServiceCIDRDeletionGracePeriod(t *testing.T) {
	ctx := context.Background()
	deleted := metav1.Now()
	cidr := &networkingv1.ServiceCIDR{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "extra",
			DeletionTimestamp: &deleted,
			Finalizers:        []string{servicecidrs.ServiceCIDRProtectionFinalizer},
		},
		Spec: networkingv1.ServiceCIDRSpec{CIDRs: []string{"10.90.0.0/24"}},
	}
	client := fake.NewClientset(cidr)
	result, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"servicecidrs"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.NetworkingV1().ServiceCIDRs().Get(ctx, "extra", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Finalizers, servicecidrs.ServiceCIDRProtectionFinalizer) {
		t.Fatalf("the finalizer left before the grace period: %v", got.Finalizers)
	}
	next := time.Duration(result.NextMs) * time.Millisecond
	if next <= 0 || next > 10*time.Second {
		t.Fatalf("next pass in %s, want within the deletion grace period", next)
	}
}

func TestSyncReleasesADeletedServiceCIDRAfterTheGracePeriod(t *testing.T) {
	ctx := context.Background()
	deleted := metav1.NewTime(time.Now().Add(-time.Minute))
	cidr := &networkingv1.ServiceCIDR{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "extra",
			DeletionTimestamp: &deleted,
			Finalizers:        []string{servicecidrs.ServiceCIDRProtectionFinalizer},
		},
		Spec: networkingv1.ServiceCIDRSpec{CIDRs: []string{"10.90.0.0/24"}},
	}
	client := fake.NewClientset(cidr)
	if _, err := workloads.Sync(ctx, client, []byte("ca"), nil, nil, []string{"servicecidrs"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.NetworkingV1().ServiceCIDRs().Get(ctx, "extra", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(got.Finalizers, servicecidrs.ServiceCIDRProtectionFinalizer) {
		t.Fatalf("the finalizer is still there: %v", got.Finalizers)
	}
}
