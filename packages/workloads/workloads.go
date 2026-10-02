package workloads

import (
	"context"
	"encoding/json"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	admissionlisters "k8s.io/client-go/listers/admissionregistration/v1"
	certlisters "k8s.io/client-go/listers/certificates/v1"
	netlisters "k8s.io/client-go/listers/networking/v1"
	rbaclisters "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	clientretry "k8s.io/client-go/util/retry"
)

const (
	workers              = 5
	listPage             = 500
	drainPoll            = 200 * time.Millisecond
	maxDrain             = 10 * time.Second
	inFlightGrace        = 150 * time.Second
	followGrace          = 30 * time.Second
	followDrain          = 5 * time.Second
	replicaRetry         = 5 * time.Second
	syncLockWait         = 10 * time.Second
	listBudget           = 30 * time.Second
	listConcurrency      = 6
	syncBusyRetry        = 2 * time.Second
	yieldGrace           = time.Second
	shutdownGrace        = 5 * time.Second
	maxEndpointsPerSlice = 100
	daemonSetWorkers     = 2
	unfinishedJobRecheck = 10 * time.Second
	taintClearRetry      = 2 * time.Second
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
	Live    bool           `json:"live"`
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
	"serviceaccounttoken":       {"secrets", "serviceaccounts"},
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

var syncMu sync.Mutex

var yieldRequested atomic.Bool

func lockSync(ctx context.Context, budget time.Duration) bool {
	wait := syncLockWait
	if budget > 0 && budget/4 < wait {
		wait = budget / 4
	}
	deadline := time.Now().Add(wait)
	for {
		if syncMu.TryLock() {
			yieldRequested.Store(false)
			return true
		}
		yieldRequested.Store(true)
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func Sync(ctx context.Context, client kubernetes.Interface, rootCA, signingCA, servingCA []byte, changed []string) (*Result, error) {
	return SyncWithin(ctx, client, rootCA, signingCA, servingCA, changed, 0, nil)
}

func SyncWithin(ctx context.Context, client kubernetes.Interface, rootCA, signingCA, servingCA []byte, changed []string, budget time.Duration, holdWhileRunning func(context.Context) func()) (*Result, error) {
	var deadline time.Time
	if budget > 0 {
		deadline = time.Now().Add(budget)
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	if err := releaseProtection(ctx, client, changed); err != nil {
		return nil, err
	}
	if !lockSync(ctx, budget) {
		return &Result{Objects: map[string]int{}, NextMs: int64(syncBusyRetry / time.Millisecond)}, nil
	}
	defer syncMu.Unlock()
	if holdWhileRunning != nil {
		defer holdWhileRunning(ctx)()
	}
	if err := ensureServiceIPAddresses(ctx, client, changed); err != nil {
		return nil, err
	}
	controllers, _ := wanted(changed)
	result, err := syncPass(ctx, client, rootCA, signingCA, servingCA, changed, maxDrain, inFlightGrace, deadline)
	if err != nil {
		return result, err
	}
	if needsFollowUp(controllers) && !result.Live {
		follow, err := syncPass(ctx, client, rootCA, signingCA, servingCA, followUpChanged(controllers), followDrain, followGrace, deadline)
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
	if controllers["statefulset"] && changedHas(changed, "pods") && result.Objects["statefulsets"] > 0 {
		if gap, err := statefulSetGap(ctx, client); err != nil || gap {
			result.NextMs = soonest(result.NextMs, 2*time.Second)
		}
	}
	if retry, ok := replicaGap(ctx, client, controllers); ok {
		result.NextMs = soonest(result.NextMs, retry)
	}
	return result, nil
}

func releaseProtection(ctx context.Context, client kubernetes.Interface, changed []string) error {
	if err := releaseVolumeAttributesClasses(ctx, client, changed); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	if err := releaseStorageProtection(ctx, client, changed); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}

func replicaControllers(controllers map[string]bool) bool {
	return controllers["replication"] || controllers["replicaset"] || controllers["deployment"]
}

func replicaGap(ctx context.Context, client kubernetes.Interface, controllers map[string]bool) (time.Duration, bool) {
	if controllers["replication"] {
		gap, err := replicationGap(ctx, client)
		if err != nil || gap {
			return replicaRetry, true
		}
	}
	if controllers["replicaset"] || controllers["deployment"] {
		gap, err := replicaSetGap(ctx, client)
		if err != nil || gap {
			return replicaRetry, true
		}
	}
	if controllers["deployment"] {
		gap, err := deploymentGap(ctx, client)
		if err != nil || gap {
			return replicaRetry, true
		}
	}
	return 0, false
}

func replicationGap(ctx context.Context, client kubernetes.Interface) (bool, error) {
	list, err := client.CoreV1().ReplicationControllers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for i := range list.Items {
		rc := &list.Items[i]
		if rc.DeletionTimestamp != nil || rc.Spec.Replicas == nil {
			continue
		}
		if rc.Status.Replicas != *rc.Spec.Replicas {
			return true, nil
		}
	}
	return false, nil
}

func replicaSetGap(ctx context.Context, client kubernetes.Interface) (bool, error) {
	list, err := client.AppsV1().ReplicaSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for i := range list.Items {
		rs := &list.Items[i]
		if rs.DeletionTimestamp != nil || rs.Spec.Replicas == nil {
			continue
		}
		if rs.Status.Replicas != *rs.Spec.Replicas {
			return true, nil
		}
	}
	return false, nil
}

func deploymentGap(ctx context.Context, client kubernetes.Interface) (bool, error) {
	list, err := client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for i := range list.Items {
		dep := &list.Items[i]
		if dep.DeletionTimestamp != nil || dep.Spec.Replicas == nil {
			continue
		}
		if dep.Status.Replicas != *dep.Spec.Replicas {
			return true, nil
		}
	}
	return false, nil
}

func statefulSetGap(ctx context.Context, client kubernetes.Interface) (bool, error) {
	list, err := client.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for i := range list.Items {
		set := &list.Items[i]
		if set.DeletionTimestamp != nil || set.Spec.Replicas == nil {
			continue
		}
		want := *set.Spec.Replicas
		status := set.Status
		if status.ObservedGeneration < set.Generation || status.Replicas != want || status.ReadyReplicas != want || status.UpdatedReplicas < rollingUpdateTarget(set) {
			return true, nil
		}
	}
	return false, nil
}

func rollingUpdateTarget(set *appsv1.StatefulSet) int32 {
	strategy := set.Spec.UpdateStrategy
	if strategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return 0
	}
	target := *set.Spec.Replicas
	if strategy.RollingUpdate != nil && strategy.RollingUpdate.Partition != nil {
		target -= *strategy.RollingUpdate.Partition
	}
	return target
}

func syncPass(ctx context.Context, client kubernetes.Interface, rootCA, signingCA, servingCA []byte, changed []string, drainFor, grace time.Duration, deadline time.Time) (*Result, error) {
	controllers, needed := wanted(changed)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	passStart := time.Now()
	stage := passStart
	took := func(name string) {
		println("workloads: stage", name, time.Since(stage).Round(time.Millisecond).String())
		stage = time.Now()
	}

	work.reset(workloadQueue)
	pending.reset()
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
	listCtx, listCancel := context.WithTimeout(ctx, listBudget)
	defer listCancel()
	loaded, revisions, listErr := listSources(listCtx, src)
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
		informer := register(ctx, factory, s.example)
		informer.ownersSyncedInPass = s.name == "endpointslices"
		all = append(all, loadedSource{informer, objs})
	}
	if err := recordCronChildren(ctx, client, loadedNamed(src, loaded, "cronjobs"), loadedNamed(src, loaded, "jobs")); err != nil {
		println("workloads: cronjob active list:", err.Error())
	}
	took("lists")
	client = observeWrites(client, all)

	if needed == nil || needed["configmaps"] {
		ensureClusterInfo(ctx, client, rootCA)
	}
	runs, err := Deps{Client: client, Factory: factory, RootCA: rootCA, SigningCA: signingCA, ServingCA: servingCA, ServiceAccountKey: ServiceAccountKey}.buildShards(ctx, controllers)
	if err != nil {
		return nil, err
	}

	for _, l := range all {
		l.informer.fill(l.objs)
	}
	for _, l := range all {
		l.informer.replay(l.objs)
	}
	took("build+fill")
	feed := followWrites(ctx, all, revisions)
	done := make(chan struct{}, len(runs))
	for _, run := range runs {
		go func() { run(ctx); done <- struct{}{} }()
	}

	if err := clearRecoveredNodes(ctx, client, nodesOf(all)); err != nil {
		println("workloads: clearing node taints failed:", err.Error())
		result.NextMs = soonest(result.NextMs, taintClearRetry)
	}
	if controllers["tainteviction"] {
		if err := evictTaintedPods(ctx, client, nodesOf(all), podsOf(all), time.Now()); err != nil {
			println("workloads: taint eviction failed:", err.Error())
		}
	}
	result.Drained = drain(workloadQueue, drainFor, grace, deadline, feed.live)
	result.Live = feed.live()
	took("drain")
	println("workloads: live feed", strconv.FormatBool(result.Live), "applied", strconv.FormatInt(feed.count(), 10))
	if next, ok := pending.next(); ok {
		result.NextMs = soonest(result.NextMs, next)
	}
	cancel()
	stopping := time.NewTimer(shutdownGrace)
	defer stopping.Stop()
	for i := range runs {
		select {
		case <-done:
		case <-stopping.C:
			println("workloads: controllers did not stop within", shutdownGrace.String(),
				"- leaving", strconv.Itoa(len(runs)-i), "of", strconv.Itoa(len(runs)), "behind")
			took("shutdown")
			return result, nil
		}
	}
	took("shutdown")
	println("workloads: pass total", time.Since(passStart).Round(time.Millisecond).String(), "controllers", strconv.Itoa(len(runs)))
	return result, nil
}

func register(ctx context.Context, factory informers.SharedInformerFactory, example runtime.Object) *snapshotInformer {
	s := newSnapshotInformer(example)
	s.passCtx = ctx
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

func listSources(ctx context.Context, src []source) ([][]runtime.Object, []int64, error) {
	loaded := make([][]runtime.Object, len(src))
	revisions := make([]int64, len(src))
	var listErr error
	var wg sync.WaitGroup
	var mu sync.Mutex
	slots := make(chan struct{}, listConcurrency)
	for i, s := range src {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			objs, revision, err := list(ctx, s.page)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				listErr = err
				return
			}
			loaded[i] = objs
			revisions[i] = revision
		}()
	}
	wg.Wait()
	return loaded, revisions, listErr
}

func list(ctx context.Context, page pageFunc) ([]runtime.Object, int64, error) {
	var out []runtime.Object
	var revision int64
	opts := metav1.ListOptions{Limit: listPage}
	for {
		l, err := page(ctx, opts)
		if err != nil {
			return nil, 0, err
		}
		items, err := meta.ExtractList(l)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, items...)
		lm, err := meta.ListAccessor(l)
		if err != nil {
			return nil, 0, err
		}
		if revision == 0 {
			revision, _ = strconv.ParseInt(lm.GetResourceVersion(), 10, 64)
		}
		if lm.GetContinue() == "" {
			return out, revision, nil
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

func drain(owned func(string) bool, limit, grace time.Duration, budgetBy time.Time, live func() bool) bool {
	started := time.Now()
	deadline := started.Add(limit)
	finishBy := deadline.Add(grace)
	if !budgetBy.IsZero() && budgetBy.Before(finishBy) {
		finishBy = budgetBy
		if deadline.After(finishBy) {
			deadline = finishBy
		}
	}
	defer func() {
		if busy := work.busy(owned); busy != "" {
			println("workloads: drain gave up after", time.Since(started).String(), "with", busy)
			if work.inFlight(owned) {
				println(inFlightStacks())
			}
		}
	}()
	quiet := 0
	for {
		time.Sleep(drainPoll)
		if work.idle(owned) {
			quiet++
		} else {
			quiet = 0
		}
		if quiet >= 2 {
			return true
		}
		now := time.Now()
		if now.After(finishBy) {
			return false
		}
		yielding := !live() && yieldRequested.Load() && now.Sub(started) > yieldGrace
		if (now.After(deadline) || yielding) && !work.inFlight(owned) && !work.idle(owned) {
			return false
		}
	}
}

func inFlightStacks() string {
	buf := make([]byte, 8<<20)
	buf = buf[:goruntime.Stack(buf, true)]
	var held []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "processNext") && !strings.Contains(g, "workqueue.(*Typed") {
			held = append(held, g)
		}
	}
	return "workloads: in-flight handler stacks (" + strconv.Itoa(len(held)) + "):\n" + strings.Join(held, "\n\n")
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
		stale := node.DeepCopy()
		if !removeUnreachableTaints(stale) && !restoreReady(stale) {
			continue
		}
		var fresh *v1.Node
		ready := false
		err := clientretry.RetryOnConflict(clientretry.DefaultRetry, func() error {
			current, err := client.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			fresh = current.DeepCopy()
			taints := removeUnreachableTaints(fresh)
			ready = restoreReady(fresh)
			if !taints {
				return nil
			}
			updated, err := client.CoreV1().Nodes().Update(ctx, fresh, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			fresh = updated
			return nil
		})
		if err != nil {
			return err
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
