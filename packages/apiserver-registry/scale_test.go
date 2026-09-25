package registry

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestScaleFromDeployment(t *testing.T) {
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", ResourceVersion: "7"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 2},
	}
	scale, err := scaleFrom(deploy)
	if err != nil {
		t.Fatal(err)
	}
	if scale.Spec.Replicas != 3 || scale.Status.Replicas != 2 || scale.Status.Selector != "app=web" {
		t.Fatalf("%+v", scale)
	}
	want := int32(5)
	if err := applyScale(deploy, &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: want}}); err != nil {
		t.Fatal(err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 5 {
		t.Fatalf("replicas %v", deploy.Spec.Replicas)
	}
}

func TestScaleRegisteredOnParentGroupVersions(t *testing.T) {
	for _, gv := range []schema.GroupVersion{appsv1.SchemeGroupVersion, corev1.SchemeGroupVersion, batchv1.SchemeGroupVersion} {
		gvk := gv.WithKind("Scale")
		obj, err := scheme.Scheme.New(gvk)
		if err != nil {
			t.Fatalf("%s: %v", gvk, err)
		}
		if _, ok := obj.(*autoscalingv1.Scale); !ok {
			t.Fatalf("%s: %T", gvk, obj)
		}
		if err := scheme.Scheme.Convert(&autoscalingv1.Scale{}, obj, gv); err != nil {
			t.Fatalf("convert into %s: %v", gv, err)
		}
	}
}

func TestScaleFromReplicationController(t *testing.T) {
	replicas := int32(1)
	rc := &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "rc"},
		Spec: corev1.ReplicationControllerSpec{
			Replicas: &replicas,
			Selector: map[string]string{"app": "rc"},
		},
	}
	scale, err := scaleFrom(rc)
	if err != nil {
		t.Fatal(err)
	}
	if scale.Spec.Replicas != 1 || scale.Status.Selector != "app=rc" {
		t.Fatalf("%+v", scale)
	}
}

func TestScaleFromJob(t *testing.T) {
	parallel := int32(4)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "batch", Namespace: "default"},
		Spec: batchv1.JobSpec{
			Parallelism: &parallel,
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"job": "batch"}},
		},
		Status: batchv1.JobStatus{Active: 3},
	}
	scale, err := scaleFrom(job)
	if err != nil {
		t.Fatal(err)
	}
	if scale.Spec.Replicas != 4 || scale.Status.Replicas != 3 || scale.Status.Selector != "job=batch" {
		t.Fatalf("%+v", scale)
	}
	if err := applyScale(job, &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 6}}); err != nil {
		t.Fatal(err)
	}
	if job.Spec.Parallelism == nil || *job.Spec.Parallelism != 6 {
		t.Fatalf("parallelism %v", job.Spec.Parallelism)
	}
}
