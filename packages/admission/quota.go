package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	quota "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/apiserver/pkg/quota/v1/generic"
	"k8s.io/kubernetes/pkg/quota/v1/evaluator/core"
	"k8s.io/utils/clock"
)

const quotaWriteRetries = 10

func applyResourceQuota(ctx context.Context, s *store, req *admit.Request) error {
	if req.Phase != "validate" || (req.Operation != "CREATE" && req.Operation != "UPDATE") {
		return nil
	}
	if req.Subresource != "" || req.Namespace == "" || req.Object == nil {
		return nil
	}
	ev, item, ok := quotaItem(req)
	if !ok {
		return nil
	}
	for attempt := 0; ; attempt++ {
		quotas, err := listStored[corev1.ResourceQuota](ctx, s.client, "/registry/resourcequotas/"+req.Namespace+"/")
		if err != nil || len(quotas) == 0 {
			return err
		}
		listed, err := quotaUsage(ctx, s, req, ev, item)
		if err != nil {
			return err
		}
		delta, err := ev.Usage(item)
		if err != nil {
			return err
		}
		var reserved []stored[corev1.ResourceQuota]
		for i := range quotas {
			rq := &quotas[i].object
			hard := rq.Status.Hard
			if len(hard) == 0 {
				hard = rq.Spec.Hard
			}
			probe := rq.DeepCopy()
			if len(probe.Status.Hard) == 0 {
				probe.Status.Hard = hard
			}
			match, err := ev.Matches(probe, item)
			if err != nil {
				return err
			}
			if !match {
				continue
			}
			tracked := ev.MatchingResources(quota.ResourceNames(hard))
			if err := ev.Constraints(tracked, item); err != nil {
				return fmt.Errorf("failed quota: %s: %v", rq.Name, err)
			}
			used := quota.Subtract(listed, delta)
			if len(rq.Status.Used) > 0 {
				used = quota.Max(rq.Status.Used, used)
			}
			requested := quota.RemoveZeros(quota.Mask(delta, tracked))
			if len(requested) == 0 {
				continue
			}
			newUsage := quota.Add(used, requested)
			masked := quota.Mask(newUsage, quota.ResourceNames(requested))
			if allowed, exceeded := quota.LessThanOrEqual(masked, quota.Mask(hard, tracked)); !allowed {
				return fmt.Errorf("exceeded quota: %s, requested: %s, used: %s, limited: %s",
					rq.Name,
					prettyPrint(quota.Mask(requested, exceeded)),
					prettyPrint(quota.Mask(used, exceeded)),
					prettyPrint(quota.Mask(hard, exceeded)))
			}
			if req.DryRun {
				continue
			}
			reservedUsage := quota.Mask(newUsage, quota.ResourceNames(hard))
			if quota.Equals(rq.Status.Used, reservedUsage) {
				continue
			}
			rq.Status.Used = reservedUsage
			reserved = append(reserved, quotas[i])
		}
		conflict := false
		for _, rq := range reserved {
			err := putJSON(ctx, s.client, rq.key, &rq.object, rq.revision)
			if err == kine.ErrConflict {
				conflict = true
				continue
			}
			if err != nil {
				return fmt.Errorf("quota %s: %w", rq.object.Name, err)
			}
		}
		if !conflict {
			return nil
		}
		if attempt+1 >= quotaWriteRetries {
			return fmt.Errorf("quota %s: %w", req.Namespace, kine.ErrConflict)
		}
	}
}

func prettyPrint(item corev1.ResourceList) string {
	keys := make([]string, 0, len(item))
	for key := range item {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := item[corev1.ResourceName(key)]
		parts = append(parts, key+"="+value.String())
	}
	return strings.Join(parts, ",")
}

func quotaUsage(ctx context.Context, s *store, req *admit.Request, ev quota.Evaluator, item runtime.Object) (corev1.ResourceList, error) {
	objs, err := listQuotaObjects(ctx, s, req)
	if err != nil {
		return nil, err
	}
	if req.Operation == "CREATE" {
		objs = append(objs, item)
	} else {
		objs = replaceQuotaObject(objs, req.Name, item)
	}
	used := corev1.ResourceList{}
	for _, obj := range objs {
		u, err := ev.Usage(obj)
		if err != nil {
			return nil, err
		}
		used = quota.Add(used, u)
	}
	return used, nil
}

func listQuotaObjects(ctx context.Context, s *store, req *admit.Request) ([]runtime.Object, error) {
	prefix, ok := quotaStoragePrefix(req.Resource.Group, req.Resource.Resource, req.Namespace)
	if !ok {
		return nil, nil
	}
	switch {
	case req.Resource.Group == "" && req.Resource.Resource == "services":
		return pointers(listPrefix[corev1.Service](ctx, s.client, prefix))
	case req.Resource.Group == "" && req.Resource.Resource == "pods":
		return pointers(listPrefix[corev1.Pod](ctx, s.client, prefix))
	case req.Resource.Group == "" && req.Resource.Resource == "secrets":
		return pointers(listPrefix[corev1.Secret](ctx, s.client, prefix))
	case req.Resource.Group == "" && req.Resource.Resource == "configmaps":
		return pointers(listPrefix[corev1.ConfigMap](ctx, s.client, prefix))
	case req.Resource.Group == "" && req.Resource.Resource == "resourcequotas":
		return pointers(listPrefix[corev1.ResourceQuota](ctx, s.client, prefix))
	case req.Resource.Group == "" && req.Resource.Resource == "persistentvolumeclaims":
		return pointers(listPrefix[corev1.PersistentVolumeClaim](ctx, s.client, prefix))
	case req.Resource.Group == "" && req.Resource.Resource == "replicationcontrollers":
		return pointers(listPrefix[corev1.ReplicationController](ctx, s.client, prefix))
	default:
		return pointers(listPrefix[unstructured.Unstructured](ctx, s.client, prefix))
	}
}

func quotaStoragePrefix(group, resource, ns string) (string, bool) {
	if group == "" {
		return "/registry/" + resource + "/" + ns + "/", true
	}
	root, ok := quotaGroupedStorage[group+"/"+resource]
	if !ok {
		return "", false
	}
	return root + ns + "/", true
}

var quotaGroupedStorage = map[string]string{
	"apps/replicasets":                       "/registry/replicasets/",
	"apps/deployments":                       "/registry/deployments/",
	"apps/statefulsets":                      "/registry/statefulsets/",
	"apps/daemonsets":                        "/registry/daemonsets/",
	"batch/jobs":                             "/registry/jobs/",
	"batch/cronjobs":                         "/registry/cronjobs/",
	"policy/poddisruptionbudgets":            "/registry/poddisruptionbudgets/",
	"networking.k8s.io/ingresses":            "/registry/ingresses/",
	"networking.k8s.io/networkpolicies":      "/registry/networkpolicies/",
	"rbac.authorization.k8s.io/roles":        "/registry/roles/",
	"rbac.authorization.k8s.io/rolebindings": "/registry/rolebindings/",
}

func pointers[T any](items []T, err error) ([]runtime.Object, error) {
	if err != nil {
		return nil, err
	}
	out := make([]runtime.Object, 0, len(items))
	for i := range items {
		out = append(out, any(&items[i]).(runtime.Object))
	}
	return out, nil
}

func replaceQuotaObject(objs []runtime.Object, name string, item runtime.Object) []runtime.Object {
	out := make([]runtime.Object, 0, len(objs)+1)
	found := false
	for _, obj := range objs {
		acc, err := meta.Accessor(obj)
		if err == nil && acc.GetName() == name {
			out = append(out, item)
			found = true
			continue
		}
		out = append(out, obj)
	}
	if !found {
		out = append(out, item)
	}
	return out
}

func quotaItem(req *admit.Request) (quota.Evaluator, runtime.Object, bool) {
	raw, err := json.Marshal(req.Object)
	if err != nil {
		return nil, nil, false
	}
	switch req.Resource.Resource {
	case "services":
		var svc corev1.Service
		if json.Unmarshal(raw, &svc) != nil {
			return nil, nil, false
		}
		return core.NewServiceEvaluator(nil), &svc, true
	case "pods":
		var pod corev1.Pod
		if json.Unmarshal(raw, &pod) != nil {
			return nil, nil, false
		}
		return core.NewPodEvaluator(nil, clock.RealClock{}), &pod, true
	case "persistentvolumeclaims":
		var pvc corev1.PersistentVolumeClaim
		if json.Unmarshal(raw, &pvc) != nil {
			return nil, nil, false
		}
		return core.NewPersistentVolumeClaimEvaluator(nil), &pvc, true
	case "secrets":
		var sec corev1.Secret
		if json.Unmarshal(raw, &sec) != nil {
			return nil, nil, false
		}
		return countEvaluator("", "secrets", corev1.ResourceSecrets), &sec, true
	case "configmaps":
		var cm corev1.ConfigMap
		if json.Unmarshal(raw, &cm) != nil {
			return nil, nil, false
		}
		return countEvaluator("", "configmaps", corev1.ResourceConfigMaps), &cm, true
	case "resourcequotas":
		var rq corev1.ResourceQuota
		if json.Unmarshal(raw, &rq) != nil {
			return nil, nil, false
		}
		return countEvaluator("", "resourcequotas", corev1.ResourceQuotas), &rq, true
	case "replicationcontrollers":
		var rc corev1.ReplicationController
		if json.Unmarshal(raw, &rc) != nil {
			return nil, nil, false
		}
		return countEvaluator("", "replicationcontrollers", corev1.ResourceReplicationControllers), &rc, true
	default:
		if _, ok := quotaStoragePrefix(req.Resource.Group, req.Resource.Resource, req.Namespace); !ok {
			return nil, nil, false
		}
		var obj unstructured.Unstructured
		if json.Unmarshal(raw, &obj) != nil {
			return nil, nil, false
		}
		return countEvaluator(req.Resource.Group, req.Resource.Resource, ""), &obj, true
	}
}

func countEvaluator(group, resource string, alias corev1.ResourceName) quota.Evaluator {
	gvr := corev1.SchemeGroupVersion.WithResource(resource)
	if group != "" {
		gvr.Group = group
	}
	return generic.NewObjectCountEvaluator(gvr.GroupResource(), func(string) ([]runtime.Object, error) { return nil, nil }, alias)
}
