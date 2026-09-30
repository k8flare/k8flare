package apps

import (
	"context"
	"strings"
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
)

func deploymentStore(t *testing.T) *registry.Store {
	t.Helper()
	store, err := registry.NewStore(nil, schema.GroupVersion{Group: "apps", Version: "v1"}, metav1.APIResource{Name: "deployments", Kind: "Deployment", Namespaced: true})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func validDeployment() *appsv1.Deployment {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					DNSPolicy:     corev1.DNSClusterFirst,
					Containers:    []corev1.Container{{Name: "c", Image: "nginx", ImagePullPolicy: corev1.PullIfNotPresent, TerminationMessagePolicy: corev1.TerminationMessageReadFile}},
				},
			},
			RevisionHistoryLimit:    ptr.To[int32](10),
			ProgressDeadlineSeconds: ptr.To[int32](600),
		},
	}
	scheme.Scheme.Default(d)
	return d
}

func invalidFields(t *testing.T, err error) []string {
	t.Helper()
	status, ok := err.(apierrors.APIStatus)
	if !ok || !apierrors.IsInvalid(err) {
		t.Fatalf("want Invalid, got %v", err)
	}
	var fields []string
	for _, cause := range status.Status().Details.Causes {
		fields = append(fields, cause.Field+": "+cause.Message)
	}
	return fields
}

func requireFieldError(t *testing.T, err error, field, contains string) {
	t.Helper()
	for _, f := range invalidFields(t, err) {
		if strings.HasPrefix(f, field+":") && strings.Contains(f, contains) {
			return
		}
	}
	t.Fatalf("want %s error containing %q, got %v", field, contains, err)
}

func TestDeploymentCreateRejectsNegativeReplicas(t *testing.T) {
	store := deploymentStore(t)
	ctx := genericapirequest.WithNamespace(context.Background(), "default")
	d := validDeployment()
	d.Spec.Replicas = ptr.To[int32](-1)
	rest.FillObjectMetaSystemFields(d)
	err := rest.BeforeCreate(store.CreateStrategy, ctx, d)
	requireFieldError(t, err, "spec.replicas", "must be greater than or equal to 0")
}

func TestDeploymentCreateAcceptsValid(t *testing.T) {
	store := deploymentStore(t)
	ctx := genericapirequest.WithNamespace(context.Background(), "default")
	d := validDeployment()
	rest.FillObjectMetaSystemFields(d)
	if err := rest.BeforeCreate(store.CreateStrategy, ctx, d); err != nil {
		t.Fatal(err)
	}
	if d.Generation != 1 {
		t.Fatalf("generation=%d want 1", d.Generation)
	}
}

func TestDeploymentUpdateRejectsSelectorChange(t *testing.T) {
	store := deploymentStore(t)
	ctx := genericapirequest.WithNamespace(context.Background(), "default")
	old := validDeployment()
	old.ResourceVersion = "1"
	next := old.DeepCopy()
	next.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "other"}}
	next.Spec.Template.Labels = map[string]string{"app": "other"}
	store.UpdateStrategy.PrepareForUpdate(ctx, next, old)
	err := rest.BeforeUpdate(store.UpdateStrategy, ctx, next, old)
	requireFieldError(t, err, "spec.selector", "field is immutable")
}
