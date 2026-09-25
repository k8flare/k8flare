package admission

import (
	"context"
	"sort"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
)

func applyDefaultIngressClass(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "ingresses" || req.Subresource != "" || req.Object == nil {
		return nil
	}
	if req.Operation != "" && req.Operation != "CREATE" {
		return nil
	}
	spec, _ := req.Object["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	if _, ok := spec["ingressClassName"]; ok {
		return nil
	}
	if meta, _ := req.Object["metadata"].(map[string]any); meta != nil {
		if anns, _ := meta["annotations"].(map[string]any); anns != nil {
			if _, ok := anns[networkingv1beta1.AnnotationIngressClass]; ok {
				return nil
			}
		}
	}
	classes, err := s.ingressClasses(ctx)
	if err != nil {
		return err
	}
	var defaults []networkingv1.IngressClass
	for _, class := range classes {
		if class.Annotations[networkingv1.AnnotationIsDefaultIngressClass] == "true" {
			defaults = append(defaults, class)
		}
	}
	if len(defaults) == 0 {
		return nil
	}
	sort.Slice(defaults, func(i, j int) bool {
		if defaults[i].CreationTimestamp.Equal(&defaults[j].CreationTimestamp) {
			return defaults[i].Name < defaults[j].Name
		}
		return defaults[i].CreationTimestamp.After(defaults[j].CreationTimestamp.Time)
	})
	spec["ingressClassName"] = defaults[0].Name
	return nil
}
