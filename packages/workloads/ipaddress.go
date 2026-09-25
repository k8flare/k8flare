package workloads

import (
	"context"
	"net"
	"strings"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ensureServiceIPAddresses(ctx context.Context, client kubernetes.Interface, changed []string) error {
	if len(changed) > 0 && !changedHas(changed, "services") && !changedHas(changed, "ipaddresses") {
		return nil
	}
	svcs, err := client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	want := map[string]networkingv1.ParentReference{}
	if svcs != nil {
		for i := range svcs.Items {
			for _, ip := range serviceClusterIPs(&svcs.Items[i]) {
				want[ipAddressName(ip)] = networkingv1.ParentReference{
					Group:     "",
					Resource:  "services",
					Namespace: svcs.Items[i].Namespace,
					Name:      svcs.Items[i].Name,
				}
			}
		}
	}
	have, err := client.NetworkingV1().IPAddresses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	seen := map[string]bool{}
	if have != nil {
		for i := range have.Items {
			addr := &have.Items[i]
			seen[addr.Name] = true
			parent, ok := want[addr.Name]
			if ok && ipParentMatches(addr.Spec.ParentRef, parent) {
				continue
			}
			if addr.Spec.ParentRef != nil && addr.Spec.ParentRef.Resource != "services" {
				continue
			}
			if err := client.NetworkingV1().IPAddresses().Delete(ctx, addr.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if !ok {
				continue
			}
			seen[addr.Name] = false
		}
	}
	for name, parent := range want {
		if seen[name] {
			continue
		}
		obj := &networkingv1.IPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       networkingv1.IPAddressSpec{ParentRef: &parent},
		}
		if _, err := client.NetworkingV1().IPAddresses().Create(ctx, obj, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func serviceClusterIPs(svc *v1.Service) []string {
	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == v1.ClusterIPNone {
		return nil
	}
	if len(svc.Spec.ClusterIPs) == 0 {
		return []string{svc.Spec.ClusterIP}
	}
	var out []string
	for _, ip := range svc.Spec.ClusterIPs {
		if ip != "" && ip != v1.ClusterIPNone {
			out = append(out, ip)
		}
	}
	return out
}

func ipAddressName(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return strings.ReplaceAll(ip, ":", "-")
	}
	return strings.ReplaceAll(parsed.String(), ":", "-")
}

func ipParentMatches(got *networkingv1.ParentReference, want networkingv1.ParentReference) bool {
	if got == nil {
		return false
	}
	return got.Group == want.Group && got.Resource == want.Resource && got.Namespace == want.Namespace && got.Name == want.Name
}
