package batch

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "batch", Version: "v1"}, metav1.APIResource{Name: "jobs", Kind: "Job", Namespaced: true})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "default"},
		Spec: batchv1.JobSpec{
			Parallelism: ptr.To[int32](-1),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers:    []corev1.Container{{Name: "c", Image: "busybox"}},
			}},
		},
	}
	scheme.Scheme.Default(job)
	obj := job
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.parallelism", "must be greater than or equal to 0")
}

func TestUpstreamAllowsPrivilegedTemplate(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "batch", Version: "v1"}, metav1.APIResource{Name: "jobs", Kind: "Job", Namespaced: true})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "default"},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{{
					Name:            "c",
					Image:           "busybox",
					SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true)},
				}},
			}},
		},
	}
	scheme.Scheme.Default(job)
	if err := registrytest.Create(store, job); err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamGeneratesJobSelector(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "batch", Version: "v1"}, metav1.APIResource{Name: "jobs", Kind: "Job", Namespaced: true})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "default"},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers:    []corev1.Container{{Name: "c", Image: "busybox"}},
			}},
		},
	}
	scheme.Scheme.Default(job)
	if err := registrytest.Create(store, job); err != nil {
		t.Fatal(err)
	}
	if job.Spec.Selector == nil || job.Spec.Selector.MatchLabels[batchv1.ControllerUidLabel] == "" {
		t.Fatalf("selector=%v", job.Spec.Selector)
	}
	if job.Spec.Template.Labels[batchv1.JobNameLabel] != "j" {
		t.Fatalf("labels=%v", job.Spec.Template.Labels)
	}
}
