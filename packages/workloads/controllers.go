package workloads

import (
	"context"
	"net"
	"time"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	certificatesv1 "k8s.io/api/certificates/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	"k8s.io/apiserver/pkg/quota/v1/generic"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	cachediscovery "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/flowcontrol"
	csitrans "k8s.io/csi-translation-lib"
	"k8s.io/klog/v2"
	pkgcontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/bootstrap"
	"k8s.io/kubernetes/pkg/controller/certificates/approver"
	"k8s.io/kubernetes/pkg/controller/certificates/cleaner"
	"k8s.io/kubernetes/pkg/controller/certificates/rootcacertpublisher"
	"k8s.io/kubernetes/pkg/controller/clusterroleaggregation"
	"k8s.io/kubernetes/pkg/controller/cronjob"
	"k8s.io/kubernetes/pkg/controller/daemon"
	"k8s.io/kubernetes/pkg/controller/deployment"
	"k8s.io/kubernetes/pkg/controller/devicetainteviction"
	"k8s.io/kubernetes/pkg/controller/disruption"
	"k8s.io/kubernetes/pkg/controller/endpoint"
	"k8s.io/kubernetes/pkg/controller/endpointslice"
	"k8s.io/kubernetes/pkg/controller/endpointslicemirroring"
	"k8s.io/kubernetes/pkg/controller/job"
	"k8s.io/kubernetes/pkg/controller/nodeipam"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam"
	"k8s.io/kubernetes/pkg/controller/podgc"
	"k8s.io/kubernetes/pkg/controller/replicaset"
	"k8s.io/kubernetes/pkg/controller/replication"
	"k8s.io/kubernetes/pkg/controller/resourceclaim"
	"k8s.io/kubernetes/pkg/controller/resourcequota"
	"k8s.io/kubernetes/pkg/controller/serviceaccount"
	"k8s.io/kubernetes/pkg/controller/servicecidrs"
	"k8s.io/kubernetes/pkg/controller/statefulset"
	"k8s.io/kubernetes/pkg/controller/tainteviction"
	"k8s.io/kubernetes/pkg/controller/ttl"
	"k8s.io/kubernetes/pkg/controller/ttlafterfinished"
	"k8s.io/kubernetes/pkg/controller/validatingadmissionpolicystatus"
	"k8s.io/kubernetes/pkg/controller/volume/ephemeral"
	"k8s.io/kubernetes/pkg/controller/volume/expand"
	"k8s.io/kubernetes/pkg/controller/volume/persistentvolume"
	"k8s.io/kubernetes/pkg/controller/volume/pvcprotection"
	"k8s.io/kubernetes/pkg/controller/volume/pvprotection"
	"k8s.io/kubernetes/pkg/features"
	quotainstall "k8s.io/kubernetes/pkg/quota/v1/install"
	"k8s.io/kubernetes/pkg/volume/csi"
	"k8s.io/kubernetes/pkg/volume/csimigration"
	"k8s.io/utils/clock"
)

// buildControllers constructs the kube-controller-manager controllers this
// worker carries. A worker links only the controller packages its own copy of
// this function names, which is what keeps each binary under the Loader cap.
func buildControllers(ctx context.Context, client kubernetes.Interface, factory informers.SharedInformerFactory, rootCA, signingCA, servingCA []byte, controllers map[string]bool) ([]func(context.Context), error) {
	apps, core := factory.Apps().V1(), factory.Core().V1()
	runs := []func(context.Context){}
	if controllers["replicaset"] {
		rs := replicaset.NewReplicaSetController(ctx, apps.ReplicaSets(), core.Pods(), client, replicaset.BurstReplicas)
		runs = append(runs, func(ctx context.Context) { rs.Run(ctx, workers) })
	}
	if controllers["replication"] {
		rc := replication.NewReplicationManager(ctx, core.Pods(), core.ReplicationControllers(), client, replication.BurstReplicas)
		runs = append(runs, func(ctx context.Context) { rc.Run(ctx, workers) })
	}
	if controllers["deployment"] {
		dc, err := deployment.NewDeploymentController(ctx, apps.Deployments(), apps.ReplicaSets(), core.Pods(), client)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { dc.Run(ctx, workers) })
	}
	if controllers["endpoints"] {
		ep := endpoint.NewEndpointController(ctx, core.Pods(), core.Services(), core.Endpoints(), client, 0)
		runs = append(runs, func(ctx context.Context) { ep.Run(ctx, workers) })
	}
	if controllers["endpointslice"] {
		eps := endpointslice.NewController(ctx, core.Pods(), core.Services(), core.Nodes(), factory.Discovery().V1().EndpointSlices(), maxEndpointsPerSlice, client, 0)
		runs = append(runs, func(ctx context.Context) { eps.Run(ctx, workers) })
	}
	if controllers["endpointslicemirroring"] {
		mirror := endpointslicemirroring.NewController(ctx, core.Endpoints(), factory.Discovery().V1().EndpointSlices(), core.Services(), maxEndpointsPerSlice, client, 0)
		runs = append(runs, func(ctx context.Context) { mirror.Run(ctx, workers) })
	}
	if controllers["servicecidr"] {
		scc := servicecidrs.NewController(ctx, serviceCIDRSnapshot(factory), ipAddressSnapshot(factory), client)
		runs = append(runs, func(ctx context.Context) { scc.Run(ctx, 5) })
	}
	if controllers["validatingadmissionpolicy"] {
		checker := &validating.TypeChecker{
			SchemaResolver: &resolver.ClientDiscoveryResolver{Discovery: client.Discovery()},
			RestMapper:     restmapper.NewDeferredDiscoveryRESTMapper(cachediscovery.NewMemCacheClient(client.Discovery())),
		}
		vap, err := validatingadmissionpolicystatus.NewController(validatingAdmissionPolicySnapshot(factory), client.AdmissionregistrationV1().ValidatingAdmissionPolicies(), checker)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { vap.Run(ctx, 1) })
	}

	if controllers["daemonset"] {
		ds, err := daemon.NewDaemonSetsController(ctx, apps.DaemonSets(), apps.ControllerRevisions(), core.Pods(), core.Nodes(), client, flowcontrol.NewBackOff(time.Second, 15*time.Minute))
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { ds.Run(ctx, daemonSetWorkers) })
	}
	if controllers["statefulset"] {
		ss := statefulset.NewStatefulSetController(ctx, core.Pods(), apps.StatefulSets(), core.PersistentVolumeClaims(), apps.ControllerRevisions(), client)
		runs = append(runs, func(ctx context.Context) { ss.Run(ctx, workers) })
	}
	if controllers["job"] {
		jobs, err := job.NewController(ctx, client, core.Pods(), factory.Batch().V1().Jobs(), nil, nil)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { jobs.Run(ctx, workers) })
	}
	if controllers["ttlafterfinished"] {
		ttl := ttlafterfinished.New(ctx, factory.Batch().V1().Jobs(), client)
		runs = append(runs, func(ctx context.Context) { ttl.Run(ctx, 1) })
	}
	if controllers["cronjob"] {
		cron, err := cronjob.NewControllerV2(ctx, factory.Batch().V1().Jobs(), factory.Batch().V1().CronJobs(), client)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { cron.Run(ctx, workers) })
	}

	if controllers["serviceaccounts"] {
		accounts, err := serviceaccount.NewServiceAccountsController(klog.FromContext(ctx), core.ServiceAccounts(), core.Namespaces(), client, serviceaccount.DefaultServiceAccountsControllerOptions())
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { accounts.Run(ctx, 1) })
	}
	if controllers["legacytoken"] {
		cleaner, err := serviceaccount.NewLegacySATokenCleaner(core.ServiceAccounts(), core.Secrets(), core.Pods(), client, clock.RealClock{}, serviceaccount.LegacySATokenCleanerOptions{
			CleanUpPeriod: 365 * 24 * time.Hour,
			SyncInterval:  serviceaccount.DefaultCleanerSyncInterval,
		})
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { cleaner.Run(ctx) })
	}
	if controllers["rootca"] {
		publisher, err := rootcacertpublisher.NewPublisher(core.ConfigMaps(), core.Namespaces(), client, rootCA)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { publisher.Run(ctx, 1) })
	}
	if controllers["bootstrapsigner"] {
		signer, err := bootstrap.NewSigner(client, core.Secrets(), core.ConfigMaps(), bootstrap.DefaultSignerOptions())
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { signer.Run(ctx) })
	}
	if controllers["tokencleaner"] {
		cleaner, err := bootstrap.NewTokenCleaner(client, core.Secrets(), bootstrap.DefaultTokenCleanerOptions())
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { cleaner.Run(ctx) })
	}
	if controllers["resourcequota"] {
		quotaConfiguration, err := quotainstall.NewQuotaConfigurationForControllers(generic.ListerFuncForResourceFunc(func(gvr schema.GroupVersionResource) (informers.GenericInformer, error) {
			return quotaInformer(factory, gvr)
		}), factory)
		if err != nil {
			return nil, err
		}
		started := make(chan struct{})
		close(started)
		registry := generic.NewRegistry(quotaConfiguration.Evaluators())
		addQuotaCountEvaluators(registry, factory)
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
		runs = append(runs, func(ctx context.Context) { rq.Run(ctx, workers) })
	}
	if controllers["csrapproving"] || controllers["csrsigning"] || controllers["csrcleaner"] {
		inf := factory.InformerFor(&certificatesv1.CertificateSigningRequest{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return newSnapshotInformer(&certificatesv1.CertificateSigningRequest{})
		})
		if controllers["csrapproving"] {
			cc := approver.NewCSRApprovingController(ctx, client, csrInformer{inf})
			runs = append(runs, func(ctx context.Context) { cc.Run(ctx, workers) })
		}
		if controllers["csrsigning"] {
			runs = append(runs, startCSRSigners(ctx, client, csrInformer{inf}, signingCA, servingCA)...)
		}
		if controllers["csrcleaner"] {
			cln := cleaner.NewCSRCleanerController(client.CertificatesV1().CertificateSigningRequests(), csrInformer{inf})
			runs = append(runs, func(ctx context.Context) { cln.Run(ctx, 1) })
		}
	}
	if controllers["clusterroleaggregation"] {
		inf := factory.InformerFor(&rbacv1.ClusterRole{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return newSnapshotInformer(&rbacv1.ClusterRole{})
		})
		agg := clusterroleaggregation.NewClusterRoleAggregation(clusterRoleInformer{inf}, client.RbacV1())
		runs = append(runs, func(ctx context.Context) { agg.Run(ctx, workers) })
	}
	if controllers["nodeipam"] {
		ipamc, err := nodeipam.NewNodeIpamController(
			ctx,
			core.Nodes(),
			nil,
			client,
			[]*net.IPNet{supervisor.ClusterCIDR},
			supervisor.ServiceCIDR,
			nil,
			[]int{24},
			ipam.RangeAllocatorType,
		)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { ipamc.Run(ctx) })
	}
	if controllers["ttl"] {
		ttlc := ttl.NewTTLController(ctx, core.Nodes(), client)
		runs = append(runs, func(ctx context.Context) { ttlc.Run(ctx, 1) })
	}
	if controllers["tainteviction"] {
		if err := pkgcontroller.AddPodNodeNameIndexer(core.Pods().Informer()); err != nil {
			return nil, err
		}
		tm, err := tainteviction.New(ctx, client, core.Pods(), core.Nodes(), "taint-eviction-controller")
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { tm.Run(ctx) })
	}
	if controllers["podgc"] {
		gcc := podgc.NewPodGCInternal(ctx, client, core.Pods(), core.Nodes(), 12500, 20*time.Second, 40*time.Second)
		runs = append(runs, func(ctx context.Context) { gcc.Run(ctx) })
	}
	if controllers["ephemeralvolume"] {
		eph, err := ephemeral.NewController(ctx, client, core.Pods(), core.PersistentVolumeClaims())
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { eph.Run(ctx, 1) })
	}
	if controllers["resourceclaim"] {
		rc, err := resourceclaim.NewController(klog.FromContext(ctx), resourceclaim.Features{
			AdminAccess:            utilfeature.DefaultFeatureGate.Enabled(features.DRAAdminAccess),
			PrioritizedList:        utilfeature.DefaultFeatureGate.Enabled(features.DRAPrioritizedList),
			WorkloadResourceClaims: false,
		}, client, core.Pods(), nil, factory.Resource().V1().ResourceClaims(), factory.Resource().V1().ResourceClaimTemplates())
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { rc.Run(ctx, 1) })
	}
	if controllers["devicetainteviction"] && utilfeature.DefaultFeatureGate.Enabled(features.DRADeviceTaints) {
		evict := devicetainteviction.New(client, core.Pods(), factory.Resource().V1().ResourceClaims(), factory.Resource().V1().ResourceSlices(), nil, factory.Resource().V1().DeviceClasses(), "device-taint-eviction", false)
		runs = append(runs, func(ctx context.Context) {
			if err := evict.Run(ctx, 1); err != nil {
				klog.FromContext(ctx).Error(err, "device taint eviction stopped")
			}
		})
	}
	if controllers["pvcprotection"] {
		pvcProt, err := pvcprotection.NewPVCProtectionController(klog.FromContext(ctx), core.PersistentVolumeClaims(), core.Pods(), client)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { pvcProt.Run(ctx, 1) })
	}
	if controllers["pvprotection"] {
		pvProt := pvprotection.NewPVProtectionController(klog.FromContext(ctx), core.PersistentVolumes(), client)
		runs = append(runs, func(ctx context.Context) { pvProt.Run(ctx, 1) })
	}
	if controllers["volumeexpand"] {
		translator := csitrans.New()
		exp, err := expand.NewExpandController(ctx, client, core.PersistentVolumeClaims(), csi.ProbeVolumePlugins(), translator, csimigration.NewPluginManager(translator))
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { exp.Run(ctx) })
	}
	if controllers["persistentvolume"] {
		pvb, err := persistentvolume.NewController(ctx, persistentvolume.ControllerParameters{
			KubeClient:                client,
			SyncPeriod:                15 * time.Minute,
			VolumeInformer:            core.PersistentVolumes(),
			ClaimInformer:             core.PersistentVolumeClaims(),
			ClassInformer:             factory.Storage().V1().StorageClasses(),
			PodInformer:               core.Pods(),
			NodeInformer:              core.Nodes(),
			EnableDynamicProvisioning: false,
		})
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { pvb.Run(ctx) })
	}
	if controllers["disruption"] {
		dc := disruption.NewDisruptionController(
			ctx,
			core.Pods(),
			factory.Policy().V1().PodDisruptionBudgets(),
			core.ReplicationControllers(),
			apps.ReplicaSets(),
			apps.Deployments(),
			apps.StatefulSets(),
			client,
			scaleMapper(),
			clientScales{client},
			client.Discovery(),
		)
		runs = append(runs, func(ctx context.Context) { dc.Run(ctx) })
	}
	return runs, nil
}
