// Package quota carries the ResourceQuota controller. It is a shard of its own
// because pkg/quota/v1/install registers an evaluator for every quota-tracked
// resource, so it links most API groups wherever it lands.
package quota

import (
	"context"
	"github.com/k8flare/k8flare/packages/workloads"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/quota/v1/generic"
	"k8s.io/client-go/informers"
	pkgcontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/resourcequota"
	quotainstall "k8s.io/kubernetes/pkg/quota/v1/install"
)

func init() { workloads.Register(build) }

func build(ctx context.Context, d workloads.Deps, controllers map[string]bool) ([]func(context.Context), error) {
	if !controllers["resourcequota"] {
		return nil, nil
	}
	client, factory := d.Client, d.Factory
	core := d.Core()
	runs := []func(context.Context){}
	quotaConfiguration, err := quotainstall.NewQuotaConfigurationForControllers(generic.ListerFuncForResourceFunc(func(gvr schema.GroupVersionResource) (informers.GenericInformer, error) {
		return d.QuotaInformer(gvr)
	}), factory)
	if err != nil {
		return nil, err
	}
	started := make(chan struct{})
	close(started)
	registry := generic.NewRegistry(quotaConfiguration.Evaluators())
	d.AddQuotaCountEvaluators(registry)
	rq, err := resourcequota.NewController(ctx, &resourcequota.ControllerOptions{
		QuotaClient:           client.CoreV1(),
		ResourceQuotaInformer: core.ResourceQuotas(),
		ResyncPeriod:          pkgcontroller.StaticResyncPeriodFunc(0),
		Registry:              registry,
		IgnoredResourcesFunc:  quotaConfiguration.IgnoredResources,
		InformersStarted:      started,
	})
	if err != nil {
		return nil, err
	}
	runs = append(runs, func(ctx context.Context) { rq.Run(ctx, workloads.Workers) })

	return runs, nil
}
