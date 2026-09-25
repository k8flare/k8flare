package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	nodePortMin int32 = 30000
	nodePortMax int32 = 32767
)

func needsNodePorts(svc *corev1.Service) bool {
	switch svc.Spec.Type {
	case corev1.ServiceTypeNodePort:
		return true
	case corev1.ServiceTypeLoadBalancer:
		return svc.Spec.AllocateLoadBalancerNodePorts == nil || *svc.Spec.AllocateLoadBalancerNodePorts
	default:
		return false
	}
}

func assignNodePorts(ctx context.Context, svc *corev1.Service, deps registry.Deps) error {
	taken := usedNodePorts(ctx, deps, svc)
	if needsNodePorts(svc) {
		if err := allocateNodePorts(svc, taken); err != nil {
			return err
		}
	}
	if !needsHealthCheckNodePort(svc) {
		return nil
	}
	return allocateHealthCheckNodePort(svc, taken)
}

func allocateHealthCheckNodePort(svc *corev1.Service, taken map[int32]bool) error {
	if svc.Spec.HealthCheckNodePort == 0 {
		np, ok := pickNodePort(taken)
		if !ok {
			return apierrors.NewConflict(schema.GroupResource{Resource: "services"}, svc.Name, fmt.Errorf("no free node port"))
		}
		svc.Spec.HealthCheckNodePort = np
		taken[np] = true
		return nil
	}
	p := svc.Spec.HealthCheckNodePort
	if p < nodePortMin || p > nodePortMax {
		return apierrors.NewInvalid(schema.GroupKind{Kind: "Service"}, svc.Name, field.ErrorList{
			field.Invalid(field.NewPath("spec", "healthCheckNodePort"), p, "port is not in the valid range"),
		})
	}
	if taken[p] {
		return apierrors.NewConflict(schema.GroupResource{Resource: "services"}, svc.Name, fmt.Errorf("provided port is already allocated"))
	}
	taken[p] = true
	return nil
}

func allocateNodePorts(svc *corev1.Service, taken map[int32]bool) error {
	samePort := map[int32]int32{}
	for i := range svc.Spec.Ports {
		p := &svc.Spec.Ports[i]
		if p.NodePort == 0 {
			continue
		}
		if p.NodePort < nodePortMin || p.NodePort > nodePortMax {
			return apierrors.NewInvalid(schema.GroupKind{Kind: "Service"}, svc.Name, field.ErrorList{
				field.Invalid(field.NewPath("spec", "ports").Index(i).Child("nodePort"), p.NodePort, "port is not in the valid range"),
			})
		}
		if taken[p.NodePort] {
			return apierrors.NewConflict(schema.GroupResource{Resource: "services"}, svc.Name, fmt.Errorf("provided port is already allocated"))
		}
		taken[p.NodePort] = true
		samePort[p.Port] = p.NodePort
	}
	for i := range svc.Spec.Ports {
		p := &svc.Spec.Ports[i]
		if p.NodePort != 0 {
			continue
		}
		if existing, ok := samePort[p.Port]; ok {
			p.NodePort = existing
			continue
		}
		np, ok := pickNodePort(taken)
		if !ok {
			return apierrors.NewConflict(schema.GroupResource{Resource: "services"}, svc.Name, fmt.Errorf("no free node port"))
		}
		p.NodePort = np
		taken[np] = true
		samePort[p.Port] = np
	}
	return nil
}

func usedNodePorts(ctx context.Context, deps registry.Deps, self *corev1.Service) map[int32]bool {
	taken := map[int32]bool{}
	if deps.Kine == nil {
		return taken
	}
	kvs, _, _, err := deps.Kine.List(ctx, serviceStoragePrefix, "", 0)
	if err != nil {
		return taken
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
		if self != nil && other.Name == self.Name && other.Namespace == self.Namespace {
			continue
		}
		for _, p := range other.Spec.Ports {
			if p.NodePort != 0 {
				taken[p.NodePort] = true
			}
		}
		if other.Spec.HealthCheckNodePort != 0 {
			taken[other.Spec.HealthCheckNodePort] = true
		}
	}
	return taken
}

func pickNodePort(taken map[int32]bool) (int32, bool) {
	for p := nodePortMin; p <= nodePortMax; p++ {
		if !taken[p] {
			return p, true
		}
	}
	return 0, false
}
