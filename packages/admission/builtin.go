package admission

import (
	"context"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
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
