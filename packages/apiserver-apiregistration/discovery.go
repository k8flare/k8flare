package apiregistration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	apidiscoveryv2 "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints"
	"k8s.io/client-go/transport"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

const (
	remoteDiscoveryPath      = "/internal/remote-discovery"
	aggregatedAccept         = "application/json;g=apidiscovery.k8s.io;v=v2;as=APIGroupDiscoveryList"
	aggregatorUser           = "system:kube-aggregator"
	discoveryRefreshInterval = time.Minute
	discoveryFetchTimeout    = 5 * time.Second
)

var remoteDiscoveries = newDiscoveryCache()

type discoveryEntry struct {
	target  string
	version apidiscoveryv2.APIVersionDiscovery
	etag    string
	updated time.Time
	failed  bool
}

type discoveryCache struct {
	mu      sync.Mutex
	entries map[string]*discoveryEntry
}

func newDiscoveryCache() *discoveryCache {
	return &discoveryCache{entries: map[string]*discoveryEntry{}}
}

func serveRemoteDiscovery(w http.ResponseWriter, r *http.Request) {
	list := apidiscoveryv2.APIGroupDiscoveryList{
		TypeMeta: metav1.TypeMeta{APIVersion: apidiscoveryv2.SchemeGroupVersion.String(), Kind: "APIGroupDiscoveryList"},
		Items:    remoteDiscoveries.groups(r.Context()),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (c *discoveryCache) groups(ctx context.Context) []apidiscoveryv2.APIGroupDiscovery {
	svcs, err := listJSON[apiregistrationv1.APIService](ctx, "/registry/apiservices/")
	if err != nil {
		return nil
	}
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })
	var out []apidiscoveryv2.APIGroupDiscovery
	index := map[string]int{}
	for i := range svcs {
		svc := &svcs[i]
		if !isRemote(svc) || svc.Spec.Group == "" {
			continue
		}
		at, ok := index[svc.Spec.Group]
		if !ok {
			at = len(out)
			index[svc.Spec.Group] = at
			out = append(out, apidiscoveryv2.APIGroupDiscovery{ObjectMeta: metav1.ObjectMeta{Name: svc.Spec.Group}})
		}
		out[at].Versions = append(out[at].Versions, c.version(ctx, svc))
	}
	return out
}

func (c *discoveryCache) version(ctx context.Context, svc *apiregistrationv1.APIService) apidiscoveryv2.APIVersionDiscovery {
	c.mu.Lock()
	defer c.mu.Unlock()
	target := discoveryTarget(svc)
	entry := c.entries[svc.Name]
	if entry == nil || entry.target != target || time.Since(entry.updated) >= discoveryRefreshInterval {
		entry = fetchDiscovery(ctx, svc, target, entry)
		c.entries[svc.Name] = entry
	}
	version := entry.version
	version.Version = svc.Spec.Version
	version.Freshness = apidiscoveryv2.DiscoveryFreshnessCurrent
	if entry.failed {
		version.Freshness = apidiscoveryv2.DiscoveryFreshnessStale
	}
	return version
}

func discoveryTarget(svc *apiregistrationv1.APIService) string {
	if name := workerName(svc); name != "" {
		return "worker/" + name
	}
	ref := svc.Spec.Service
	port := int32(443)
	if ref.Port != nil && *ref.Port != 0 {
		port = *ref.Port
	}
	return fmt.Sprintf("service/%s/%s:%d", ref.Namespace, ref.Name, port)
}

func fetchDiscovery(ctx context.Context, svc *apiregistrationv1.APIService, target string, previous *discoveryEntry) *discoveryEntry {
	ctx, cancel := context.WithTimeout(ctx, discoveryFetchTimeout)
	defer cancel()
	now := time.Now()
	entry := &discoveryEntry{target: target, updated: now}
	if previous != nil && previous.target == target {
		entry.version, entry.etag = previous.version, previous.etag
	}
	gv := metav1.GroupVersion{Group: svc.Spec.Group, Version: svc.Spec.Version}
	resp, err := getRemote(ctx, svc, "/apis", aggregatedAccept, entry.etag)
	if err != nil {
		entry.failed = true
		return entry
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified && previous != nil:
		return entry
	case resp.StatusCode == http.StatusServiceUnavailable:
		entry.failed = true
	case resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get("Content-Type"), "as=APIGroupDiscoveryList"):
		var list apidiscoveryv2.APIGroupDiscoveryList
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			entry.failed = true
			return entry
		}
		entry.etag = resp.Header.Get("Etag")
		entry.version = apidiscoveryv2.APIVersionDiscovery{Version: gv.Version}
		for _, g := range list.Items {
			if g.Name != gv.Group {
				continue
			}
			for _, v := range g.Versions {
				if v.Version == gv.Version {
					entry.version = v
				}
			}
		}
	default:
		fetchLegacyDiscovery(ctx, svc, gv, entry)
	}
	return entry
}

func fetchLegacyDiscovery(ctx context.Context, svc *apiregistrationv1.APIService, gv metav1.GroupVersion, entry *discoveryEntry) {
	entry.etag = ""
	resp, err := getRemote(ctx, svc, "/apis/"+gv.Group+"/"+gv.Version, "application/json", "")
	if err != nil {
		entry.failed = true
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		entry.failed = true
		return
	}
	var list metav1.APIResourceList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		entry.failed = true
		return
	}
	resources, err := endpoints.ConvertGroupVersionIntoToDiscovery(list.APIResources)
	if err != nil {
		entry.failed = true
		return
	}
	entry.version = apidiscoveryv2.APIVersionDiscovery{Version: gv.Version, Resources: resources}
}

func getRemote(ctx context.Context, svc *apiregistrationv1.APIService, path, accept, etag string) (*http.Response, error) {
	target, err := resolveRemote(ctx, svc, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.location.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range target.header {
		req.Header[k] = v
	}
	req.Header.Set("Accept", accept)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	transport.SetAuthProxyHeaders(req, aggregatorUser, "", []string{"system:masters"}, nil)
	return target.transport.RoundTrip(req)
}
