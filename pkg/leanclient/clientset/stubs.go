//go:build js && wasm && !leanwidth

// Adapted from third_party/leanclient-kcm's identical-purpose stubs.go
// (originally scaffolded from k8s.io/client-go's kubernetes.Interface,
// v1.36.2-k3s1, by a throwaway script) -- that version's real group set
// was {Core, Apps, Discovery, Coordination} (no Batch); this repo's
// Clientset (clientset.go) implements {Core, Apps, Batch, Discovery,
// Coordination} for real, so BatchV1's stub was deleted here (kept
// BatchV1beta1, a different, still-unused API version). Hand-edit freely
// (e.g. promoting a method to a real implementation in clientset.go, then
// deleting its stub here) if a future controller needs one of these
// groups for real.
package clientset

import (
	discovery "k8s.io/client-go/discovery"
	admissionregistrationv1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
	admissionregistrationv1alpha1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1alpha1"
	admissionregistrationv1beta1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
	internalv1alpha1 "k8s.io/client-go/kubernetes/typed/apiserverinternal/v1alpha1"
	appsv1beta1 "k8s.io/client-go/kubernetes/typed/apps/v1beta1"
	appsv1beta2 "k8s.io/client-go/kubernetes/typed/apps/v1beta2"
	authenticationv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authenticationv1alpha1 "k8s.io/client-go/kubernetes/typed/authentication/v1alpha1"
	authenticationv1beta1 "k8s.io/client-go/kubernetes/typed/authentication/v1beta1"
	authorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	authorizationv1beta1 "k8s.io/client-go/kubernetes/typed/authorization/v1beta1"
	autoscalingv1 "k8s.io/client-go/kubernetes/typed/autoscaling/v1"
	autoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2"
	batchv1beta1 "k8s.io/client-go/kubernetes/typed/batch/v1beta1"
	certificatesv1 "k8s.io/client-go/kubernetes/typed/certificates/v1"
	certificatesv1alpha1 "k8s.io/client-go/kubernetes/typed/certificates/v1alpha1"
	certificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	coordinationv1alpha2 "k8s.io/client-go/kubernetes/typed/coordination/v1alpha2"
	coordinationv1beta1 "k8s.io/client-go/kubernetes/typed/coordination/v1beta1"
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

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	panic("leanclient: Discovery() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AdmissionregistrationV1() admissionregistrationv1.AdmissionregistrationV1Interface {
	panic("leanclient: AdmissionregistrationV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AdmissionregistrationV1alpha1() admissionregistrationv1alpha1.AdmissionregistrationV1alpha1Interface {
	panic("leanclient: AdmissionregistrationV1alpha1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AdmissionregistrationV1beta1() admissionregistrationv1beta1.AdmissionregistrationV1beta1Interface {
	panic("leanclient: AdmissionregistrationV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) InternalV1alpha1() internalv1alpha1.InternalV1alpha1Interface {
	panic("leanclient: InternalV1alpha1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AppsV1beta1() appsv1beta1.AppsV1beta1Interface {
	panic("leanclient: AppsV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AppsV1beta2() appsv1beta2.AppsV1beta2Interface {
	panic("leanclient: AppsV1beta2() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AuthenticationV1() authenticationv1.AuthenticationV1Interface {
	panic("leanclient: AuthenticationV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AuthenticationV1alpha1() authenticationv1alpha1.AuthenticationV1alpha1Interface {
	panic("leanclient: AuthenticationV1alpha1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AuthenticationV1beta1() authenticationv1beta1.AuthenticationV1beta1Interface {
	panic("leanclient: AuthenticationV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AuthorizationV1() authorizationv1.AuthorizationV1Interface {
	panic("leanclient: AuthorizationV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AuthorizationV1beta1() authorizationv1beta1.AuthorizationV1beta1Interface {
	panic("leanclient: AuthorizationV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AutoscalingV1() autoscalingv1.AutoscalingV1Interface {
	panic("leanclient: AutoscalingV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) AutoscalingV2() autoscalingv2.AutoscalingV2Interface {
	panic("leanclient: AutoscalingV2() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) BatchV1beta1() batchv1beta1.BatchV1beta1Interface {
	panic("leanclient: BatchV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) CertificatesV1() certificatesv1.CertificatesV1Interface {
	panic("leanclient: CertificatesV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) CertificatesV1beta1() certificatesv1beta1.CertificatesV1beta1Interface {
	panic("leanclient: CertificatesV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) CertificatesV1alpha1() certificatesv1alpha1.CertificatesV1alpha1Interface {
	panic("leanclient: CertificatesV1alpha1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) CoordinationV1alpha2() coordinationv1alpha2.CoordinationV1alpha2Interface {
	panic("leanclient: CoordinationV1alpha2() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) CoordinationV1beta1() coordinationv1beta1.CoordinationV1beta1Interface {
	panic("leanclient: CoordinationV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) DiscoveryV1beta1() discoveryv1beta1.DiscoveryV1beta1Interface {
	panic("leanclient: DiscoveryV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) EventsV1() eventsv1.EventsV1Interface {
	panic("leanclient: EventsV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) EventsV1beta1() eventsv1beta1.EventsV1beta1Interface {
	panic("leanclient: EventsV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) ExtensionsV1beta1() extensionsv1beta1.ExtensionsV1beta1Interface {
	panic("leanclient: ExtensionsV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) FlowcontrolV1() flowcontrolv1.FlowcontrolV1Interface {
	panic("leanclient: FlowcontrolV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) FlowcontrolV1beta1() flowcontrolv1beta1.FlowcontrolV1beta1Interface {
	panic("leanclient: FlowcontrolV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) FlowcontrolV1beta2() flowcontrolv1beta2.FlowcontrolV1beta2Interface {
	panic("leanclient: FlowcontrolV1beta2() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) FlowcontrolV1beta3() flowcontrolv1beta3.FlowcontrolV1beta3Interface {
	panic("leanclient: FlowcontrolV1beta3() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) NetworkingV1() networkingv1.NetworkingV1Interface {
	panic("leanclient: NetworkingV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) NetworkingV1beta1() networkingv1beta1.NetworkingV1beta1Interface {
	panic("leanclient: NetworkingV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) NodeV1() nodev1.NodeV1Interface {
	panic("leanclient: NodeV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) NodeV1alpha1() nodev1alpha1.NodeV1alpha1Interface {
	panic("leanclient: NodeV1alpha1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) NodeV1beta1() nodev1beta1.NodeV1beta1Interface {
	panic("leanclient: NodeV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) PolicyV1() policyv1.PolicyV1Interface {
	panic("leanclient: PolicyV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) PolicyV1beta1() policyv1beta1.PolicyV1beta1Interface {
	panic("leanclient: PolicyV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) RbacV1() rbacv1.RbacV1Interface {
	panic("leanclient: RbacV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) RbacV1beta1() rbacv1beta1.RbacV1beta1Interface {
	panic("leanclient: RbacV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) RbacV1alpha1() rbacv1alpha1.RbacV1alpha1Interface {
	panic("leanclient: RbacV1alpha1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) ResourceV1() resourcev1.ResourceV1Interface {
	panic("leanclient: ResourceV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) ResourceV1beta2() resourcev1beta2.ResourceV1beta2Interface {
	panic("leanclient: ResourceV1beta2() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) ResourceV1beta1() resourcev1beta1.ResourceV1beta1Interface {
	panic("leanclient: ResourceV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) ResourceV1alpha3() resourcev1alpha3.ResourceV1alpha3Interface {
	panic("leanclient: ResourceV1alpha3() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface {
	panic("leanclient: SchedulingV1alpha2() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) SchedulingV1beta1() schedulingv1beta1.SchedulingV1beta1Interface {
	panic("leanclient: SchedulingV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) SchedulingV1() schedulingv1.SchedulingV1Interface {
	panic("leanclient: SchedulingV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) StorageV1beta1() storagev1beta1.StorageV1beta1Interface {
	panic("leanclient: StorageV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) StorageV1() storagev1.StorageV1Interface {
	panic("leanclient: StorageV1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) StorageV1alpha1() storagev1alpha1.StorageV1alpha1Interface {
	panic("leanclient: StorageV1alpha1() not implemented -- unused by workers/controllers' enabled controllers")
}

func (c *Clientset) StoragemigrationV1beta1() storagemigrationv1beta1.StoragemigrationV1beta1Interface {
	panic("leanclient: StoragemigrationV1beta1() not implemented -- unused by workers/controllers' enabled controllers")
}
