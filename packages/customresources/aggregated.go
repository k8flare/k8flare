package customresources

import (
	"net/http"

	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	apiextensionshelpers "k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	listers "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresourcedefinition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/endpoints"
	aggregated "k8s.io/apiserver/pkg/endpoints/discovery/aggregated"
)

func wrapCRDAggregated(lister listers.CustomResourceDefinitionLister, unaggregated http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager := aggregated.NewResourceManager("apis")
		addCRDGroup(manager, apiextensionsv1.SchemeGroupVersion.Group, apiextensionsv1.SchemeGroupVersion.Version, crdDiscoveryResources())
		if crds, err := lister.List(labels.Everything()); err == nil {
			byGV := map[metav1.GroupVersion][]metav1.APIResource{}
			for _, crd := range crds {
				if !apiextensionshelpers.IsCRDConditionTrue(crd, apiextensionsv1.Established) {
					continue
				}
				for _, v := range crd.Spec.Versions {
					if !v.Served {
						continue
					}
					gv := metav1.GroupVersion{Group: crd.Spec.Group, Version: v.Name}
					verbs := metav1.Verbs{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}
					if apiextensionshelpers.IsCRDConditionTrue(crd, apiextensionsv1.Terminating) {
						verbs = metav1.Verbs{"delete", "deletecollection", "get", "list", "watch"}
					}
					namespaced := crd.Spec.Scope == apiextensionsv1.NamespaceScoped
					res := metav1.APIResource{
						Name:         crd.Status.AcceptedNames.Plural,
						SingularName: crd.Status.AcceptedNames.Singular,
						Namespaced:   namespaced,
						Group:        crd.Spec.Group,
						Version:      v.Name,
						Kind:         crd.Status.AcceptedNames.Kind,
						Verbs:        verbs,
						ShortNames:   crd.Status.AcceptedNames.ShortNames,
						Categories:   crd.Status.AcceptedNames.Categories,
					}
					if res.Name == "" {
						res.Name = crd.Spec.Names.Plural
						res.SingularName = crd.Spec.Names.Singular
						res.Kind = crd.Spec.Names.Kind
						res.ShortNames = crd.Spec.Names.ShortNames
						res.Categories = crd.Spec.Names.Categories
					}
					byGV[gv] = append(byGV[gv], res)
					sub, _ := apiextensionshelpers.GetSubresourcesForVersion(crd, v.Name)
					if sub != nil && sub.Status != nil {
						byGV[gv] = append(byGV[gv], metav1.APIResource{Name: res.Name + "/status", Namespaced: namespaced, Group: crd.Spec.Group, Version: v.Name, Kind: res.Kind, Verbs: metav1.Verbs{"get", "patch", "update"}})
					}
					if sub != nil && sub.Scale != nil {
						byGV[gv] = append(byGV[gv], metav1.APIResource{Name: res.Name + "/scale", Namespaced: namespaced, Group: "autoscaling", Version: "v1", Kind: "Scale", Verbs: metav1.Verbs{"get", "patch", "update"}})
					}
				}
			}
			for gv, resources := range byGV {
				addCRDGroup(manager, gv.Group, gv.Version, resources)
			}
		}
		aggregated.WrapAggregatedDiscoveryToHandler(unaggregated, manager, nil).ServeHTTP(w, r)
	})
}

func crdDiscoveryResources() []metav1.APIResource {
	var rest customresourcedefinition.REST
	return []metav1.APIResource{
		{Name: "customresourcedefinitions", SingularName: "customresourcedefinition", Namespaced: false, Group: apiextensionsv1.GroupName, Version: "v1", Kind: "CustomResourceDefinition", Verbs: metav1.Verbs{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}, ShortNames: rest.ShortNames(), Categories: rest.Categories()},
		{Name: "customresourcedefinitions/status", SingularName: "", Namespaced: false, Group: apiextensionsv1.GroupName, Version: "v1", Kind: "CustomResourceDefinition", Verbs: metav1.Verbs{"get", "patch", "update"}},
	}
}

func addCRDGroup(manager aggregated.ResourceManager, group, version string, resources []metav1.APIResource) {
	converted, err := endpoints.ConvertGroupVersionIntoToDiscovery(resources)
	if err != nil {
		return
	}
	manager.AddGroupVersion(group, apidiscoveryv2.APIVersionDiscovery{
		Version:   version,
		Freshness: apidiscoveryv2.DiscoveryFreshnessCurrent,
		Resources: converted,
	})
}
