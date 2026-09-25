package workloads

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	eventsv1 "k8s.io/api/events/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	appsdefaults "k8s.io/kubernetes/pkg/apis/apps/v1"
	coredefaults "k8s.io/kubernetes/pkg/apis/core/v1"
)

func init() {
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme, discoveryv1.AddToScheme, batchv1.AddToScheme, policyv1.AddToScheme, storagev1.AddToScheme, certificatesv1.AddToScheme, authorizationv1.AddToScheme, rbacv1.AddToScheme, networkingv1.AddToScheme, coordinationv1.AddToScheme, autoscalingv1.AddToScheme, eventsv1.AddToScheme, resourcev1.AddToScheme, admissionregistrationv1.AddToScheme, appsdefaults.RegisterDefaults, coredefaults.RegisterDefaults} {
		utilruntime.Must(add(scheme.Scheme))
	}
}
