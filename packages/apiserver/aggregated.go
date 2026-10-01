package apiserver

import (
	"context"
	"encoding/json"
	"net/http"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"github.com/k8flare/k8flare/packages/edgehost"
	"github.com/k8flare/k8flare/packages/metricsapi"
	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints"
	aggregated "k8s.io/apiserver/pkg/endpoints/discovery/aggregated"
)

const remoteDiscoveryPath = "/internal/remote-discovery"

const aggregatedAccept = "application/json;g=apidiscovery.k8s.io;v=v2;as=APIGroupDiscoveryList"

func builtinResourceManager(path string, core bool) aggregated.ResourceManager {
	manager := aggregated.NewResourceManager(path)
	for _, sgv := range registry.Served {
		isCore := sgv.GV.Group == ""
		if isCore != core {
			continue
		}
		resources := append([]metav1.APIResource(nil), sgv.Resources...)
		for i := range resources {
			if resources[i].Group == "" {
				resources[i].Group = sgv.GV.Group
			}
			if resources[i].Version == "" {
				resources[i].Version = sgv.GV.Version
			}
		}
		converted, err := endpoints.ConvertGroupVersionIntoToDiscovery(resources)
		if err != nil {
			continue
		}
		manager.AddGroupVersion(sgv.GV.Group, apidiscoveryv2.APIVersionDiscovery{
			Version:   sgv.GV.Version,
			Freshness: apidiscoveryv2.DiscoveryFreshnessCurrent,
			Resources: converted,
		})
	}
	if !core {
		addApiextensionsDiscovery(manager)
		if !edgehost.Disabled(edgehost.MetricsServer) {
			addMetricsDiscovery(manager)
		}
	}
	return manager
}

func addMetricsDiscovery(manager aggregated.ResourceManager) {
	resources := metricsapi.Resources()
	for i := range resources {
		resources[i].Group = metricsapi.Group
		resources[i].Version = metricsapi.Version
	}
	converted, err := endpoints.ConvertGroupVersionIntoToDiscovery(resources)
	if err != nil {
		return
	}
	manager.AddGroupVersion(metricsapi.Group, apidiscoveryv2.APIVersionDiscovery{
		Version:   metricsapi.Version,
		Freshness: apidiscoveryv2.DiscoveryFreshnessCurrent,
		Resources: converted,
	})
}

func wrapAggregated(unaggregated http.Handler, core bool, extras func(context.Context, aggregated.ResourceManager)) http.Handler {
	path := "apis"
	if core {
		path = "api"
	}
	agg := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager := builtinResourceManager(path, core)
		if extras != nil {
			extras(r.Context(), manager)
		}
		manager.ServeHTTP(w, r)
	})
	return aggregated.WrapAggregatedDiscoveryToHandler(unaggregated, agg, nil)
}

func addDynamicAggregated(cfg Config) func(context.Context, aggregated.ResourceManager) {
	return func(ctx context.Context, manager aggregated.ResourceManager) {
		crd := manager.WithSource(aggregated.CRDSource)
		for _, g := range fetchCRDAggregated(ctx, cfg) {
			if g.Name == apiextensionsGroupName {
				continue
			}
			for _, v := range g.Versions {
				crd.AddGroupVersion(g.Name, v)
			}
		}
		remote := manager.WithSource(aggregated.AggregatorSource)
		for _, g := range apiServiceAggregated(ctx, cfg) {
			for _, v := range g.Versions {
				remote.AddGroupVersion(g.Name, v)
			}
		}
	}
}

func fetchCRDAggregated(ctx context.Context, cfg Config) []apidiscoveryv2.APIGroupDiscovery {
	if cfg.CustomResources == nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, customResourcesBase+"/apis", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", aggregatedAccept)
	resp, err := cfg.CustomResources.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var list apidiscoveryv2.APIGroupDiscoveryList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil
	}
	if list.Kind != "APIGroupDiscoveryList" {
		return nil
	}
	return list.Items
}

func apiServiceAggregated(ctx context.Context, cfg Config) []apidiscoveryv2.APIGroupDiscovery {
	groups := remoteAPIServiceGroups(ctx, kineStore(cfg.Kine))
	if len(groups) == 0 {
		return nil
	}
	if fetched := fetchRemoteAggregated(ctx, cfg); fetched != nil {
		return fetched
	}
	out := make([]apidiscoveryv2.APIGroupDiscovery, 0, len(groups))
	for _, g := range groups {
		item := apidiscoveryv2.APIGroupDiscovery{ObjectMeta: metav1.ObjectMeta{Name: g.Name}}
		for _, v := range g.Versions {
			item.Versions = append(item.Versions, apidiscoveryv2.APIVersionDiscovery{
				Version:   v.Version,
				Freshness: apidiscoveryv2.DiscoveryFreshnessStale,
			})
		}
		out = append(out, item)
	}
	return out
}

func fetchRemoteAggregated(ctx context.Context, cfg Config) []apidiscoveryv2.APIGroupDiscovery {
	if cfg.Groups == nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, groupsBase+remoteDiscoveryPath, nil)
	if err != nil {
		return nil
	}
	req.Header.Set(workerHeader, "apiserver-apiregistration")
	resp, err := cfg.Groups.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var list apidiscoveryv2.APIGroupDiscoveryList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil || list.Kind != "APIGroupDiscoveryList" {
		return nil
	}
	return list.Items
}
