package workloads

import (
	"context"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	quota "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/informers"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// Deps hands a shard's controllers the harness pieces they need. A shard lives
// in its own package and its own worker binary, so the controller packages it
// imports are the only ones that binary links.
type Deps struct {
	Client            kubernetes.Interface
	Factory           informers.SharedInformerFactory
	RootCA            []byte
	SigningCA         []byte
	ServingCA         []byte
	ServiceAccountKey []byte
}

func (d Deps) VAPolicies() vapSnapshot { return validatingAdmissionPolicySnapshot(d.Factory) }

// Workers is the concurrency most controllers are started with.
const Workers = workers

// DaemonSetWorkers is the DaemonSet controller's own, lower concurrency.
const DaemonSetWorkers = daemonSetWorkers

// MaxEndpointsPerSlice is the cap the EndpointSlice controller slices at.
const MaxEndpointsPerSlice = maxEndpointsPerSlice

func (d Deps) Apps() appsinformers.Interface { return d.Factory.Apps().V1() }
func (d Deps) Core() coreinformers.Interface { return d.Factory.Core().V1() }

func (d Deps) ServiceCIDRs() cidrSnapshot { return serviceCIDRSnapshot(d.Factory) }
func (d Deps) IPAddresses() ipSnapshot    { return ipAddressSnapshot(d.Factory) }

func (d Deps) QuotaInformer(gvr schema.GroupVersionResource) (informers.GenericInformer, error) {
	return quotaInformer(d.Factory, gvr)
}

func (d Deps) ScaleMapper() meta.RESTMapper { return scaleMapper() }
func (d Deps) Scales() clientScales         { return clientScales{d.Client} }

// Snapshot registers a snapshot-backed informer for example, which replays the
// objects the pass already listed instead of watching.
func (d Deps) Snapshot(example runtime.Object) cache.SharedIndexInformer {
	return d.Factory.InformerFor(example, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
		return newSnapshotInformer(example)
	})
}

func (d Deps) CSRs() csrInformer {
	return csrInformer{d.Snapshot(&certificatesv1.CertificateSigningRequest{})}
}

func (d Deps) ClusterRoles() clusterRoleInformer {
	return clusterRoleInformer{d.Snapshot(&rbacv1.ClusterRole{})}
}

// AddQuotaCountEvaluators registers the count-based quota evaluators, which
// the generic registry does not cover.
func (d Deps) AddQuotaCountEvaluators(registry quota.Registry) {
	addQuotaCountEvaluators(registry, d.Factory)
}

func (d Deps) StartCSRSigners(ctx context.Context, csrs csrInformer) []func(context.Context) {
	return startCSRSigners(ctx, d.Client, csrs, d.SigningCA, d.ServingCA)
}

// Builder constructs the controllers a shard carries. It returns nothing when
// none of its controllers appear in the wanted set.
type Builder func(ctx context.Context, d Deps, controllers map[string]bool) ([]func(context.Context), error)

var builders []Builder

// Register adds a shard's controllers to this worker. A shard package calls it
// from init, and a worker picks its shards up by importing them.
func Register(b Builder) { builders = append(builders, b) }

func (d Deps) buildShards(ctx context.Context, controllers map[string]bool) ([]func(context.Context), error) {
	var runs []func(context.Context)
	for _, b := range builders {
		extra, err := b(ctx, d, controllers)
		if err != nil {
			return nil, err
		}
		runs = append(runs, extra...)
	}
	return runs, nil
}
