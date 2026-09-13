package scheduler

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	policyv1 "k8s.io/api/policy/v1"
	resourcev1 "k8s.io/api/resource/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	schedulingv1alpha2 "k8s.io/api/scheduling/v1alpha2"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func init() {
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, storagev1.AddToScheme, policyv1.AddToScheme,
		resourcev1.AddToScheme, resourcev1beta2.AddToScheme, schedulingv1alpha2.AddToScheme, eventsv1.AddToScheme,
	} {
		utilruntime.Must(add(scheme.Scheme))
	}
}
