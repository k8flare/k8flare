// Package vap carries the ValidatingAdmissionPolicy status controller. It is a
// shard of its own because the upstream TypeChecker it needs drags cel-go,
// antlr, the policy plugin, the CEL openapi resolver and a cached discovery
// RESTMapper: 15.5 MB of a 64 MiB worker, and none of it is wanted elsewhere.
package vap

import (
	"context"

	"github.com/k8flare/k8flare/packages/workloads"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	cachediscovery "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"k8s.io/kubernetes/pkg/controller/validatingadmissionpolicystatus"
)

func init() { workloads.Register(build) }

func build(ctx context.Context, d workloads.Deps, controllers map[string]bool) ([]func(context.Context), error) {
	if !controllers["validatingadmissionpolicy"] {
		return nil, nil
	}
	checker := &validating.TypeChecker{
		SchemaResolver: &resolver.ClientDiscoveryResolver{Discovery: d.Client.Discovery()},
		RestMapper:     restmapper.NewDeferredDiscoveryRESTMapper(cachediscovery.NewMemCacheClient(d.Client.Discovery())),
	}
	vap, err := validatingadmissionpolicystatus.NewController(d.VAPolicies(), d.Client.AdmissionregistrationV1().ValidatingAdmissionPolicies(), checker)
	if err != nil {
		return nil, err
	}
	return []func(context.Context){func(ctx context.Context) { vap.Run(ctx, 1) }}, nil
}
