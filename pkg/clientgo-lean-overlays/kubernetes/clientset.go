// Hand-curated: byte-identical to upstream's Interface declaration
// (kubernetes/clientset.go), NOT narrowed. See
// pkg/clientgo-lean-overlays/README.md's "correction" note: an
// earlier version of this file narrowed Interface to the five groups this
// repo's controllers/scheduler actually call, expecting to avoid ever
// needing the other ~49 groups' typed/*.go and applyconfigurations/*.go
// files. That held for the controller-manager path (whose controllers
// take narrow per-type informer interfaces directly), but not for
// pkg/scheduler.New's informerFactory parameter, whose fixed external
// type (k8s.io/client-go/informers.SharedInformerFactory) requires a
// *kubernetes.Interface satisfying the *full* width regardless -- Go
// interface satisfaction is static, so a value only usable for 5 groups
// cannot be passed where all ~54 are required, even if the scheduler only
// calls a handful at runtime. Reverted to upstream's full interface (this
// file, unmodified) + this repo's Clientset implementing the other ~49
// methods as panic stubs (pkg/leanclient/clientset/stubs.go, adapted from
// an earlier spike's identical pattern, see that file's doc comment) instead.
//go:build !leanwidth

package kubernetes

import (
	restclient "k8s.io/client-go/rest"

	discovery "k8s.io/client-go/discovery"
	admissionregistrationv1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
	admissionregistrationv1alpha1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1alpha1"
	admissionregistrationv1beta1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
	internalv1alpha1 "k8s.io/client-go/kubernetes/typed/apiserverinternal/v1alpha1"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	appsv1beta1 "k8s.io/client-go/kubernetes/typed/apps/v1beta1"
	appsv1beta2 "k8s.io/client-go/kubernetes/typed/apps/v1beta2"
	authenticationv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authenticationv1alpha1 "k8s.io/client-go/kubernetes/typed/authentication/v1alpha1"
	authenticationv1beta1 "k8s.io/client-go/kubernetes/typed/authentication/v1beta1"
	authorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	authorizationv1beta1 "k8s.io/client-go/kubernetes/typed/authorization/v1beta1"
	autoscalingv1 "k8s.io/client-go/kubernetes/typed/autoscaling/v1"
	autoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2"
	batchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	batchv1beta1 "k8s.io/client-go/kubernetes/typed/batch/v1beta1"
	certificatesv1 "k8s.io/client-go/kubernetes/typed/certificates/v1"
	certificatesv1alpha1 "k8s.io/client-go/kubernetes/typed/certificates/v1alpha1"
	certificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	coordinationv1alpha2 "k8s.io/client-go/kubernetes/typed/coordination/v1alpha2"
	coordinationv1beta1 "k8s.io/client-go/kubernetes/typed/coordination/v1beta1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1 "k8s.io/client-go/kubernetes/typed/discovery/v1"
	discoveryv1beta1 "k8s.io/client-go/kubernetes/typed/discovery/v1beta1"
	eventsv1 "k8s.io/client-go/kubernetes/typed/events/v1"
	eventsv1beta1 "k8s.io/client-go/kubernetes/typed/events/v1beta1"
	extensionsv1beta1 "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	flowcontrolv1 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1"
	flowcontrolv1beta1 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1beta1"
	flowcontrolv1beta2 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1beta2"
	flowcontrolv1beta3 "k8s.io/client-go/kubernetes/typed/flowcontrol/v1beta3"
	networkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	networkingv1beta1 "k8s.io/client-go/kubernetes/typed/networking/v1beta1"
	nodev1 "k8s.io/client-go/kubernetes/typed/node/v1"
	nodev1alpha1 "k8s.io/client-go/kubernetes/typed/node/v1alpha1"
	nodev1beta1 "k8s.io/client-go/kubernetes/typed/node/v1beta1"
	policyv1 "k8s.io/client-go/kubernetes/typed/policy/v1"
	policyv1beta1 "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	rbacv1alpha1 "k8s.io/client-go/kubernetes/typed/rbac/v1alpha1"
	rbacv1beta1 "k8s.io/client-go/kubernetes/typed/rbac/v1beta1"
	resourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
	resourcev1alpha3 "k8s.io/client-go/kubernetes/typed/resource/v1alpha3"
	resourcev1beta1 "k8s.io/client-go/kubernetes/typed/resource/v1beta1"
	resourcev1beta2 "k8s.io/client-go/kubernetes/typed/resource/v1beta2"
	schedulingv1 "k8s.io/client-go/kubernetes/typed/scheduling/v1"
	schedulingv1alpha2 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha2"
	schedulingv1beta1 "k8s.io/client-go/kubernetes/typed/scheduling/v1beta1"
	storagev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
	storagev1alpha1 "k8s.io/client-go/kubernetes/typed/storage/v1alpha1"
	storagev1beta1 "k8s.io/client-go/kubernetes/typed/storage/v1beta1"
	storagemigrationv1beta1 "k8s.io/client-go/kubernetes/typed/storagemigration/v1beta1"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	AdmissionregistrationV1() admissionregistrationv1.AdmissionregistrationV1Interface
	AdmissionregistrationV1alpha1() admissionregistrationv1alpha1.AdmissionregistrationV1alpha1Interface
	AdmissionregistrationV1beta1() admissionregistrationv1beta1.AdmissionregistrationV1beta1Interface
	InternalV1alpha1() internalv1alpha1.InternalV1alpha1Interface
	AppsV1() appsv1.AppsV1Interface
	AppsV1beta1() appsv1beta1.AppsV1beta1Interface
	AppsV1beta2() appsv1beta2.AppsV1beta2Interface
	AuthenticationV1() authenticationv1.AuthenticationV1Interface
	AuthenticationV1alpha1() authenticationv1alpha1.AuthenticationV1alpha1Interface
	AuthenticationV1beta1() authenticationv1beta1.AuthenticationV1beta1Interface
	AuthorizationV1() authorizationv1.AuthorizationV1Interface
	AuthorizationV1beta1() authorizationv1beta1.AuthorizationV1beta1Interface
	AutoscalingV1() autoscalingv1.AutoscalingV1Interface
	AutoscalingV2() autoscalingv2.AutoscalingV2Interface
	BatchV1() batchv1.BatchV1Interface
	BatchV1beta1() batchv1beta1.BatchV1beta1Interface
	CertificatesV1() certificatesv1.CertificatesV1Interface
	CertificatesV1beta1() certificatesv1beta1.CertificatesV1beta1Interface
	CertificatesV1alpha1() certificatesv1alpha1.CertificatesV1alpha1Interface
	CoordinationV1alpha2() coordinationv1alpha2.CoordinationV1alpha2Interface
	CoordinationV1beta1() coordinationv1beta1.CoordinationV1beta1Interface
	CoordinationV1() coordinationv1.CoordinationV1Interface
	CoreV1() corev1.CoreV1Interface
	DiscoveryV1() discoveryv1.DiscoveryV1Interface
	DiscoveryV1beta1() discoveryv1beta1.DiscoveryV1beta1Interface
	EventsV1() eventsv1.EventsV1Interface
	EventsV1beta1() eventsv1beta1.EventsV1beta1Interface
	ExtensionsV1beta1() extensionsv1beta1.ExtensionsV1beta1Interface
	FlowcontrolV1() flowcontrolv1.FlowcontrolV1Interface
	FlowcontrolV1beta1() flowcontrolv1beta1.FlowcontrolV1beta1Interface
	FlowcontrolV1beta2() flowcontrolv1beta2.FlowcontrolV1beta2Interface
	FlowcontrolV1beta3() flowcontrolv1beta3.FlowcontrolV1beta3Interface
	NetworkingV1() networkingv1.NetworkingV1Interface
	NetworkingV1beta1() networkingv1beta1.NetworkingV1beta1Interface
	NodeV1() nodev1.NodeV1Interface
	NodeV1alpha1() nodev1alpha1.NodeV1alpha1Interface
	NodeV1beta1() nodev1beta1.NodeV1beta1Interface
	PolicyV1() policyv1.PolicyV1Interface
	PolicyV1beta1() policyv1beta1.PolicyV1beta1Interface
	RbacV1() rbacv1.RbacV1Interface
	RbacV1beta1() rbacv1beta1.RbacV1beta1Interface
	RbacV1alpha1() rbacv1alpha1.RbacV1alpha1Interface
	ResourceV1() resourcev1.ResourceV1Interface
	ResourceV1beta2() resourcev1beta2.ResourceV1beta2Interface
	ResourceV1beta1() resourcev1beta1.ResourceV1beta1Interface
	ResourceV1alpha3() resourcev1alpha3.ResourceV1alpha3Interface
	SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface
	SchedulingV1beta1() schedulingv1beta1.SchedulingV1beta1Interface
	SchedulingV1() schedulingv1.SchedulingV1Interface
	StorageV1beta1() storagev1beta1.StorageV1beta1Interface
	StorageV1() storagev1.StorageV1Interface
	StorageV1alpha1() storagev1alpha1.StorageV1alpha1Interface
	StoragemigrationV1beta1() storagemigrationv1beta1.StoragemigrationV1beta1Interface
}

// Clientset's only purpose is satisfying
// k8s.io/client-go/tools/leaderelection/resourcelock's
// NewFromKubeconfig (clientset.NewForConfigOrDie(...).CoreV1()/
// .CoordinationV1()), a non-test function that's part of resourcelock's
// regular build (transitively compiled into pkg/scheduler) even though
// this repo never calls it (--leader-elect=false, no leader election).
// Embedding Interface (nil by default) makes *Clientset satisfy it via
// method promotion without hand-writing 54 more panic stubs; calling any
// promoted method on a nil embedded Interface panics at runtime same as
// this constructor already does, and NewFromKubeconfig is never reached.
type Clientset struct {
	Interface
}

// NewForConfigOrDie mirrors upstream's own convenience constructor's
// name+shape exactly (resourcelock calls it positionally).
func NewForConfigOrDie(c *restclient.Config) *Clientset {
	panic("leanclient: kubernetes.NewForConfigOrDie not implemented -- resourcelock's leader-election path is unused (--leader-elect=false)")
}
