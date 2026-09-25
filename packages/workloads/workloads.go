package workloads

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"github.com/robfig/cron/v3"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	"k8s.io/apiserver/pkg/quota/v1/generic"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	cachediscovery "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	admissionlisters "k8s.io/client-go/listers/admissionregistration/v1"
	certlisters "k8s.io/client-go/listers/certificates/v1"
	netlisters "k8s.io/client-go/listers/networking/v1"
	rbaclisters "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
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

const (
	workers              = 5
	listPage             = 500
	drainPoll            = 200 * time.Millisecond
	maxDrain             = 30 * time.Second
	maxEndpointsPerSlice = 100
	daemonSetWorkers     = 2
	unfinishedJobRecheck = 10 * time.Second
	maxDelay             = 24 * time.Hour
)

func init() {
	for _, gate := range []string{"ReplicaSet", "Job", "StatefulSet", "DaemonSet"} {
		if err := utilfeature.DefaultMutableFeatureGate.Set("StaleControllerConsistency" + gate + "=false"); err != nil {
			panic(err)
		}
	}
}

type Result struct {
	Objects map[string]int `json:"objects"`
	Drained bool           `json:"drained"`
	NextMs  int64          `json:"nextMs"`
}

type pageFunc func(context.Context, metav1.ListOptions) (runtime.Object, error)

type source struct {
	name    string
	example runtime.Object
	page    pageFunc
}

func sources(client kubernetes.Interface) []source {
	core, apps := client.CoreV1(), client.AppsV1()
	return []source{
		{"pods", &v1.Pod{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Pods("").List(ctx, o)
		}},
		{"replicasets", &appsv1.ReplicaSet{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.ReplicaSets("").List(ctx, o)
		}},
		{"deployments", &appsv1.Deployment{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.Deployments("").List(ctx, o)
		}},
		{"replicationcontrollers", &v1.ReplicationController{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.ReplicationControllers("").List(ctx, o)
		}},
		{"services", &v1.Service{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Services("").List(ctx, o)
		}},
		{"endpoints", &v1.Endpoints{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Endpoints("").List(ctx, o)
		}},
		{"limitranges", &v1.LimitRange{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.LimitRanges("").List(ctx, o)
		}},
		{"endpointslices", &discoveryv1.EndpointSlice{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.DiscoveryV1().EndpointSlices("").List(ctx, o)
		}},
		{"jobs", &batchv1.Job{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.BatchV1().Jobs("").List(ctx, o)
		}},
		{"cronjobs", &batchv1.CronJob{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.BatchV1().CronJobs("").List(ctx, o)
		}},
		{"statefulsets", &appsv1.StatefulSet{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.StatefulSets("").List(ctx, o)
		}},
		{"daemonsets", &appsv1.DaemonSet{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.DaemonSets("").List(ctx, o)
		}},
		{"controllerrevisions", &appsv1.ControllerRevision{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return apps.ControllerRevisions("").List(ctx, o)
		}},
		{"persistentvolumeclaims", &v1.PersistentVolumeClaim{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.PersistentVolumeClaims("").List(ctx, o)
		}},
		{"persistentvolumes", &v1.PersistentVolume{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.PersistentVolumes().List(ctx, o)
		}},
		{"storageclasses", &storagev1.StorageClass{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.StorageV1().StorageClasses().List(ctx, o)
		}},
		{"namespaces", &v1.Namespace{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Namespaces().List(ctx, o)
		}},
		{"serviceaccounts", &v1.ServiceAccount{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.ServiceAccounts("").List(ctx, o)
		}},
		{"ingresses", &networkingv1.Ingress{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.NetworkingV1().Ingresses("").List(ctx, o)
		}},
		{"networkpolicies", &networkingv1.NetworkPolicy{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.NetworkingV1().NetworkPolicies("").List(ctx, o)
		}},
		{"servicecidrs", &networkingv1.ServiceCIDR{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.NetworkingV1().ServiceCIDRs().List(ctx, o)
		}},
		{"ipaddresses", &networkingv1.IPAddress{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.NetworkingV1().IPAddresses().List(ctx, o)
		}},
		{"validatingadmissionpolicies", &admissionregistrationv1.ValidatingAdmissionPolicy{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.AdmissionregistrationV1().ValidatingAdmissionPolicies().List(ctx, o)
		}},
		{"configmaps", &v1.ConfigMap{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.ConfigMaps("").List(ctx, o)
		}},
		{"nodes", &v1.Node{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Nodes().List(ctx, o)
		}},
		{"resourcequotas", &v1.ResourceQuota{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.ResourceQuotas("").List(ctx, o)
		}},
		{"secrets", &v1.Secret{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return core.Secrets("").List(ctx, o)
		}},
		{"poddisruptionbudgets", &policyv1.PodDisruptionBudget{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.PolicyV1().PodDisruptionBudgets("").List(ctx, o)
		}},
		{"certificatesigningrequests", &certificatesv1.CertificateSigningRequest{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.CertificatesV1().CertificateSigningRequests().List(ctx, o)
		}},
		{"clusterroles", &rbacv1.ClusterRole{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.RbacV1().ClusterRoles().List(ctx, o)
		}},
		{"roles", &rbacv1.Role{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.RbacV1().Roles("").List(ctx, o)
		}},
		{"rolebindings", &rbacv1.RoleBinding{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.RbacV1().RoleBindings("").List(ctx, o)
		}},
		{"resourceclaims", &resourcev1.ResourceClaim{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.ResourceV1().ResourceClaims("").List(ctx, o)
		}},
		{"resourceclaimtemplates", &resourcev1.ResourceClaimTemplate{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.ResourceV1().ResourceClaimTemplates("").List(ctx, o)
		}},
		{"resourceslices", &resourcev1.ResourceSlice{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.ResourceV1().ResourceSlices().List(ctx, o)
		}},
		{"deviceclasses", &resourcev1.DeviceClass{}, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return client.ResourceV1().DeviceClasses().List(ctx, o)
		}},
	}
}

var controllerNeeds = map[string][]string{
	"replicaset":                {"pods", "replicasets"},
	"replication":               {"pods", "replicationcontrollers"},
	"deployment":                {"pods", "replicasets", "deployments"},
	"endpoints":                 {"pods", "services", "endpoints"},
	"endpointslice":             {"pods", "services", "endpointslices", "nodes"},
	"servicecidr":               {"servicecidrs", "ipaddresses"},
	"validatingadmissionpolicy": {"validatingadmissionpolicies"},
	"endpointslicemirroring":    {"endpoints", "endpointslices", "services"},
	"job":                       {"pods", "jobs"},
	"ttlafterfinished":          {"jobs"},
	"cronjob":                   {"jobs", "cronjobs"},
	"statefulset":               {"pods", "statefulsets", "persistentvolumeclaims", "controllerrevisions"},
	"daemonset":                 {"pods", "daemonsets", "controllerrevisions", "nodes"},
	"serviceaccounts":           {"namespaces", "serviceaccounts"},
	"rootca":                    {"namespaces", "configmaps"},
	"bootstrapsigner":           {"configmaps", "secrets"},
	"tokencleaner":              {"secrets"},
	"legacytoken":               {"secrets", "serviceaccounts", "pods"},
	"resourcequota":             {"resourcequotas", "pods", "services", "persistentvolumeclaims", "secrets", "configmaps", "replicationcontrollers", "replicasets", "deployments", "statefulsets", "daemonsets", "jobs", "cronjobs", "poddisruptionbudgets", "serviceaccounts", "ingresses", "networkpolicies", "networking.k8s.io", "roles", "rolebindings", "endpoints", "limitranges"},
	"disruption":                {"poddisruptionbudgets", "pods", "replicasets", "deployments", "replicationcontrollers", "statefulsets", "jobs"},
	"persistentvolume":          {"pods", "persistentvolumes", "persistentvolumeclaims", "storageclasses", "nodes"},
	"pvcprotection":             {"pods", "persistentvolumeclaims"},
	"pvprotection":              {"persistentvolumes"},
	"ephemeralvolume":           {"pods", "persistentvolumeclaims"},
	"resourceclaim":             {"pods", "resourceclaims", "resourceclaimtemplates"},
	"devicetainteviction":       {"pods", "resourceclaims", "resourceslices", "deviceclasses"},
	"volumeexpand":              {"persistentvolumeclaims", "persistentvolumes"},
	"tainteviction":             {"pods", "nodes"},
	"nodeipam":                  {"nodes"},
	"ttl":                       {"nodes"},
	"podgc":                     {"pods", "nodes"},
	"csrapproving":              {"certificatesigningrequests"},
	"csrsigning":                {"certificatesigningrequests"},
	"csrcleaner":                {"certificatesigningrequests"},
	"clusterroleaggregation":    {"clusterroles"},
}

var controllerFollows = map[string][]string{
	"deployment": {"replicaset"},
	"cronjob":    {"job"},
}

// wanted picks the controllers whose inputs changed in this batch and the
// sources they read, so a batch only pays for the work it can actually do.
func wanted(changed []string) (map[string]bool, map[string]bool) {
	if len(changed) == 0 {
		all := map[string]bool{}
		for name := range controllerNeeds {
			all[name] = true
		}
		return all, nil
	}
	touched := map[string]bool{}
	for _, name := range changed {
		if name == "storage.k8s.io" {
			name = "storageclasses"
		}
		if name == "certificates.k8s.io" {
			name = "certificatesigningrequests"
		}
		if name == "minions" {
			name = "nodes"
		}
		touched[name] = true
	}
	controllers := map[string]bool{}
	needed := map[string]bool{}
	for name, needs := range controllerNeeds {
		for _, need := range needs {
			if touched[need] {
				controllers[name] = true
				break
			}
		}
		if controllers[name] {
			for _, need := range needs {
				needed[need] = true
			}
		}
	}
	for name := range controllers {
		for _, follow := range controllerFollows[name] {
			if controllers[follow] {
				continue
			}
			controllers[follow] = true
			for _, need := range controllerNeeds[follow] {
				needed[need] = true
			}
		}
	}
	return controllers, needed
}

func needsFollowUp(controllers map[string]bool) bool {
	return controllers["deployment"] || controllers["replication"] || controllers["replicaset"] || controllers["statefulset"] || controllers["daemonset"] || controllers["persistentvolume"] || controllers["csrapproving"] || controllers["csrsigning"] || controllers["clusterroleaggregation"] || controllers["job"] || controllers["cronjob"] || controllers["resourceclaim"]
}

func followUpChanged(controllers map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	add := func(names ...string) {
		for _, name := range names {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	if controllers["deployment"] || controllers["replicaset"] || controllers["replication"] {
		add("pods", "replicasets", "replicationcontrollers", "deployments")
	}
	if controllers["job"] || controllers["cronjob"] {
		add("pods", "jobs", "cronjobs")
	}
	if controllers["statefulset"] {
		add("pods", "statefulsets", "persistentvolumeclaims", "controllerrevisions")
	}
	if controllers["daemonset"] {
		add("pods", "daemonsets", "controllerrevisions", "nodes")
	}
	if controllers["persistentvolume"] {
		add("persistentvolumes", "persistentvolumeclaims", "storageclasses", "pods", "nodes")
	}
	if controllers["csrapproving"] || controllers["csrsigning"] {
		add("certificatesigningrequests")
	}
	if controllers["clusterroleaggregation"] {
		add("clusterroles")
	}
	if controllers["resourceclaim"] {
		add("pods", "resourceclaims", "resourceclaimtemplates")
	}
	return out
}

func changedHas(changed []string, name string) bool {
	for _, item := range changed {
		if item == name {
			return true
		}
	}
	return false
}

func Sync(ctx context.Context, client kubernetes.Interface, rootCA, signingCA, servingCA []byte, changed []string) (*Result, error) {
	if err := ensureServiceIPAddresses(ctx, client, changed); err != nil {
		return nil, err
	}
	if err := releaseVolumeAttributesClasses(ctx, client, changed); err != nil {
		return nil, err
	}
	if err := releaseStorageProtection(ctx, client, changed); err != nil {
		return nil, err
	}
	result, err := syncPass(ctx, client, rootCA, signingCA, servingCA, changed, maxDrain)
	controllers, _ := wanted(changed)
	if err != nil {
		return result, err
	}
	if needsFollowUp(controllers) {
		follow, err := syncPass(ctx, client, rootCA, signingCA, servingCA, followUpChanged(controllers), 15*time.Second)
		if err != nil {
			return result, err
		}
		for k, v := range follow.Objects {
			result.Objects[k] = v
		}
		result.Drained = result.Drained && follow.Drained
		if follow.NextMs > 0 && (result.NextMs <= 0 || follow.NextMs < result.NextMs) {
			result.NextMs = follow.NextMs
		}
	}
	if controllers["statefulset"] && changedHas(changed, "pods") {
		result.NextMs = soonest(result.NextMs, 2*time.Second)
	}
	return result, nil
}

func syncPass(ctx context.Context, client kubernetes.Interface, rootCA, signingCA, servingCA []byte, changed []string, drainFor time.Duration) (*Result, error) {
	controllers, needed := wanted(changed)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	work.reset(workloadQueue)
	factory := informers.NewSharedInformerFactory(client, 0)
	result := &Result{Objects: map[string]int{}}
	src := sources(client)
	if needed != nil {
		kept := src[:0]
		for _, s := range src {
			if needed[s.name] {
				kept = append(kept, s)
			}
		}
		src = kept
	}
	loaded := make([][]runtime.Object, len(src))
	var listErr error
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, s := range src {
		wg.Add(1)
		go func() {
			defer wg.Done()
			objs, err := list(ctx, s.page)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				listErr = err
				return
			}
			loaded[i] = objs
		}()
	}
	wg.Wait()
	if listErr != nil {
		return nil, listErr
	}
	var all []loadedSource
	for i, s := range src {
		objs := loaded[i]
		if s.name == "jobs" {
			if anyUnfinished(objs) {
				result.NextMs = unfinishedJobRecheck.Milliseconds()
			}
			if next, ok := nextJobTTL(objs); ok {
				result.NextMs = soonest(result.NextMs, next)
			}
		}
		if s.name == "cronjobs" {
			if next, ok := nextCronRun(objs); ok {
				result.NextMs = soonest(result.NextMs, next)
			}
		}
		result.Objects[s.name] = len(objs)
		all = append(all, loadedSource{register(factory, s.example), objs})
	}
	if err := recordCronChildren(ctx, client, loadedNamed(src, loaded, "cronjobs"), loadedNamed(src, loaded, "jobs")); err != nil {
		println("workloads: cronjob active list:", err.Error())
	}
	client = observeWrites(client, all)

	apps, core := factory.Apps().V1(), factory.Core().V1()
	runs := []func(context.Context){}
	if needed == nil || needed["configmaps"] {
		ensureClusterInfo(ctx, client, rootCA)
	}
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

	for _, l := range all {
		l.informer.fill(l.objs)
	}
	for _, l := range all {
		l.informer.replay(l.objs)
	}
	done := make(chan struct{}, len(runs))
	for _, run := range runs {
		go func() { run(ctx); done <- struct{}{} }()
	}

	if err := clearRecoveredNodes(ctx, client, nodesOf(all)); err != nil {
		println("workloads: clearing node taints failed:", err.Error())
	}
	result.Drained = drain(workloadQueue, drainFor)
	cancel()
	for range runs {
		<-done
	}
	return result, nil
}

func register(factory informers.SharedInformerFactory, example runtime.Object) *snapshotInformer {
	s := newSnapshotInformer(example)
	factory.InformerFor(example, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer { return s })
	return s
}

type cidrSnapshot struct{ cache.SharedIndexInformer }

func (c cidrSnapshot) Informer() cache.SharedIndexInformer { return c.SharedIndexInformer }
func (c cidrSnapshot) Lister() netlisters.ServiceCIDRLister {
	return netlisters.NewServiceCIDRLister(c.GetIndexer())
}

type ipSnapshot struct{ cache.SharedIndexInformer }

func (i ipSnapshot) Informer() cache.SharedIndexInformer { return i.SharedIndexInformer }
func (i ipSnapshot) Lister() netlisters.IPAddressLister {
	return netlisters.NewIPAddressLister(i.GetIndexer())
}

func serviceCIDRSnapshot(factory informers.SharedInformerFactory) cidrSnapshot {
	return cidrSnapshot{factory.InformerFor(&networkingv1.ServiceCIDR{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
		return newSnapshotInformer(&networkingv1.ServiceCIDR{})
	})}
}

func ipAddressSnapshot(factory informers.SharedInformerFactory) ipSnapshot {
	return ipSnapshot{factory.InformerFor(&networkingv1.IPAddress{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
		return newSnapshotInformer(&networkingv1.IPAddress{})
	})}
}

type vapSnapshot struct{ cache.SharedIndexInformer }

func (v vapSnapshot) Informer() cache.SharedIndexInformer { return v.SharedIndexInformer }
func (v vapSnapshot) Lister() admissionlisters.ValidatingAdmissionPolicyLister {
	return admissionlisters.NewValidatingAdmissionPolicyLister(v.GetIndexer())
}

func validatingAdmissionPolicySnapshot(factory informers.SharedInformerFactory) vapSnapshot {
	return vapSnapshot{factory.InformerFor(&admissionregistrationv1.ValidatingAdmissionPolicy{}, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
		return newSnapshotInformer(&admissionregistrationv1.ValidatingAdmissionPolicy{})
	})}
}

func list(ctx context.Context, page pageFunc) ([]runtime.Object, error) {
	var out []runtime.Object
	opts := metav1.ListOptions{Limit: listPage}
	for {
		l, err := page(ctx, opts)
		if err != nil {
			return nil, err
		}
		items, err := meta.ExtractList(l)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		lm, err := meta.ListAccessor(l)
		if err != nil {
			return nil, err
		}
		if lm.GetContinue() == "" {
			return out, nil
		}
		opts.Continue = lm.GetContinue()
	}
}

func nextJobTTL(objs []runtime.Object) (time.Duration, bool) {
	var soonestRun time.Duration
	found := false
	for _, o := range objs {
		j := o.(*batchv1.Job)
		if j.DeletionTimestamp != nil || j.Spec.TTLSecondsAfterFinished == nil {
			continue
		}
		var finish time.Time
		for _, c := range j.Status.Conditions {
			if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == v1.ConditionTrue {
				finish = c.LastTransitionTime.Time
				break
			}
		}
		if finish.IsZero() {
			continue
		}
		wait := time.Until(finish.Add(time.Duration(*j.Spec.TTLSecondsAfterFinished) * time.Second))
		if wait < time.Second {
			wait = time.Second
		}
		if wait > maxDelay {
			wait = maxDelay
		}
		if !found || wait < soonestRun {
			soonestRun = wait
			found = true
		}
	}
	return soonestRun, found
}

func anyUnfinished(objs []runtime.Object) bool {
	for _, o := range objs {
		j := o.(*batchv1.Job)
		finished := false
		for _, c := range j.Status.Conditions {
			if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == v1.ConditionTrue {
				finished = true
			}
		}
		if !finished && j.DeletionTimestamp == nil {
			return true
		}
	}
	return false
}

func drain(owned func(string) bool, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	defer func() {
		if busy := work.busy(owned); busy != "" {
			println("workloads: drain gave up with", busy)
		}
	}()
	quiet := 0
	for time.Now().Before(deadline) {
		time.Sleep(drainPoll)
		if work.idle(owned) {
			quiet++
		} else {
			quiet = 0
		}
		if quiet >= 2 {
			return true
		}
	}
	return false
}

func nextCronRun(objs []runtime.Object) (time.Duration, bool) {
	var soonestRun time.Duration
	found := false
	now := time.Now()
	for _, o := range objs {
		cj := o.(*batchv1.CronJob)
		if cj.DeletionTimestamp != nil || (cj.Spec.Suspend != nil && *cj.Spec.Suspend) {
			continue
		}
		next, ok := nextSchedule(cj, now)
		if !ok {
			continue
		}
		wait := time.Until(next)
		if wait < time.Second {
			wait = time.Second
		}
		if wait > maxDelay {
			wait = maxDelay
		}
		if !found || wait < soonestRun {
			soonestRun = wait
			found = true
		}
	}
	return soonestRun, found
}

func soonest(currentMs int64, next time.Duration) int64 {
	nextMs := next.Milliseconds()
	if currentMs <= 0 || nextMs < currentMs {
		return nextMs
	}
	return currentMs
}

func nextSchedule(cj *batchv1.CronJob, now time.Time) (time.Time, bool) {
	schedule := cj.Spec.Schedule
	if cj.Spec.TimeZone != nil && *cj.Spec.TimeZone != "" {
		schedule = "TZ=" + *cj.Spec.TimeZone + " " + schedule
	}
	parsed, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, false
	}
	next := parsed.Next(now)
	if next.IsZero() {
		return time.Time{}, false
	}
	return next, true
}

func nodesOf(all []loadedSource) []*v1.Node {
	for _, l := range all {
		if len(l.objs) == 0 {
			continue
		}
		if _, ok := l.objs[0].(*v1.Node); !ok {
			continue
		}
		nodes := make([]*v1.Node, 0, len(l.objs))
		for _, o := range l.objs {
			nodes = append(nodes, o.(*v1.Node))
		}
		return nodes
	}
	return nil
}

func clearRecoveredNodes(ctx context.Context, client kubernetes.Interface, nodes []*v1.Node) error {
	for _, node := range nodes {
		held, known, _ := nodeLeaseState(ctx, client, node.Name)
		if !nodeReady(node) && !(held && known) {
			continue
		}
		fresh := node.DeepCopy()
		taints := removeUnreachableTaints(fresh)
		ready := restoreReady(fresh)
		if taints {
			var err error
			if fresh, err = client.CoreV1().Nodes().Update(ctx, fresh, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
		if ready {
			if _, err := client.CoreV1().Nodes().UpdateStatus(ctx, fresh, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func scaleMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion, appsv1.SchemeGroupVersion})
	m.Add(v1.SchemeGroupVersion.WithKind("ReplicationController"), meta.RESTScopeNamespace)
	m.Add(appsv1.SchemeGroupVersion.WithKind("ReplicaSet"), meta.RESTScopeNamespace)
	m.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
	m.Add(appsv1.SchemeGroupVersion.WithKind("StatefulSet"), meta.RESTScopeNamespace)
	m.Add(batchv1.SchemeGroupVersion.WithKind("Job"), meta.RESTScopeNamespace)
	return m
}

type clientScales struct{ client kubernetes.Interface }

func (s clientScales) Scales(namespace string) scale.ScaleInterface {
	return clientScale{client: s.client, namespace: namespace}
}

type clientScale struct {
	client    kubernetes.Interface
	namespace string
}

func (s clientScale) Get(ctx context.Context, resource schema.GroupResource, name string, opts metav1.GetOptions) (*autoscalingv1.Scale, error) {
	switch resource.Resource {
	case "replicationcontrollers":
		obj, err := s.client.CoreV1().ReplicationControllers(s.namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "replicasets":
		obj, err := s.client.AppsV1().ReplicaSets(s.namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "deployments":
		obj, err := s.client.AppsV1().Deployments(s.namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "statefulsets":
		obj, err := s.client.AppsV1().StatefulSets(s.namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "jobs":
		obj, err := s.client.BatchV1().Jobs(s.namespace).Get(ctx, name, opts)
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Parallelism), nil
	default:
		return nil, apierrors.NewNotFound(resource, name)
	}
}

func (s clientScale) Update(ctx context.Context, resource schema.GroupResource, newScale *autoscalingv1.Scale, _ metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	if newScale == nil {
		return nil, apierrors.NewBadRequest("scale is required")
	}
	return s.setReplicas(ctx, resource, newScale.Name, newScale.Spec.Replicas)
}

func (s clientScale) Patch(ctx context.Context, gvr schema.GroupVersionResource, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions) (*autoscalingv1.Scale, error) {
	n, err := replicasFromPatch(pt, data)
	if err != nil {
		return nil, err
	}
	return s.setReplicas(ctx, gvr.GroupResource(), name, n)
}

func (s clientScale) setReplicas(ctx context.Context, resource schema.GroupResource, name string, replicas int32) (*autoscalingv1.Scale, error) {
	switch resource.Resource {
	case "replicationcontrollers":
		obj, err := s.client.CoreV1().ReplicationControllers(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		obj.Spec.Replicas = &replicas
		obj, err = s.client.CoreV1().ReplicationControllers(s.namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "replicasets":
		obj, err := s.client.AppsV1().ReplicaSets(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		obj.Spec.Replicas = &replicas
		obj, err = s.client.AppsV1().ReplicaSets(s.namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "deployments":
		obj, err := s.client.AppsV1().Deployments(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		obj.Spec.Replicas = &replicas
		obj, err = s.client.AppsV1().Deployments(s.namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "statefulsets":
		obj, err := s.client.AppsV1().StatefulSets(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		obj.Spec.Replicas = &replicas
		obj, err = s.client.AppsV1().StatefulSets(s.namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Replicas), nil
	case "jobs":
		obj, err := s.client.BatchV1().Jobs(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		obj.Spec.Parallelism = &replicas
		obj, err = s.client.BatchV1().Jobs(s.namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return scaleFrom(obj.Name, obj.Namespace, obj.UID, obj.Spec.Parallelism), nil
	default:
		return nil, apierrors.NewNotFound(resource, name)
	}
}

func replicasFromPatch(pt types.PatchType, data []byte) (int32, error) {
	switch pt {
	case types.MergePatchType, types.StrategicMergePatchType:
		var body struct {
			Spec struct {
				Replicas *int32 `json:"replicas"`
			} `json:"spec"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return 0, apierrors.NewBadRequest(err.Error())
		}
		if body.Spec.Replicas == nil {
			return 0, apierrors.NewBadRequest("spec.replicas is required")
		}
		return *body.Spec.Replicas, nil
	case types.JSONPatchType:
		var ops []struct {
			Op    string          `json:"op"`
			Path  string          `json:"path"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &ops); err != nil {
			return 0, apierrors.NewBadRequest(err.Error())
		}
		for _, op := range ops {
			if op.Path != "/spec/replicas" {
				continue
			}
			var n int32
			if err := json.Unmarshal(op.Value, &n); err != nil {
				return 0, apierrors.NewBadRequest(err.Error())
			}
			return n, nil
		}
		return 0, apierrors.NewBadRequest("spec.replicas is required")
	default:
		return 0, apierrors.NewBadRequest("unsupported patch type")
	}
}

func scaleFrom(name, namespace string, uid types.UID, replicas *int32) *autoscalingv1.Scale {
	n := int32(1)
	if replicas != nil {
		n = *replicas
	}
	return &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: uid},
		Spec:       autoscalingv1.ScaleSpec{Replicas: n},
	}
}

func nodeReady(node *v1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady {
			return c.Status == v1.ConditionTrue
		}
	}
	return false
}

type loadedSource struct {
	informer *snapshotInformer
	objs     []runtime.Object
}

type csrInformer struct{ inf cache.SharedIndexInformer }

func (c csrInformer) Informer() cache.SharedIndexInformer { return c.inf }

func (c csrInformer) Lister() certlisters.CertificateSigningRequestLister {
	return certlisters.NewCertificateSigningRequestLister(c.inf.GetIndexer())
}

type clusterRoleInformer struct{ inf cache.SharedIndexInformer }

func (c clusterRoleInformer) Informer() cache.SharedIndexInformer { return c.inf }

func (c clusterRoleInformer) Lister() rbaclisters.ClusterRoleLister {
	return rbaclisters.NewClusterRoleLister(c.inf.GetIndexer())
}
