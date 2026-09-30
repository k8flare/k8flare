package apiserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

const workerAnnot = "k8flare.com/worker"

func isRemoteAPIService(svc *apiregistrationv1.APIService) bool {
	return svc.Spec.Service != nil || strings.TrimSpace(svc.Annotations[workerAnnot]) != ""
}

func kineStore(httpClient *http.Client) *kine.Client {
	if httpClient == nil {
		return nil
	}
	return &kine.Client{HTTP: httpClient}
}

func apiServiceName(group, version string) string {
	return version + "." + group
}

func loadAPIService(ctx context.Context, client *kine.Client, name string) (*apiregistrationv1.APIService, bool) {
	if client == nil || name == "" {
		return nil, false
	}
	kv, _, err := client.Get(ctx, "/registry/apiservices/"+name)
	if err != nil || kv == nil {
		return nil, false
	}
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, false
	}
	var svc apiregistrationv1.APIService
	if err := json.Unmarshal(data, &svc); err != nil {
		return nil, false
	}
	return &svc, true
}

func remoteAPIService(ctx context.Context, client *kine.Client, group, version string) (*apiregistrationv1.APIService, bool) {
	svc, ok := loadAPIService(ctx, client, apiServiceName(group, version))
	if !ok || !isRemoteAPIService(svc) {
		return nil, false
	}
	return svc, true
}

func hasRemoteAPIServiceGroup(ctx context.Context, client *kine.Client, group string) bool {
	if client == nil || group == "" {
		return false
	}
	kvs, _, _, err := client.List(ctx, "/registry/apiservices/", "", 0)
	if err != nil {
		return false
	}
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		var svc apiregistrationv1.APIService
		if err := json.Unmarshal(data, &svc); err != nil {
			continue
		}
		if isRemoteAPIService(&svc) && svc.Spec.Group == group {
			return true
		}
	}
	return false
}

func remoteAPIServiceGroups(ctx context.Context, client *kine.Client) []metav1.APIGroup {
	if client == nil {
		return nil
	}
	kvs, _, _, err := client.List(ctx, "/registry/apiservices/", "", 0)
	if err != nil {
		return nil
	}
	byGroup := map[string]*metav1.APIGroup{}
	var order []string
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		var svc apiregistrationv1.APIService
		if err := json.Unmarshal(data, &svc); err != nil || !isRemoteAPIService(&svc) || svc.Spec.Group == "" {
			continue
		}
		gv := metav1.GroupVersionForDiscovery{GroupVersion: svc.Spec.Group + "/" + svc.Spec.Version, Version: svc.Spec.Version}
		g, ok := byGroup[svc.Spec.Group]
		if !ok {
			g = &metav1.APIGroup{Name: svc.Spec.Group, PreferredVersion: gv}
			byGroup[svc.Spec.Group] = g
			order = append(order, svc.Spec.Group)
		}
		g.Versions = append(g.Versions, gv)
	}
	out := make([]metav1.APIGroup, 0, len(order))
	for _, name := range order {
		out = append(out, *byGroup[name])
	}
	return out
}

func apiGroupAndVersion(path string) (group, version string) {
	rest := strings.TrimPrefix(path, "/apis/")
	if rest == path {
		return "", ""
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}
	group = parts[0]
	if len(parts) > 1 {
		version = parts[1]
	}
	return group, version
}
