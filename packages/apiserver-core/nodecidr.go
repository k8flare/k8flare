package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
)

const (
	nodeStoragePrefix = "/registry/nodes/"
	podCIDRMaskSize   = 24
)

func init() {
	registry.Customizers["nodes"] = func(store *registry.Store, deps registry.Deps) {
		assign := func(ctx context.Context, obj runtime.Object) {
			node, ok := obj.(*corev1.Node)
			if !ok || node.Spec.PodCIDR != "" {
				return
			}
			cidr, err := freePodCIDR(ctx, deps, node.Name)
			if err != nil {
				println("nodes: pod cidr allocation failed:", err.Error())
				return
			}
			node.Spec.PodCIDR = cidr
			node.Spec.PodCIDRs = []string{cidr}
		}
		store.BeginCreate = func(ctx context.Context, obj runtime.Object, _ *metav1.CreateOptions) (genericregistry.FinishFunc, error) {
			assign(ctx, obj)
			return func(context.Context, bool) {}, nil
		}
		store.BeginUpdate = func(ctx context.Context, obj, _ runtime.Object, _ *metav1.UpdateOptions) (genericregistry.FinishFunc, error) {
			assign(ctx, obj)
			return func(context.Context, bool) {}, nil
		}
	}
}

func freePodCIDR(ctx context.Context, deps registry.Deps, name string) (string, error) {
	taken := map[string]bool{}
	kvs, _, _, err := deps.Kine.List(ctx, nodeStoragePrefix, "", 0)
	if err != nil {
		return "", err
	}
	for _, kv := range kvs {
		if strings.TrimPrefix(kv.Key, nodeStoragePrefix) == name {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		var other corev1.Node
		if json.Unmarshal(data, &other) != nil {
			continue
		}
		if other.Spec.PodCIDR != "" {
			taken[other.Spec.PodCIDR] = true
		}
	}
	base := supervisor.ClusterCIDR.IP.Mask(supervisor.ClusterCIDR.Mask).To4()
	if base == nil {
		return "", fmt.Errorf("cluster cidr %s is not ipv4", supervisor.ClusterCIDR)
	}
	for block := 0; block < 256; block++ {
		ip := net.IPv4(base[0], base[1], byte(block), 0)
		candidate := fmt.Sprintf("%s/%d", ip.String(), podCIDRMaskSize)
		if !taken[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free /%d inside %s", podCIDRMaskSize, supervisor.ClusterCIDR)
}
