package installer

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
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

func Root(gv schema.GroupVersion) string {
	if gv.Group == "" {
		return "/api"
	}
	return "/apis"
}

func APIGroup(gv schema.GroupVersion) metav1.APIGroup {
	v := metav1.GroupVersionForDiscovery{GroupVersion: gv.String(), Version: gv.Version}
	return metav1.APIGroup{Name: gv.Group, Versions: []metav1.GroupVersionForDiscovery{v}, PreferredVersion: v}
}

type Installed struct {
	Stores    map[string]*registry.Store
	Container *restful.Container
}

func Install(mux *http.ServeMux, deps registry.Deps, only ...schema.GroupVersion) (*Installed, error) {
	stores := map[string]*registry.Store{}
	container := restful.NewContainer()
	container.ServeMux = mux
	container.Router(restful.CurlyRouter{})
	for _, sgv := range registry.Served {
		if len(only) > 0 && !contains(only, sgv.GV) {
			continue
		}
		storage := map[string]rest.Storage{}
		for _, res := range sgv.Resources {
			if strings.Contains(res.Name, "/") {
				continue
			}
			if custom, ok := registry.Resources[res.Name]; ok {
				storage[res.Name] = custom(sgv.GV, res, deps)
				continue
			}
			store, err := registry.NewStore(deps.Kine, sgv.GV, res)
			if err != nil {
				return nil, err
			}
			if customize, ok := registry.Customizers[res.Name]; ok {
				customize(store, deps)
			}
			stores[res.Name] = store
			storage[res.Name] = registry.WithNames(store, res)
		}
		for _, res := range sgv.Resources {
			parent, sub, ok := strings.Cut(res.Name, "/")
			if !ok {
				continue
			}
			switch build, custom := registry.Subresources[res.Name]; {
			case custom:
				storage[res.Name] = build(stores, deps)
			case sub == "status":
				storage[res.Name] = registry.NewStatusREST(stores[parent])
			default:
				return nil, fmt.Errorf("no implementation for subresource %s", res.Name)
			}
		}
		group := &endpoints.APIGroupVersion{
			Storage:                     storage,
			Root:                        Root(sgv.GV),
			GroupVersion:                sgv.GV,
			MetaGroupVersion:            &metav1.SchemeGroupVersion,
			AllServedVersionsByResource: servedVersions(sgv.GV, storage),
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
			return nil, fmt.Errorf("install %s: %w", sgv.GV, err)
		}
		if sgv.GV.Group != "" {
			container.Add(discovery.NewAPIGroupHandler(scheme.Codecs, APIGroup(sgv.GV)).WebService())
		}
	}
	return &Installed{Stores: stores, Container: container}, nil
}

func contains(list []schema.GroupVersion, gv schema.GroupVersion) bool {
	for _, v := range list {
		if v == gv {
			return true
		}
	}
	return false
}

func servedVersions(gv schema.GroupVersion, storage map[string]rest.Storage) map[string][]string {
	out := map[string][]string{}
	for name := range storage {
		out[name] = []string{gv.Version}
	}
	return out
}
