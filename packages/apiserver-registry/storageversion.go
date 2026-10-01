package registry

import (
	"fmt"
	"slices"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func storageGVK(gv schema.GroupVersion, kind string) schema.GroupVersionKind {
	switch {
	case gv.Group == "events.k8s.io":
		return corev1.SchemeGroupVersion.WithKind(kind)
	case gv.Group == "autoscaling":
		return autoscalingv2.SchemeGroupVersion.WithKind(kind)
	}
	return gv.WithKind(kind)
}

func StoredResources() ([]kine.StoredResource, error) {
	var resources []kine.StoredResource
	seen := map[string]schema.GroupVersionKind{}
	for _, sgv := range Served {
		for _, res := range sgv.Resources {
			if strings.Contains(res.Name, "/") || !scheme.Scheme.Recognizes(sgv.GV.WithKind(res.Kind)) {
				continue
			}
			if !slices.Contains(res.Verbs, "watch") {
				continue
			}
			gvk := storageGVK(sgv.GV, res.Kind)
			if prev, ok := seen[res.Name]; ok {
				if prev != gvk {
					return nil, fmt.Errorf("%s is stored as both %s and %s", res.Name, prev, gvk)
				}
				continue
			}
			seen[res.Name] = gvk
			resources = append(resources, kine.StoredResource{
				Name:  res.Name,
				Hash:  discovery.StorageVersionHash(gvk.Group, gvk.Version, gvk.Kind),
				Codec: storageCodec(sgv.GV),
				New: func() runtime.Object {
					obj, _ := scheme.Scheme.New(gvk)
					return obj
				},
			})
		}
	}
	return resources, nil
}
