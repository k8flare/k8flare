package apiserver

import (
	"fmt"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"net/http"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/endpoints"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
)

func apiRoot(gv schema.GroupVersion) string {
	if gv.Group == "" {
		return "/api"
	}
	return "/apis"
}

// installAPI builds every route, and the per-version discovery documents,
// with k8s.io/apiserver's own API installer. The root discovery documents
// (/api, /apis) are the only ones added by hand. It returns the stores by
// resource name.
func installAPI(mux *http.ServeMux, client *kine.Client, tokens authenticator.Token, kubelet registry.KubeletProxy) (map[string]*genericregistry.Store, error) {
	stores := map[string]*genericregistry.Store{}
	byGV := map[schema.GroupVersion]map[string]rest.Storage{}
	for _, sgv := range registry.Served {
		storage := map[string]rest.Storage{}
		for _, res := range sgv.Resources {
			if strings.Contains(res.Name, "/") {
				continue
			}
			if create, ok := reviewCreators(tokens)[res.Name]; ok {
				storage[res.Name] = newReviewREST(sgv.GV.WithKind(res.Kind), res.SingularName, create)
				continue
			}
			store, err := registry.NewStore(client, sgv.GV, res, supervisor.ClusterCIDR)
			if err != nil {
				return nil, err
			}
			stores[res.Name] = store
			storage[res.Name] = registry.WithNames(store, res)
		}
		for _, res := range sgv.Resources {
			parent, sub, ok := strings.Cut(res.Name, "/")
			if !ok {
				continue
			}
			switch sub {
			case "status":
				storage[res.Name] = registry.NewStatusREST(stores[parent])
			case "log":
				storage[res.Name] = registry.NewLogREST(stores["pods"], stores["nodes"], kubelet)
			default:
				return nil, fmt.Errorf("no implementation for subresource %s", res.Name)
			}
		}
		byGV[sgv.GV] = storage
	}
	container := restful.NewContainer()
	container.ServeMux = mux
	container.Router(restful.CurlyRouter{})
	addresses := discovery.DefaultAddresses{DefaultAddress: "k8flare"}
	rootAPIs := discovery.NewRootAPIsHandler(addresses, scheme.Codecs)
	for _, sgv := range registry.Served {
		gv := sgv.GV
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
			return nil, fmt.Errorf("install %s: %w", gv, err)
		}
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
	return stores, nil
}

func servedVersions(gv schema.GroupVersion, storage map[string]rest.Storage) map[string][]string {
	served := make(map[string][]string, len(storage))
	for resource := range storage {
		served[resource] = []string{gv.String()}
	}
	return served
}
