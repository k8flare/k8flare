package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

var ipCodec runtime.Codec

func init() {
	s := runtime.NewScheme()
	networkingv1.AddToScheme(s)
	ipCodec = serializer.NewCodecFactory(s).LegacyCodec(networkingv1.SchemeGroupVersion)
}

const serviceStoragePrefix = "/registry/services/"

func init() {
	registry.Customizers["services"] = func(store *registry.Store, deps registry.Deps) {
		store.CreateStrategy = serviceCreateStrategy{store.CreateStrategy}
		store.UpdateStrategy = serviceUpdateStrategy{store.UpdateStrategy}
		assign := func(ctx context.Context, obj runtime.Object) error {
			svc, ok := obj.(*corev1.Service)
			if !ok {
				return nil
			}
			if releaseExternalName(svc) {
				return nil
			}
			if svc.Spec.ClusterIP == corev1.ClusterIPNone {
				defaultHeadlessClusterIPs(svc)
				return nil
			}
			if svc.Spec.ClusterIP == "" {
				ip, err := freeClusterIP(ctx, deps)
				if err != nil {
					println("services: cluster ip allocation failed:", err.Error())
					return err
				}
				svc.Spec.ClusterIP = ip
			} else if err := specifiedClusterIPAllowed(svc); err != nil {
				return err
			}
			if len(svc.Spec.ClusterIPs) == 0 && svc.Spec.ClusterIP != "" {
				svc.Spec.ClusterIPs = []string{svc.Spec.ClusterIP}
			}
			defaultServiceIPFamily(svc)
			return assignNodePorts(ctx, svc, deps)
		}
		store.BeginCreate = func(ctx context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			if err := clusterIPSliceAgrees(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := clusterIPNoneAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := clusterIPNodePortAllowed(serviceOf(obj), nil); err != nil {
				return nil, err
			}
			if err := duplicateServicePorts(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := loadBalancerSourceRangesAllowed(serviceOf(obj), nil); err != nil {
				return nil, err
			}
			if err := sessionAffinityAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := servicePortsRequired(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := externalNameAllowed(serviceOf(obj), nil); err != nil {
				return nil, err
			}
			if err := servicePortValues(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := externalTrafficPolicyAllowed(serviceOf(obj), nil); err != nil {
				return nil, err
			}
			if err := internalTrafficPolicyAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := externalIPsAllowed(serviceOf(obj), nil); err != nil {
				return nil, err
			}
			if err := serviceTypeAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := serviceSelectorAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := healthCheckNodePortAllowed(serviceOf(obj), nil); err != nil {
				return nil, err
			}
			picked := serviceOf(obj) != nil && needsClusterIP(serviceOf(obj))
			if err := assign(ctx, obj); err != nil {
				return nil, err
			}
			return claimPickedClusterIP(ctx, deps, serviceOf(obj), picked)
		}
		store.BeginUpdate = func(ctx context.Context, obj, old runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			if err := immutableClusterIP(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := immutableClusterIPs(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := immutableIPFamilies(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := clusterIPSliceAgrees(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := clusterIPNoneAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := clusterIPNodePortAllowed(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := duplicateServicePorts(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := loadBalancerSourceRangesAllowed(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := sessionAffinityAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := servicePortsRequired(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := externalNameAllowed(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := servicePortValues(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := externalTrafficPolicyAllowed(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := internalTrafficPolicyAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := externalIPsAllowed(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := serviceTypeAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := serviceSelectorAllowed(serviceOf(obj)); err != nil {
				return nil, err
			}
			if err := immutableHealthCheckNodePort(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := healthCheckNodePortAllowed(serviceOf(obj), serviceOf(old)); err != nil {
				return nil, err
			}
			if err := assign(ctx, obj); err != nil {
				return nil, err
			}
			return finishServiceIPs(ctx, deps, serviceOf(old), serviceOf(obj))
		}
		store.AfterDelete = func(obj runtime.Object, options *metav1.DeleteOptions) {
			releaseServiceIPs(context.Background(), deps, serviceOf(obj))
			deleteServiceEndpoints(serviceOf(obj), options)
		}
	}
}

type serviceCreateStrategy struct{ rest.RESTCreateStrategy }

func (s serviceCreateStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	if svc, ok := obj.(*corev1.Service); ok && svc.Spec.ClusterIP != "" && len(svc.Spec.ClusterIPs) == 0 {
		svc.Spec.ClusterIPs = []string{svc.Spec.ClusterIP}
	}
	if s.RESTCreateStrategy != nil {
		s.RESTCreateStrategy.PrepareForCreate(ctx, obj)
	}
}

type serviceUpdateStrategy struct{ rest.RESTUpdateStrategy }

func (s serviceUpdateStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	if s.RESTUpdateStrategy != nil {
		s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	}
	newSvc, ok1 := obj.(*corev1.Service)
	oldSvc, ok2 := old.(*corev1.Service)
	if !ok1 || !ok2 {
		return
	}
	dropServiceTypeFields(newSvc, oldSvc)
}

func dropServiceTypeFields(newSvc, oldSvc *corev1.Service) {
	if needsNodePorts(oldSvc) && !needsNodePorts(newSvc) && sameNodePorts(oldSvc, newSvc) {
		for i := range newSvc.Spec.Ports {
			newSvc.Spec.Ports[i].NodePort = 0
		}
	}
	if needsHealthCheckNodePort(oldSvc) && !needsHealthCheckNodePort(newSvc) && oldSvc.Spec.HealthCheckNodePort == newSvc.Spec.HealthCheckNodePort {
		newSvc.Spec.HealthCheckNodePort = 0
	}
	if oldSvc.Spec.Type == corev1.ServiceTypeLoadBalancer && newSvc.Spec.Type != corev1.ServiceTypeLoadBalancer && sameBoolPtr(oldSvc.Spec.AllocateLoadBalancerNodePorts, newSvc.Spec.AllocateLoadBalancerNodePorts) {
		newSvc.Spec.AllocateLoadBalancerNodePorts = nil
	}
	if oldSvc.Spec.Type == corev1.ServiceTypeLoadBalancer && newSvc.Spec.Type != corev1.ServiceTypeLoadBalancer && sameStringPtr(oldSvc.Spec.LoadBalancerClass, newSvc.Spec.LoadBalancerClass) {
		newSvc.Spec.LoadBalancerClass = nil
	}
	if externallyAccessible(oldSvc) && !externallyAccessible(newSvc) && oldSvc.Spec.ExternalTrafficPolicy == newSvc.Spec.ExternalTrafficPolicy {
		newSvc.Spec.ExternalTrafficPolicy = ""
	}
	if newSvc.Spec.Type == corev1.ServiceTypeExternalName {
		if sameIPFamilies(oldSvc, newSvc) {
			newSvc.Spec.IPFamilies = nil
		}
		if sameIPFamilyPolicy(oldSvc, newSvc) {
			newSvc.Spec.IPFamilyPolicy = nil
		}
	}
	if newSvc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		newSvc.Status.LoadBalancer = corev1.LoadBalancerStatus{}
	}
}

func externallyAccessible(svc *corev1.Service) bool {
	return svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeLoadBalancer
}

func sameStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameIPFamilies(oldSvc, newSvc *corev1.Service) bool {
	if len(oldSvc.Spec.IPFamilies) != len(newSvc.Spec.IPFamilies) {
		return false
	}
	for i := range oldSvc.Spec.IPFamilies {
		if oldSvc.Spec.IPFamilies[i] != newSvc.Spec.IPFamilies[i] {
			return false
		}
	}
	return true
}

func sameIPFamilyPolicy(oldSvc, newSvc *corev1.Service) bool {
	if oldSvc.Spec.IPFamilyPolicy == nil || newSvc.Spec.IPFamilyPolicy == nil {
		return oldSvc.Spec.IPFamilyPolicy == nil && newSvc.Spec.IPFamilyPolicy == nil
	}
	return *oldSvc.Spec.IPFamilyPolicy == *newSvc.Spec.IPFamilyPolicy
}

func sameNodePorts(oldSvc, newSvc *corev1.Service) bool {
	oldPorts := map[int32]bool{}
	for _, p := range oldSvc.Spec.Ports {
		if p.NodePort != 0 {
			oldPorts[p.NodePort] = true
		}
	}
	for _, p := range newSvc.Spec.Ports {
		if p.NodePort != 0 && !oldPorts[p.NodePort] {
			return false
		}
	}
	return true
}

func needsHealthCheckNodePort(svc *corev1.Service) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer && svc.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyLocal
}

func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func immutableHealthCheckNodePort(newSvc, oldSvc *corev1.Service) error {
	if newSvc == nil || oldSvc == nil || !needsHealthCheckNodePort(oldSvc) || !needsHealthCheckNodePort(newSvc) || newSvc.Spec.HealthCheckNodePort == oldSvc.Spec.HealthCheckNodePort {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), newSvc.Name, field.ErrorList{
		field.Forbidden(field.NewPath("spec", "healthCheckNodePort"), "field is immutable"),
	})
}

func healthCheckNodePortAllowed(svc, old *corev1.Service) error {
	if svc == nil || needsHealthCheckNodePort(svc) || svc.Spec.HealthCheckNodePort == 0 {
		return nil
	}
	if old != nil && old.Spec.HealthCheckNodePort == svc.Spec.HealthCheckNodePort && needsHealthCheckNodePort(old) {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
		field.Invalid(field.NewPath("spec", "healthCheckNodePort"), svc.Spec.HealthCheckNodePort, "may only be set when `type` is 'LoadBalancer' and `externalTrafficPolicy` is 'Local'"),
	})
}

func immutableClusterIPs(newSvc, oldSvc *corev1.Service) error {
	if newSvc == nil || oldSvc == nil {
		return nil
	}
	if newSvc.Spec.Type == corev1.ServiceTypeExternalName || oldSvc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}
	if headlessService(newSvc) && headlessService(oldSvc) {
		return nil
	}
	var errs field.ErrorList
	switch {
	case len(oldSvc.Spec.ClusterIPs) == len(newSvc.Spec.ClusterIPs):
		for i, ip := range oldSvc.Spec.ClusterIPs {
			if ip != newSvc.Spec.ClusterIPs[i] {
				errs = append(errs, field.Invalid(field.NewPath("spec", "clusterIPs").Index(i), newSvc.Spec.ClusterIPs, "may not change once set"))
			}
		}
	case len(oldSvc.Spec.ClusterIPs) > len(newSvc.Spec.ClusterIPs):
		if len(newSvc.Spec.ClusterIPs) == 0 {
			errs = append(errs, field.Invalid(field.NewPath("spec", "clusterIPs").Index(0), newSvc.Spec.ClusterIPs, "primary clusterIP can not be unset"))
		}
		if len(oldSvc.Spec.ClusterIPs) > 0 && len(newSvc.Spec.ClusterIPs) > 0 && newSvc.Spec.ClusterIPs[0] != oldSvc.Spec.ClusterIPs[0] {
			errs = append(errs, field.Invalid(field.NewPath("spec", "clusterIPs").Index(0), newSvc.Spec.ClusterIPs, "may not change once set"))
		}
		if len(newSvc.Spec.ClusterIPs) == 1 && (newSvc.Spec.IPFamilyPolicy == nil || *newSvc.Spec.IPFamilyPolicy != corev1.IPFamilyPolicySingleStack) {
			errs = append(errs, field.Invalid(field.NewPath("spec", "ipFamilyPolicy"), newSvc.Spec.IPFamilyPolicy, "must be set to 'SingleStack' when releasing the secondary clusterIP"))
		}
	case len(oldSvc.Spec.ClusterIPs) > 0 && len(newSvc.Spec.ClusterIPs) > 0 && newSvc.Spec.ClusterIPs[0] != oldSvc.Spec.ClusterIPs[0]:
		errs = append(errs, field.Invalid(field.NewPath("spec", "clusterIPs").Index(0), newSvc.Spec.ClusterIPs, "may not change once set"))
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), newSvc.Name, errs)
}

func serviceSelectorAllowed(svc *corev1.Service) error {
	if svc == nil || svc.Spec.Selector == nil {
		return nil
	}
	errs := metav1validation.ValidateLabels(svc.Spec.Selector, field.NewPath("spec", "selector"))
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, errs)
}

func serviceTypeAllowed(svc *corev1.Service) error {
	if svc == nil || svc.Spec.Type == "" {
		return nil
	}
	switch svc.Spec.Type {
	case corev1.ServiceTypeClusterIP, corev1.ServiceTypeNodePort, corev1.ServiceTypeLoadBalancer, corev1.ServiceTypeExternalName:
		return nil
	default:
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.NotSupported(field.NewPath("spec", "type"), svc.Spec.Type, []string{
				string(corev1.ServiceTypeClusterIP),
				string(corev1.ServiceTypeExternalName),
				string(corev1.ServiceTypeLoadBalancer),
				string(corev1.ServiceTypeNodePort),
			}),
		})
	}
}

func externalIPsAllowed(svc, old *corev1.Service) error {
	if svc == nil {
		return nil
	}
	existing := map[string]bool{}
	if old != nil {
		for _, ip := range old.Spec.ExternalIPs {
			existing[ip] = true
		}
	}
	var errs field.ErrorList
	for i, ip := range svc.Spec.ExternalIPs {
		if existing[ip] {
			continue
		}
		path := field.NewPath("spec", "externalIPs").Index(i)
		parsed := net.ParseIP(ip)
		if parsed == nil {
			errs = append(errs, field.Invalid(path, ip, "must be a valid IP address"))
			continue
		}
		switch {
		case parsed.IsUnspecified():
			errs = append(errs, field.Invalid(path, ip, fmt.Sprintf("may not be unspecified (%v)", ip)))
		case parsed.IsLoopback():
			errs = append(errs, field.Invalid(path, ip, "may not be in the loopback range (127.0.0.0/8, ::1/128)"))
		case parsed.IsLinkLocalUnicast():
			errs = append(errs, field.Invalid(path, ip, "may not be in the link-local range (169.254.0.0/16, fe80::/10)"))
		case parsed.IsLinkLocalMulticast():
			errs = append(errs, field.Invalid(path, ip, "may not be in the link-local multicast range (224.0.0.0/24, ff02::/10)"))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, errs)
}

func internalTrafficPolicyAllowed(svc *corev1.Service) error {
	if svc == nil || svc.Spec.InternalTrafficPolicy == nil {
		return nil
	}
	switch *svc.Spec.InternalTrafficPolicy {
	case corev1.ServiceInternalTrafficPolicyCluster, corev1.ServiceInternalTrafficPolicyLocal:
		return nil
	default:
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.NotSupported(field.NewPath("spec", "internalTrafficPolicy"), *svc.Spec.InternalTrafficPolicy, []string{string(corev1.ServiceInternalTrafficPolicyCluster), string(corev1.ServiceInternalTrafficPolicyLocal)}),
		})
	}
}

func externalTrafficPolicyAllowed(svc, old *corev1.Service) error {
	if svc == nil || svc.Spec.ExternalTrafficPolicy == "" {
		return nil
	}
	if !serviceExternallyAccessible(svc) {
		if old != nil && serviceExternallyAccessible(old) && old.Spec.ExternalTrafficPolicy == svc.Spec.ExternalTrafficPolicy {
			return nil
		}
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.Invalid(field.NewPath("spec", "externalTrafficPolicy"), svc.Spec.ExternalTrafficPolicy, "may only be set for externally-accessible services"),
		})
	}
	switch svc.Spec.ExternalTrafficPolicy {
	case corev1.ServiceExternalTrafficPolicyCluster, corev1.ServiceExternalTrafficPolicyLocal:
		return nil
	default:
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.NotSupported(field.NewPath("spec", "externalTrafficPolicy"), svc.Spec.ExternalTrafficPolicy, []string{string(corev1.ServiceExternalTrafficPolicyCluster), string(corev1.ServiceExternalTrafficPolicyLocal)}),
		})
	}
}

func serviceExternallyAccessible(svc *corev1.Service) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort || (svc.Spec.Type == corev1.ServiceTypeClusterIP && len(svc.Spec.ExternalIPs) > 0)
}

func servicePortValues(svc *corev1.Service) error {
	if svc == nil {
		return nil
	}
	var errs field.ErrorList
	names := map[string]bool{}
	requireName := len(svc.Spec.Ports) > 1
	for i, port := range svc.Spec.Ports {
		path := field.NewPath("spec", "ports").Index(i)
		if requireName && port.Name == "" {
			errs = append(errs, field.Required(path.Child("name"), ""))
		} else if port.Name != "" {
			if names[port.Name] {
				errs = append(errs, field.Duplicate(path.Child("name"), port.Name))
			}
			names[port.Name] = true
			for _, msg := range validation.IsDNS1123Label(port.Name) {
				errs = append(errs, field.Invalid(path.Child("name"), port.Name, msg))
			}
		}
		if port.Port < 1 || port.Port > 65535 {
			errs = append(errs, field.Invalid(path.Child("port"), port.Port, "must be between 1 and 65535, inclusive"))
		}
		switch port.Protocol {
		case "", corev1.ProtocolTCP, corev1.ProtocolUDP, corev1.ProtocolSCTP:
		default:
			errs = append(errs, field.NotSupported(path.Child("protocol"), port.Protocol, []string{"SCTP", "TCP", "UDP"}))
		}
		switch port.TargetPort.Type {
		case intstr.Int:
			if port.TargetPort.IntVal != 0 && (port.TargetPort.IntVal < 1 || port.TargetPort.IntVal > 65535) {
				errs = append(errs, field.Invalid(path.Child("targetPort"), port.TargetPort.IntVal, "must be between 1 and 65535, inclusive"))
			}
		case intstr.String:
			if port.TargetPort.StrVal != "" {
				for _, msg := range validation.IsDNS1123Label(port.TargetPort.StrVal) {
					errs = append(errs, field.Invalid(path.Child("targetPort"), port.TargetPort.StrVal, msg))
				}
			}
		}
		if port.AppProtocol != nil {
			for _, msg := range validation.IsQualifiedName(*port.AppProtocol) {
				errs = append(errs, field.Invalid(path.Child("appProtocol"), *port.AppProtocol, msg))
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, errs)
}

func externalNameAllowed(svc, old *corev1.Service) error {
	if svc == nil || svc.Spec.Type != corev1.ServiceTypeExternalName {
		return nil
	}
	var errs field.ErrorList
	if strings.TrimSuffix(svc.Spec.ExternalName, ".") == "" {
		errs = append(errs, field.Required(field.NewPath("spec", "externalName"), ""))
	} else {
		cname := strings.TrimSuffix(svc.Spec.ExternalName, ".")
		for _, msg := range validation.IsDNS1123Subdomain(cname) {
			errs = append(errs, field.Invalid(field.NewPath("spec", "externalName"), svc.Spec.ExternalName, msg))
		}
	}
	carried := old != nil && old.Spec.Type != corev1.ServiceTypeExternalName
	if len(svc.Spec.IPFamilies) > 0 && !(carried && sameIPFamilies(old, svc)) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "ipFamilies"), "may not be set for ExternalName services"))
	}
	if svc.Spec.IPFamilyPolicy != nil && !(carried && sameIPFamilyPolicy(old, svc)) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "ipFamilyPolicy"), "may not be set for ExternalName services"))
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, errs)
}

func servicePortsRequired(svc *corev1.Service) error {
	if svc == nil || len(svc.Spec.Ports) > 0 || headlessService(svc) || svc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
		field.Required(field.NewPath("spec", "ports"), ""),
	})
}

func sessionAffinityAllowed(svc *corev1.Service) error {
	if svc == nil || svc.Spec.SessionAffinity == "" {
		return nil
	}
	switch svc.Spec.SessionAffinity {
	case corev1.ServiceAffinityNone, corev1.ServiceAffinityClientIP:
	default:
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.NotSupported(field.NewPath("spec", "sessionAffinity"), svc.Spec.SessionAffinity, []string{string(corev1.ServiceAffinityClientIP), string(corev1.ServiceAffinityNone)}),
		})
	}
	cfg := svc.Spec.SessionAffinityConfig
	if svc.Spec.SessionAffinity != corev1.ServiceAffinityClientIP || cfg == nil || cfg.ClientIP == nil || cfg.ClientIP.TimeoutSeconds == nil {
		return nil
	}
	timeout := *cfg.ClientIP.TimeoutSeconds
	if timeout > 0 && timeout <= maxClientIPServiceAffinitySeconds {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
		field.Invalid(field.NewPath("spec", "sessionAffinityConfig", "clientIP", "timeoutSeconds"), timeout, fmt.Sprintf("must be greater than 0 and less than %d", maxClientIPServiceAffinitySeconds)),
	})
}

const maxClientIPServiceAffinitySeconds int32 = 86400

func loadBalancerSourceRangesAllowed(svc, old *corev1.Service) error {
	if svc == nil {
		return nil
	}
	var errs field.ErrorList
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		if len(svc.Spec.LoadBalancerSourceRanges) > 0 {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "LoadBalancerSourceRanges"), "may only be used when `type` is 'LoadBalancer'"))
		}
		if _, ok := svc.Annotations[corev1.AnnotationLoadBalancerSourceRangesKey]; ok {
			errs = append(errs, field.Forbidden(field.NewPath("metadata", "annotations").Key(corev1.AnnotationLoadBalancerSourceRangesKey), "may only be used when `type` is 'LoadBalancer'"))
		}
	} else {
		var existing []string
		if old != nil {
			for _, value := range old.Spec.LoadBalancerSourceRanges {
				existing = append(existing, strings.TrimSpace(value))
			}
		}
		for i, value := range svc.Spec.LoadBalancerSourceRanges {
			errs = append(errs, validation.IsValidCIDRForLegacyField(field.NewPath("spec", "LoadBalancerSourceRanges").Index(i), strings.TrimSpace(value), false, existing)...)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, errs)
}

func duplicateServicePorts(svc *corev1.Service) error {
	if svc == nil {
		return nil
	}
	var errs field.ErrorList
	nodePorts := map[corev1.ServicePort]bool{}
	ports := map[corev1.ServicePort]bool{}
	for i, port := range svc.Spec.Ports {
		portPath := field.NewPath("spec", "ports").Index(i)
		if port.NodePort != 0 {
			key := corev1.ServicePort{Protocol: port.Protocol, NodePort: port.NodePort}
			if nodePorts[key] {
				errs = append(errs, field.Duplicate(portPath.Child("nodePort"), port.NodePort))
			}
			nodePorts[key] = true
		}
		key := corev1.ServicePort{Protocol: port.Protocol, Port: port.Port}
		if ports[key] {
			errs = append(errs, field.Duplicate(portPath, key))
		}
		ports[key] = true
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, errs)
}

func clusterIPNodePortAllowed(svc, old *corev1.Service) error {
	if svc == nil || svc.Spec.Type != corev1.ServiceTypeClusterIP {
		return nil
	}
	if old != nil && needsNodePorts(old) && sameNodePorts(old, svc) {
		return nil
	}
	var errs field.ErrorList
	for i := range svc.Spec.Ports {
		if svc.Spec.Ports[i].NodePort != 0 {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "ports").Index(i).Child("nodePort"), "may not be used when `type` is 'ClusterIP'"))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, errs)
}

func clusterIPNoneAllowed(svc *corev1.Service) error {
	if svc == nil || !headlessService(svc) {
		return nil
	}
	var msg string
	switch svc.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		msg = "may not be set to 'None' for LoadBalancer services"
	case corev1.ServiceTypeNodePort:
		msg = "may not be set to 'None' for NodePort services"
	default:
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
		field.Invalid(field.NewPath("spec", "clusterIPs").Index(0), corev1.ClusterIPNone, msg),
	})
}

func clusterIPSliceAgrees(svc *corev1.Service) error {
	if svc == nil || svc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}
	if svc.Spec.ClusterIP == "" {
		if len(svc.Spec.ClusterIPs) == 0 {
			return nil
		}
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.Invalid(field.NewPath("spec", "clusterIPs"), svc.Spec.ClusterIPs, "must be empty when `clusterIP` is not specified"),
		})
	}
	if len(svc.Spec.ClusterIPs) == 0 {
		return nil
	}
	if svc.Spec.ClusterIPs[0] != svc.Spec.ClusterIP {
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.Invalid(field.NewPath("spec", "clusterIPs"), svc.Spec.ClusterIPs, "first value must match `clusterIP`"),
		})
	}
	if svc.Spec.ClusterIPs[0] == corev1.ClusterIPNone && len(svc.Spec.ClusterIPs) > 1 {
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.Invalid(field.NewPath("spec", "clusterIPs"), svc.Spec.ClusterIPs, "'None' must be the first and only value"),
		})
	}
	return nil
}

func immutableIPFamilies(newSvc, oldSvc *corev1.Service) error {
	if newSvc == nil || oldSvc == nil {
		return nil
	}
	if newSvc.Spec.Type == corev1.ServiceTypeExternalName || oldSvc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}
	if headlessService(newSvc) != headlessService(oldSvc) || headlessService(newSvc) {
		return nil
	}
	var errs field.ErrorList
	switch {
	case len(oldSvc.Spec.IPFamilies) == len(newSvc.Spec.IPFamilies):
		for i, family := range oldSvc.Spec.IPFamilies {
			if family != newSvc.Spec.IPFamilies[i] {
				errs = append(errs, field.Invalid(field.NewPath("spec", "ipFamilies").Index(0), newSvc.Spec.IPFamilies, "may not change once set"))
			}
		}
	case len(oldSvc.Spec.IPFamilies) > len(newSvc.Spec.IPFamilies):
		if len(newSvc.Spec.ClusterIPs) == 0 {
			errs = append(errs, field.Invalid(field.NewPath("spec", "ipFamilies").Index(0), newSvc.Spec.IPFamilies, "primary ipFamily can not be unset"))
		}
		if len(newSvc.Spec.IPFamilies) > 0 && len(oldSvc.Spec.IPFamilies) > 0 && newSvc.Spec.IPFamilies[0] != oldSvc.Spec.IPFamilies[0] {
			errs = append(errs, field.Invalid(field.NewPath("spec", "ipFamilies").Index(0), newSvc.Spec.ClusterIPs, "may not change once set"))
		}
		if len(newSvc.Spec.IPFamilies) == 1 && (newSvc.Spec.IPFamilyPolicy == nil || *newSvc.Spec.IPFamilyPolicy != corev1.IPFamilyPolicySingleStack) {
			errs = append(errs, field.Invalid(field.NewPath("spec", "ipFamilyPolicy"), newSvc.Spec.IPFamilyPolicy, "must be set to 'SingleStack' when releasing the secondary ipFamily"))
		}
	case len(oldSvc.Spec.IPFamilies) > 0 && len(newSvc.Spec.IPFamilies) > 0 && newSvc.Spec.IPFamilies[0] != oldSvc.Spec.IPFamilies[0]:
		errs = append(errs, field.Invalid(field.NewPath("spec", "ipFamilies").Index(0), newSvc.Spec.ClusterIPs, "may not change once set"))
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), newSvc.Name, errs)
}

func headlessService(svc *corev1.Service) bool {
	return svc.Spec.ClusterIP == corev1.ClusterIPNone || (len(svc.Spec.ClusterIPs) > 0 && svc.Spec.ClusterIPs[0] == corev1.ClusterIPNone)
}

func immutableClusterIP(newSvc, oldSvc *corev1.Service) error {
	if newSvc == nil || oldSvc == nil || oldSvc.Spec.ClusterIP == "" || newSvc.Spec.ClusterIP == oldSvc.Spec.ClusterIP {
		return nil
	}
	if newSvc.Spec.Type == corev1.ServiceTypeExternalName || oldSvc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}
	return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), newSvc.Name, field.ErrorList{
		field.Invalid(field.NewPath("spec", "clusterIP"), newSvc.Spec.ClusterIP, "field is immutable"),
	})
}

func serviceOf(obj runtime.Object) *corev1.Service {
	svc, _ := obj.(*corev1.Service)
	return svc
}

const clusterIPAttempts = 8

func claimPickedClusterIP(ctx context.Context, deps registry.Deps, svc *corev1.Service, picked bool) (genericregistry.FinishFunc, error) {
	for attempt := 1; ; attempt++ {
		finish, err := finishServiceIPs(ctx, deps, nil, svc)
		var allocated ipAllocatedError
		if err == nil || !picked || attempt >= clusterIPAttempts || !errors.As(err, &allocated) {
			return finish, err
		}
		ip, err := freeClusterIP(ctx, deps)
		if err != nil {
			return nil, err
		}
		svc.Spec.ClusterIP = ip
		svc.Spec.ClusterIPs = []string{ip}
	}
}

type ipAllocatedError string

func (e ipAllocatedError) Error() string { return "ipaddress " + string(e) + " is already allocated" }

func finishServiceIPs(ctx context.Context, deps registry.Deps, old, next *corev1.Service) (genericregistry.FinishFunc, error) {
	previous := serviceIPs(old)
	current := serviceIPs(next)
	added := addedIPs(previous, current)
	if err := claimServiceIPs(ctx, deps, next, added); err != nil {
		return nil, err
	}
	removed := addedIPs(current, previous)
	return func(_ context.Context, success bool) {
		if success {
			releaseIPs(context.Background(), deps, removed)
			return
		}
		releaseIPs(context.Background(), deps, ipsAbsentFromService(deps, next, added))
	}, nil
}

func ipsAbsentFromService(deps registry.Deps, svc *corev1.Service, ips []string) []string {
	var out []string
	for _, ip := range ips {
		if persistedServiceHasIP(context.Background(), deps, svc, ip) {
			continue
		}
		out = append(out, ip)
	}
	return out
}

func persistedServiceHasIP(ctx context.Context, deps registry.Deps, svc *corev1.Service, ip string) bool {
	if deps.Kine == nil || svc == nil || ip == "" {
		return false
	}
	kv, _, err := deps.Kine.Get(ctx, "/registry/services/"+svc.Namespace+"/"+svc.Name)
	if err != nil || kv == nil {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		raw = []byte(kv.Value)
	}
	return bytes.Contains(raw, []byte(ip))
}

func serviceIPs(svc *corev1.Service) []string {
	if svc == nil || svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == corev1.ClusterIPNone {
		return nil
	}
	if len(svc.Spec.ClusterIPs) == 0 {
		return []string{svc.Spec.ClusterIP}
	}
	var out []string
	for _, ip := range svc.Spec.ClusterIPs {
		if ip != "" && ip != corev1.ClusterIPNone {
			out = append(out, ip)
		}
	}
	return out
}

func addedIPs(old, next []string) []string {
	have := map[string]bool{}
	for _, ip := range old {
		have[ip] = true
	}
	var out []string
	for _, ip := range next {
		if !have[ip] {
			out = append(out, ip)
		}
	}
	return out
}

func claimServiceIPs(ctx context.Context, deps registry.Deps, svc *corev1.Service, ips []string) error {
	if deps.Kine == nil || svc == nil {
		return nil
	}
	for _, ip := range ips {
		name := ipAddressKeyName(ip)
		obj := &networkingv1.IPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: name, UID: uuid.NewUUID(), CreationTimestamp: metav1.Now()},
			Spec: networkingv1.IPAddressSpec{ParentRef: &networkingv1.ParentReference{
				Resource:  "services",
				Namespace: svc.Namespace,
				Name:      svc.Name,
			}},
		}
		data, err := runtime.Encode(ipCodec, obj)
		if err != nil {
			return err
		}
		if _, err := deps.Kine.Put(ctx, "/registry/ipaddresses/"+name, data, 0); err != nil {
			if err == kine.ErrConflict && ipOwnedBy(ctx, deps, name, svc) {
				if err := ensureIPIdentity(ctx, deps, name); err != nil {
					return err
				}
				continue
			}
			if err == kine.ErrConflict {
				return ipAllocatedError(name)
			}
			releaseIPs(ctx, deps, ips)
			return err
		}
	}
	return nil
}

func ensureIPIdentity(ctx context.Context, deps registry.Deps, name string) error {
	kv, _, err := deps.Kine.Get(ctx, "/registry/ipaddresses/"+name)
	if err != nil || kv == nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return err
	}
	obj, _, err := ipCodec.Decode(raw, nil, &networkingv1.IPAddress{})
	if err != nil {
		return err
	}
	ip, ok := obj.(*networkingv1.IPAddress)
	if !ok {
		return fmt.Errorf("ipaddress %s has an unexpected type", name)
	}
	if ip.UID != "" && !ip.CreationTimestamp.IsZero() {
		return nil
	}
	if ip.UID == "" {
		ip.UID = uuid.NewUUID()
	}
	if ip.CreationTimestamp.IsZero() {
		ip.CreationTimestamp = metav1.Now()
	}
	data, err := runtime.Encode(ipCodec, ip)
	if err != nil {
		return err
	}
	_, err = deps.Kine.Put(ctx, "/registry/ipaddresses/"+name, data, kv.ModRevision)
	return err
}

func ipOwnedBy(ctx context.Context, deps registry.Deps, name string, svc *corev1.Service) bool {
	kv, _, err := deps.Kine.Get(ctx, "/registry/ipaddresses/"+name)
	if err != nil || kv == nil {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return false
	}
	obj, _, err := ipCodec.Decode(raw, nil, &networkingv1.IPAddress{})
	if err != nil {
		return false
	}
	ip, ok := obj.(*networkingv1.IPAddress)
	if !ok || ip.Spec.ParentRef == nil {
		return false
	}
	ref := ip.Spec.ParentRef
	return ref.Resource == "services" && ref.Namespace == svc.Namespace && ref.Name == svc.Name
}

func releaseServiceIPs(ctx context.Context, deps registry.Deps, svc *corev1.Service) {
	releaseIPs(ctx, deps, serviceIPs(svc))
}

func releaseIPs(ctx context.Context, deps registry.Deps, ips []string) {
	if deps.Kine == nil {
		return
	}
	for _, ip := range ips {
		name := ipAddressKeyName(ip)
		if _, err := deps.Kine.Delete(ctx, "/registry/ipaddresses/"+name, 0); err != nil && err != kine.ErrNotFound {
			println("services: release ipaddress", name, err.Error())
		}
	}
}

func ipAddressKeyName(ip string) string {
	if parsed := net.ParseIP(ip); parsed != nil {
		return strings.ReplaceAll(parsed.String(), ":", "-")
	}
	return strings.ReplaceAll(ip, ":", "-")
}

func defaultHeadlessClusterIPs(svc *corev1.Service) {
	if svc.Spec.ClusterIP == corev1.ClusterIPNone && len(svc.Spec.ClusterIPs) == 0 {
		svc.Spec.ClusterIPs = []string{corev1.ClusterIPNone}
	}
}

func releaseExternalName(svc *corev1.Service) bool {
	if svc.Spec.Type != corev1.ServiceTypeExternalName {
		return false
	}
	svc.Spec.ClusterIP = ""
	svc.Spec.ClusterIPs = nil
	svc.Spec.HealthCheckNodePort = 0
	for i := range svc.Spec.Ports {
		svc.Spec.Ports[i].NodePort = 0
	}
	return true
}

func specifiedClusterIPAllowed(svc *corev1.Service) error {
	ip := svc.Spec.ClusterIP
	parsed := net.ParseIP(ip)
	cidr := supervisor.ServiceCIDR
	base := cidr.IP.Mask(cidr.Mask)
	if parsed == nil || base == nil || !cidr.Contains(parsed) || parsed.Equal(base) {
		return apierrors.NewInvalid(corev1.SchemeGroupVersion.WithKind("Service").GroupKind(), svc.Name, field.ErrorList{
			field.Invalid(field.NewPath("spec", "clusterIP"), ip, fmt.Sprintf("provided IP is not in the valid range. The range of valid IPs is %s", cidr)),
		})
	}
	return nil
}

func defaultServiceIPFamily(svc *corev1.Service) {
	if svc.Spec.ClusterIP == corev1.ClusterIPNone {
		return
	}
	if len(svc.Spec.IPFamilies) == 0 {
		svc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
	}
	if svc.Spec.IPFamilyPolicy != nil {
		return
	}
	policy := corev1.IPFamilyPolicySingleStack
	if len(svc.Spec.IPFamilies) > 1 {
		policy = corev1.IPFamilyPolicyRequireDualStack
	}
	svc.Spec.IPFamilyPolicy = &policy
}

func needsClusterIP(svc *corev1.Service) bool {
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return false
	}
	if svc.Spec.ClusterIP == corev1.ClusterIPNone {
		return false
	}
	return svc.Spec.ClusterIP == ""
}

func freeClusterIP(ctx context.Context, deps registry.Deps) (string, error) {
	taken := reservedClusterIPs()
	kvs, _, _, err := deps.Kine.List(ctx, serviceStoragePrefix, "", 0)
	if err != nil {
		return "", err
	}
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		var other corev1.Service
		if json.Unmarshal(data, &other) != nil {
			continue
		}
		if other.Spec.ClusterIP != "" && other.Spec.ClusterIP != corev1.ClusterIPNone {
			taken[other.Spec.ClusterIP] = true
		}
	}
	addrs, _, _, err := deps.Kine.List(ctx, "/registry/ipaddresses/", "", 0)
	if err != nil {
		return "", err
	}
	for _, kv := range addrs {
		if ip, ok := ipFromAddressKey(kv.Key); ok {
			taken[ip] = true
		}
	}
	return pickClusterIP(taken, rand.Int())
}

func ipFromAddressKey(key string) (string, bool) {
	const prefix = "/registry/ipaddresses/"
	name := strings.TrimPrefix(key, prefix)
	if name == "" || name == key || strings.Contains(name, "/") {
		return "", false
	}
	if strings.Contains(name, ":") {
		return "", false
	}
	ip := net.ParseIP(strings.ReplaceAll(name, "-", ":"))
	if ip == nil {
		return "", false
	}
	return ip.String(), true
}

func reservedClusterIPs() map[string]bool {
	base := supervisor.ServiceCIDR.IP.Mask(supervisor.ServiceCIDR.Mask).To4()
	out := map[string]bool{}
	if base == nil {
		return out
	}
	out[base.String()] = true
	out[net.IPv4(base[0], base[1], base[2], 1).String()] = true
	out[net.IPv4(base[0], base[1], base[2], 10).String()] = true
	return out
}

func pickClusterIP(taken map[string]bool, start int) (string, error) {
	cidr := supervisor.ServiceCIDR
	base := cidr.IP.Mask(cidr.Mask).To4()
	if base == nil {
		return "", fmt.Errorf("service cidr %s is not ipv4", cidr)
	}
	ones, bits := cidr.Mask.Size()
	n := 1 << uint(bits-ones)
	for step := 0; step < n-1; step++ {
		i := 1 + (start%(n-1)+step)%(n-1)
		ip := net.IPv4(base[0], base[1], byte(i>>8), byte(i))
		if !cidr.Contains(ip) {
			continue
		}
		s := ip.String()
		if !taken[s] {
			return s, nil
		}
	}
	return "", fmt.Errorf("no free address in %s", cidr)
}
