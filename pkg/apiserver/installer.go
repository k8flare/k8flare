package apiserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/endpoints"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

func apiRoot(gv schema.GroupVersion) string {
	if gv.Group == "" {
		return "/api"
	}
	return "/apis"
}

// installAPI builds every route with k8s.io/apiserver's own API installer
// and the discovery documents kubectl and client-go read.
func installAPI(mux *http.ServeMux, kine *KineClient) error {
	byGV := map[schema.GroupVersion]map[string]rest.Storage{}
	resources := map[schema.GroupVersion][]metav1.APIResource{}
	var order []schema.GroupVersion
	for _, k := range Kinds {
		if byGV[k.GV] == nil {
			byGV[k.GV] = map[string]rest.Storage{}
			order = append(order, k.GV)
		}
		store := newStore(kine, k)
		byGV[k.GV][k.Resource] = store
		resources[k.GV] = append(resources[k.GV], metav1.APIResource{
			Name: k.Resource, SingularName: k.Singular, Namespaced: k.Namespaced, Kind: k.Kind, ShortNames: k.ShortNames,
			Verbs: metav1.Verbs{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		})
		if k.Status {
			byGV[k.GV][k.Resource+"/status"] = newStatusREST(store)
			resources[k.GV] = append(resources[k.GV], metav1.APIResource{
				Name: k.Resource + "/status", SingularName: "", Namespaced: k.Namespaced, Kind: k.Kind,
				Verbs: metav1.Verbs{"get", "patch", "update"},
			})
		}
	}
	container := restful.NewContainer()
	container.ServeMux = mux
	container.Router(restful.CurlyRouter{})
	addresses := discovery.DefaultAddresses{DefaultAddress: "k8flare"}
	rootAPIs := discovery.NewRootAPIsHandler(addresses, scheme.Codecs)
	for _, gv := range order {
		group := &endpoints.APIGroupVersion{
			Storage:                     byGV[gv],
			Root:                        apiRoot(gv),
			GroupVersion:                gv,
			MetaGroupVersion:            &metav1.SchemeGroupVersion,
			AllServedVersionsByResource: servedVersions(gv, byGV[gv]),
			Creater:                     scheme.Scheme,
			Convertor:                   scheme.Scheme,
			Typer:                       scheme.Scheme,
			Defaulter:                   scheme.Scheme,
			ConvertabilityChecker:       scheme.Scheme,
			UnsafeConvertor:             runtime.UnsafeObjectConvertor(scheme.Scheme),
			Namer:                       runtime.Namer(meta.NewAccessor()),
			Serializer:                  scheme.Codecs,
			ParameterCodec:              scheme.ParameterCodec,
			EquivalentResourceRegistry:  runtime.NewEquivalentResourceRegistry(),
			TypeConverter:               managedfields.NewDeducedTypeConverter(),
			Admit:                       admission.NewChainHandler(),
			MinRequestTimeout:           30 * time.Minute,
		}
		if _, _, err := group.InstallREST(container); err != nil {
			return fmt.Errorf("install %s: %w", gv, err)
		}
		list := resources[gv]
		versionHandler := discovery.NewAPIVersionHandler(scheme.Codecs, gv, discovery.APIResourceListerFunc(func() []metav1.APIResource { return list }))
		versionHandler.AddToWebService(webServiceFor(container, apiRoot(gv)+"/"+gv.String()))
		if gv.Group != "" {
			apiGroup := metav1.APIGroup{
				Name:             gv.Group,
				Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: gv.String(), Version: gv.Version}},
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: gv.String(), Version: gv.Version},
			}
			rootAPIs.AddGroup(apiGroup)
			container.Add(discovery.NewAPIGroupHandler(scheme.Codecs, apiGroup).WebService())
		}
	}
	container.Add(discovery.NewLegacyRootAPIHandler(addresses, scheme.Codecs, "/api").WebService())
	container.Add(rootAPIs.WebService())
	return nil
}

func webServiceFor(container *restful.Container, root string) *restful.WebService {
	for _, ws := range container.RegisteredWebServices() {
		if ws.RootPath() == root {
			return ws
		}
	}
	panic("no web service installed at " + root)
}

func servedVersions(gv schema.GroupVersion, storage map[string]rest.Storage) map[string][]string {
	served := make(map[string][]string, len(storage))
	for resource := range storage {
		served[resource] = []string{gv.String()}
	}
	return served
}
