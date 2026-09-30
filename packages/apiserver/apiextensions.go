package apiserver

import (
	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints"
	aggregated "k8s.io/apiserver/pkg/endpoints/discovery/aggregated"
)

const (
	apiextensionsGroupName = "apiextensions.k8s.io"
	apiextensionsVersion   = "v1"
)

func apiextensionsGroup() metav1.APIGroup {
	gv := metav1.GroupVersionForDiscovery{GroupVersion: apiextensionsGroupName + "/" + apiextensionsVersion, Version: apiextensionsVersion}
	return metav1.APIGroup{Name: apiextensionsGroupName, Versions: []metav1.GroupVersionForDiscovery{gv}, PreferredVersion: gv}
}

func apiextensionsResources() []metav1.APIResource {
	return []metav1.APIResource{
		{Name: "customresourcedefinitions", SingularName: "customresourcedefinition", Namespaced: false, Group: apiextensionsGroupName, Version: apiextensionsVersion, Kind: "CustomResourceDefinition", Verbs: metav1.Verbs{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}, ShortNames: []string{"crd", "crds"}, Categories: []string{"api-extensions"}},
		{Name: "customresourcedefinitions/status", Namespaced: false, Group: apiextensionsGroupName, Version: apiextensionsVersion, Kind: "CustomResourceDefinition", Verbs: metav1.Verbs{"get", "patch", "update"}},
	}
}

func addApiextensionsDiscovery(manager aggregated.ResourceManager) {
	converted, err := endpoints.ConvertGroupVersionIntoToDiscovery(apiextensionsResources())
	if err != nil {
		return
	}
	manager.AddGroupVersion(apiextensionsGroupName, apidiscoveryv2.APIVersionDiscovery{
		Version:   apiextensionsVersion,
		Freshness: apidiscoveryv2.DiscoveryFreshnessCurrent,
		Resources: converted,
	})
}
