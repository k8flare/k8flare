package admission

import (
	"context"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func applyServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "pods" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	name, _ := spec["serviceAccountName"].(string)
	if name == "" {
		name = "default"
		spec["serviceAccountName"] = name
	}
	var sa corev1.ServiceAccount
	var saOK bool
	if s != nil && req.Namespace != "" {
		got, ok, err := podServiceAccount(ctx, s, req.Namespace, name)
		if err != nil {
			return err
		}
		if ok {
			sa, saOK = got, true
			if _, hasPulls := spec["imagePullSecrets"]; !hasPulls && len(sa.ImagePullSecrets) > 0 {
				pulls := make([]any, 0, len(sa.ImagePullSecrets))
				for _, ref := range sa.ImagePullSecrets {
					pulls = append(pulls, map[string]any{"name": ref.Name})
				}
				spec["imagePullSecrets"] = pulls
			}
		}
	}
	if automount, ok := spec["automountServiceAccountToken"].(bool); ok && !automount {
		return nil
	}
	if _, podSet := spec["automountServiceAccountToken"].(bool); !podSet && saOK && sa.AutomountServiceAccountToken != nil && !*sa.AutomountServiceAccountToken {
		return nil
	}
	const volName = "kube-api-access"
	const mountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
	volumes, _ := spec["volumes"].([]any)
	for _, raw := range volumes {
		v, _ := raw.(map[string]any)
		if v != nil && v["name"] == volName {
			return nil
		}
	}
	spec["volumes"] = append(volumes, map[string]any{
		"name": volName,
		"projected": map[string]any{
			"defaultMode": int64(420),
			"sources": []any{
				map[string]any{"serviceAccountToken": map[string]any{"expirationSeconds": int64(3607), "path": "token"}},
				map[string]any{"configMap": map[string]any{"name": "kube-root-ca.crt", "items": []any{map[string]any{"key": "ca.crt", "path": "ca.crt"}}}},
				map[string]any{"downwardAPI": map[string]any{"items": []any{map[string]any{"path": "namespace", "fieldRef": map[string]any{"apiVersion": "v1", "fieldPath": "metadata.namespace"}}}}},
			},
		},
	})
	mountSAToken(spec, "containers", volName, mountPath)
	mountSAToken(spec, "initContainers", volName, mountPath)
	return nil
}

func validateServiceAccount(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "pods" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	meta, _ := req.Object["metadata"].(map[string]any)
	annotations, _ := meta["annotations"].(map[string]any)
	if _, mirror := annotations[mirrorPodAnnotationKey]; mirror {
		return nil
	}
	name, _ := spec["serviceAccountName"].(string)
	if name == "" {
		podName, _ := meta["name"].(string)
		return fmt.Errorf("no service account specified for pod %s/%s", req.Namespace, podName)
	}
	if s == nil || req.Namespace == "" {
		return nil
	}
	_, _, err := podServiceAccount(ctx, s, req.Namespace, name)
	return err
}

func podServiceAccount(ctx context.Context, s *store, namespace, name string) (corev1.ServiceAccount, bool, error) {
	sa, ok, err := s.serviceAccount(ctx, namespace, name)
	if err != nil {
		return corev1.ServiceAccount{}, false, fmt.Errorf("error looking up service account %s/%s: %w", namespace, name, err)
	}
	if !ok && name != "default" {
		return corev1.ServiceAccount{}, false, fmt.Errorf("error looking up service account %s/%s: serviceaccount %q not found", namespace, name, name)
	}
	return sa, ok, nil
}

func mountSAToken(spec map[string]any, field, volName, mountPath string) {
	list, _ := spec[field].([]any)
	for _, raw := range list {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		mounts, _ := c["volumeMounts"].([]any)
		already := false
		for _, m := range mounts {
			mm, _ := m.(map[string]any)
			if mm != nil && mm["mountPath"] == mountPath {
				already = true
				break
			}
		}
		if already {
			continue
		}
		c["volumeMounts"] = append(mounts, map[string]any{"name": volName, "mountPath": mountPath, "readOnly": true})
	}
}

func applyLimitRanger(ctx context.Context, s *store, req *admit.Request) error {
	if !limitRangerSupports(req) || req.Resource.Resource != "pods" {
		return nil
	}
	ranges, err := s.limitRanges(ctx, req.Namespace)
	if err != nil {
		return err
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	for _, lr := range ranges {
		for _, item := range lr.Spec.Limits {
			if item.Type == corev1.LimitTypeContainer {
				applyContainerDefaults(spec, "containers", item)
				applyContainerDefaults(spec, "initContainers", item)
			}
		}
	}
	return nil
}

func validateLimitRanger(ctx context.Context, s *store, req *admit.Request) error {
	if !limitRangerSupports(req) {
		return nil
	}
	ranges, err := s.limitRanges(ctx, req.Namespace)
	if err != nil {
		return err
	}
	if req.Resource.Resource == "persistentvolumeclaims" {
		return validatePVCLimitRange(ranges, req)
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	for _, lr := range ranges {
		for _, item := range lr.Spec.Limits {
			switch item.Type {
			case corev1.LimitTypeContainer:
				if err := checkContainerLimits(spec, item); err != nil {
					return err
				}
			case corev1.LimitTypePod:
				if err := checkPodLimits(spec, item); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func limitRangerSupports(req *admit.Request) bool {
	if req.Object == nil || req.Namespace == "" {
		return false
	}
	if req.Operation != "" && req.Operation != "CREATE" && req.Operation != "UPDATE" {
		return false
	}
	oldMeta, _ := req.OldObject["metadata"].(map[string]any)
	if oldMeta["deletionTimestamp"] != nil {
		return false
	}
	pod := req.Resource.Resource == "pods"
	if pod && req.Subresource == "resize" && req.Operation == "UPDATE" {
		return true
	}
	if req.Subresource != "" {
		return false
	}
	if pod && req.Operation == "UPDATE" {
		return false
	}
	return pod || req.Resource.Resource == "persistentvolumeclaims"
}

func validatePVCLimitRange(ranges []corev1.LimitRange, req *admit.Request) error {
	for _, lr := range ranges {
		for _, item := range lr.Spec.Limits {
			if item.Type != corev1.LimitTypePersistentVolumeClaim {
				continue
			}
			for name, min := range item.Min {
				q, ok := pvcResourceRequest(req.Object, string(name))
				if !ok {
					return fmt.Errorf("minimum %s usage per PersistentVolumeClaim is %s.  No request is specified", name, min.String())
				}
				if q.Cmp(min) < 0 {
					return fmt.Errorf("minimum %s usage per PersistentVolumeClaim is %s, but request is %s", name, min.String(), q.String())
				}
			}
			for name, max := range item.Max {
				q, ok := pvcResourceRequest(req.Object, string(name))
				if !ok {
					return fmt.Errorf("maximum %s usage per PersistentVolumeClaim is %s.  No request is specified", name, max.String())
				}
				if q.Cmp(max) > 0 {
					return fmt.Errorf("maximum %s usage per PersistentVolumeClaim is %s, but request is %s", name, max.String(), q.String())
				}
			}
		}
	}
	return nil
}

func pvcResourceRequest(obj map[string]any, name string) (resource.Quantity, bool) {
	spec, _ := obj["spec"].(map[string]any)
	resources, _ := spec["resources"].(map[string]any)
	requests, _ := resources["requests"].(map[string]any)
	raw, ok := requests[name]
	if !ok {
		return resource.Quantity{}, false
	}
	s, _ := raw.(string)
	if s == "" {
		return resource.Quantity{}, false
	}
	q, err := resource.ParseQuantity(s)
	return q, err == nil
}

func applyContainerDefaults(spec map[string]any, field string, item corev1.LimitRangeItem) {
	list, _ := spec[field].([]any)
	for _, raw := range list {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		resources, _ := c["resources"].(map[string]any)
		if resources == nil {
			resources = map[string]any{}
			c["resources"] = resources
		}
		limits, _ := resources["limits"].(map[string]any)
		if limits == nil {
			limits = map[string]any{}
		}
		requests, _ := resources["requests"].(map[string]any)
		if requests == nil {
			requests = map[string]any{}
		}
		for name, qty := range item.Default {
			if _, ok := limits[string(name)]; !ok {
				limits[string(name)] = qty.String()
			}
		}
		for name, qty := range item.DefaultRequest {
			if _, ok := requests[string(name)]; !ok {
				requests[string(name)] = qty.String()
			}
		}
		for name := range item.Default {
			if _, ok := requests[string(name)]; !ok {
				if qty, exists := limits[string(name)]; exists {
					requests[string(name)] = qty
				}
			}
		}
		if len(limits) > 0 {
			resources["limits"] = limits
		}
		if len(requests) > 0 {
			resources["requests"] = requests
		}
	}
}

func checkContainerLimits(spec map[string]any, item corev1.LimitRangeItem) error {
	for _, field := range []string{"containers", "initContainers"} {
		list, _ := spec[field].([]any)
		for _, raw := range list {
			c, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			req := resourceMap(c, "requests")
			lim := resourceMap(c, "limits")
			for name, min := range item.Min {
				if err := minConstraint("Container", string(name), min, req, lim); err != nil {
					return err
				}
			}
			for name, max := range item.Max {
				if err := maxConstraint("Container", string(name), max, req, lim); err != nil {
					return err
				}
			}
			for name, ratio := range item.MaxLimitRequestRatio {
				if err := limitRequestRatioConstraint("Container", string(name), ratio, req, lim); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkPodLimits(spec map[string]any, item corev1.LimitRangeItem) error {
	requests := podResourceUsage(spec, "requests")
	limits := podResourceUsage(spec, "limits")
	for name, min := range item.Min {
		if err := minConstraint("Pod", string(name), min, requests, limits); err != nil {
			return err
		}
	}
	for name, max := range item.Max {
		if err := maxConstraint("Pod", string(name), max, requests, limits); err != nil {
			return err
		}
	}
	for name, ratio := range item.MaxLimitRequestRatio {
		if err := limitRequestRatioConstraint("Pod", string(name), ratio, requests, limits); err != nil {
			return err
		}
	}
	return nil
}

func limitRequestRatioConstraint(limitType, resourceName string, enforced resource.Quantity, request, limit map[corev1.ResourceName]resource.Quantity) error {
	rn := corev1.ResourceName(resourceName)
	req, reqOK := request[rn]
	lim, limOK := limit[rn]
	if !reqOK || req.IsZero() {
		return fmt.Errorf("%s max limit to request ratio per %s is %s, but no request is specified or request is 0", resourceName, limitType, enforced.String())
	}
	if !limOK || lim.IsZero() {
		return fmt.Errorf("%s max limit to request ratio per %s is %s, but no limit is specified or limit is 0", resourceName, limitType, enforced.String())
	}
	observed := float64(lim.MilliValue()) / float64(req.MilliValue())
	maxRatio := float64(enforced.MilliValue()) / 1000
	if observed > maxRatio {
		return fmt.Errorf("%s max limit to request ratio per %s is %s, but provided ratio is %f", resourceName, limitType, enforced.String(), observed)
	}
	return nil
}

func minConstraint(limitType, resourceName string, enforced resource.Quantity, request, limit map[corev1.ResourceName]resource.Quantity) error {
	rn := corev1.ResourceName(resourceName)
	req, reqOK := request[rn]
	lim, limOK := limit[rn]
	if !reqOK {
		return fmt.Errorf("minimum %s usage per %s is %s. No request is specified", resourceName, limitType, enforced.String())
	}
	if req.Cmp(enforced) < 0 {
		return fmt.Errorf("minimum %s usage per %s is %s, but request is %s", resourceName, limitType, enforced.String(), req.String())
	}
	if limOK && lim.Cmp(enforced) < 0 {
		return fmt.Errorf("minimum %s usage per %s is %s, but limit is %s", resourceName, limitType, enforced.String(), lim.String())
	}
	return nil
}

func maxConstraint(limitType, resourceName string, enforced resource.Quantity, request, limit map[corev1.ResourceName]resource.Quantity) error {
	rn := corev1.ResourceName(resourceName)
	req, reqOK := request[rn]
	lim, limOK := limit[rn]
	if !limOK {
		return fmt.Errorf("maximum %s usage per %s is %s. No limit is specified", resourceName, limitType, enforced.String())
	}
	if lim.Cmp(enforced) > 0 {
		return fmt.Errorf("maximum %s usage per %s is %s, but limit is %s", resourceName, limitType, enforced.String(), lim.String())
	}
	if reqOK && req.Cmp(enforced) > 0 {
		return fmt.Errorf("maximum %s usage per %s is %s, but request is %s", resourceName, limitType, enforced.String(), req.String())
	}
	return nil
}

func resourceMap(container map[string]any, bucket string) map[corev1.ResourceName]resource.Quantity {
	out := map[corev1.ResourceName]resource.Quantity{}
	resources, _ := container["resources"].(map[string]any)
	if resources == nil {
		return out
	}
	m, _ := resources[bucket].(map[string]any)
	for name, v := range m {
		s, _ := v.(string)
		if s == "" {
			continue
		}
		q, err := resource.ParseQuantity(s)
		if err != nil {
			continue
		}
		out[corev1.ResourceName(name)] = q
	}
	return out
}

func podResourceUsage(spec map[string]any, bucket string) map[corev1.ResourceName]resource.Quantity {
	sum := containerResourceSum(spec, "containers", bucket)
	inits := containerResourceSum(spec, "initContainers", bucket)
	for name, q := range inits {
		if cur, ok := sum[name]; !ok || q.Cmp(cur) > 0 {
			sum[name] = q
		}
	}
	return sum
}

func containerResourceSum(spec map[string]any, field, bucket string) map[corev1.ResourceName]resource.Quantity {
	sum := map[corev1.ResourceName]resource.Quantity{}
	list, _ := spec[field].([]any)
	for _, raw := range list {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		resources, _ := c["resources"].(map[string]any)
		if resources == nil {
			continue
		}
		m, _ := resources[bucket].(map[string]any)
		for name, v := range m {
			s, _ := v.(string)
			if s == "" {
				continue
			}
			q, err := resource.ParseQuantity(s)
			if err != nil {
				continue
			}
			cur := sum[corev1.ResourceName(name)]
			cur.Add(q)
			sum[corev1.ResourceName(name)] = cur
		}
	}
	return sum
}
