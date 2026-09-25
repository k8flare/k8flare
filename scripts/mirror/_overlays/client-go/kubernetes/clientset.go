//go:build js

package kubernetes

import (
	"fmt"
	"net/http"

	discovery "k8s.io/client-go/discovery"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	autoscalingv1 "k8s.io/client-go/kubernetes/typed/autoscaling/v1"
	autoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2"
	authorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	batchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	certificatesv1 "k8s.io/client-go/kubernetes/typed/certificates/v1"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1 "k8s.io/client-go/kubernetes/typed/discovery/v1"
	eventsv1 "k8s.io/client-go/kubernetes/typed/events/v1"
	networkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	policyv1 "k8s.io/client-go/kubernetes/typed/policy/v1"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	resourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
	resourcev1beta2 "k8s.io/client-go/kubernetes/typed/resource/v1beta2"
	schedulingv1alpha2 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha2"
	storagev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	CoreV1() corev1.CoreV1Interface
	AppsV1() appsv1.AppsV1Interface
	AutoscalingV1() autoscalingv1.AutoscalingV1Interface
	AutoscalingV2() autoscalingv2.AutoscalingV2Interface
	BatchV1() batchv1.BatchV1Interface
	CoordinationV1() coordinationv1.CoordinationV1Interface
	DiscoveryV1() discoveryv1.DiscoveryV1Interface
	StorageV1() storagev1.StorageV1Interface
	CertificatesV1() certificatesv1.CertificatesV1Interface
	AuthorizationV1() authorizationv1.AuthorizationV1Interface
	RbacV1() rbacv1.RbacV1Interface
	ResourceV1() resourcev1.ResourceV1Interface
	ResourceV1beta2() resourcev1beta2.ResourceV1beta2Interface
	SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface
	PolicyV1() policyv1.PolicyV1Interface
	EventsV1() eventsv1.EventsV1Interface
	NetworkingV1() networkingv1.NetworkingV1Interface
}

type Clientset struct {
	*discovery.DiscoveryClient
	corev1             *corev1.CoreV1Client
	appsv1             *appsv1.AppsV1Client
	autoscalingv1      *autoscalingv1.AutoscalingV1Client
	autoscalingv2      *autoscalingv2.AutoscalingV2Client
	batchv1            *batchv1.BatchV1Client
	coordinationv1     *coordinationv1.CoordinationV1Client
	discoveryv1        *discoveryv1.DiscoveryV1Client
	storagev1          *storagev1.StorageV1Client
	certificatesv1     *certificatesv1.CertificatesV1Client
	authorizationv1    *authorizationv1.AuthorizationV1Client
	rbacv1             *rbacv1.RbacV1Client
	resourcev1         *resourcev1.ResourceV1Client
	resourcev1beta2    *resourcev1beta2.ResourceV1beta2Client
	schedulingv1alpha2 *schedulingv1alpha2.SchedulingV1alpha2Client
	policyv1           *policyv1.PolicyV1Client
	eventsv1           *eventsv1.EventsV1Client
	networkingv1       *networkingv1.NetworkingV1Client
}

func (c *Clientset) CoreV1() corev1.CoreV1Interface {
	return c.corev1
}

func (c *Clientset) AppsV1() appsv1.AppsV1Interface {
	return c.appsv1
}

func (c *Clientset) AutoscalingV1() autoscalingv1.AutoscalingV1Interface {
	return c.autoscalingv1
}

func (c *Clientset) AutoscalingV2() autoscalingv2.AutoscalingV2Interface {
	return c.autoscalingv2
}

func (c *Clientset) BatchV1() batchv1.BatchV1Interface {
	return c.batchv1
}

func (c *Clientset) CoordinationV1() coordinationv1.CoordinationV1Interface {
	return c.coordinationv1
}

func (c *Clientset) DiscoveryV1() discoveryv1.DiscoveryV1Interface {
	return c.discoveryv1
}

func (c *Clientset) StorageV1() storagev1.StorageV1Interface {
	return c.storagev1
}

func (c *Clientset) CertificatesV1() certificatesv1.CertificatesV1Interface {
	return c.certificatesv1
}

func (c *Clientset) AuthorizationV1() authorizationv1.AuthorizationV1Interface {
	return c.authorizationv1
}

func (c *Clientset) RbacV1() rbacv1.RbacV1Interface {
	return c.rbacv1
}

func (c *Clientset) ResourceV1() resourcev1.ResourceV1Interface {
	return c.resourcev1
}

func (c *Clientset) ResourceV1beta2() resourcev1beta2.ResourceV1beta2Interface {
	return c.resourcev1beta2
}

func (c *Clientset) SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface {
	return c.schedulingv1alpha2
}

func (c *Clientset) PolicyV1() policyv1.PolicyV1Interface {
	return c.policyv1
}

func (c *Clientset) EventsV1() eventsv1.EventsV1Interface {
	return c.eventsv1
}

func (c *Clientset) NetworkingV1() networkingv1.NetworkingV1Interface {
	return c.networkingv1
}

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.UserAgent == "" {
		configShallowCopy.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	httpClient, err := rest.HTTPClientFor(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	return NewForConfigAndClient(&configShallowCopy, httpClient)
}

func NewForConfigAndClient(c *rest.Config, httpClient *http.Client) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		if configShallowCopy.Burst <= 0 {
			return nil, fmt.Errorf("burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0")
		}
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	var cs Clientset
	var err error
	cs.corev1, err = corev1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.appsv1, err = appsv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.autoscalingv1, err = autoscalingv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.autoscalingv2, err = autoscalingv2.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.batchv1, err = batchv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.coordinationv1, err = coordinationv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.discoveryv1, err = discoveryv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.storagev1, err = storagev1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.certificatesv1, err = certificatesv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.authorizationv1, err = authorizationv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.rbacv1, err = rbacv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.resourcev1, err = resourcev1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.resourcev1beta2, err = resourcev1beta2.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.schedulingv1alpha2, err = schedulingv1alpha2.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.policyv1, err = policyv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.eventsv1, err = eventsv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.networkingv1, err = networkingv1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

func NewForConfigOrDie(c *rest.Config) *Clientset {
	cs, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return cs
}

func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.corev1 = corev1.New(c)
	cs.appsv1 = appsv1.New(c)
	cs.autoscalingv1 = autoscalingv1.New(c)
	cs.autoscalingv2 = autoscalingv2.New(c)
	cs.batchv1 = batchv1.New(c)
	cs.coordinationv1 = coordinationv1.New(c)
	cs.discoveryv1 = discoveryv1.New(c)
	cs.storagev1 = storagev1.New(c)
	cs.certificatesv1 = certificatesv1.New(c)
	cs.authorizationv1 = authorizationv1.New(c)
	cs.rbacv1 = rbacv1.New(c)
	cs.resourcev1 = resourcev1.New(c)
	cs.resourcev1beta2 = resourcev1beta2.New(c)
	cs.schedulingv1alpha2 = schedulingv1alpha2.New(c)
	cs.policyv1 = policyv1.New(c)
	cs.eventsv1 = eventsv1.New(c)
	cs.networkingv1 = networkingv1.New(c)
	cs.DiscoveryClient = discovery.NewDiscoveryClient(c)
	return &cs
}
