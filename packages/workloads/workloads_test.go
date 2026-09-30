package workloads

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
)

func TestWantedSelectsControllersForChangedResources(t *testing.T) {
	controllers, needed := wanted([]string{"deployments"})
	if !controllers["deployment"] || !controllers["replicaset"] || !controllers["resourcequota"] || controllers["job"] {
		t.Fatalf("controllers = %v", controllers)
	}
	for _, want := range []string{"pods", "replicasets", "deployments", "cronjobs", "jobs", "resourcequotas"} {
		if !needed[want] {
			t.Fatalf("needed is missing %s: %v", want, needed)
		}
	}
	all, none := wanted(nil)
	if len(all) != len(controllerNeeds) || none != nil {
		t.Fatalf("an empty batch must run everything: %d %v", len(all), none)
	}
}

func TestWantedSelectsBootstrapSigner(t *testing.T) {
	controllers, needed := wanted([]string{"configmaps"})
	if !controllers["bootstrapsigner"] || !needed["configmaps"] || !needed["secrets"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
}

func TestWantedSelectsTokenCleaner(t *testing.T) {
	controllers, needed := wanted([]string{"secrets"})
	if !controllers["tokencleaner"] || !needed["secrets"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
}

func TestWantedSelectsValidatingAdmissionPolicy(t *testing.T) {
	controllers, needed := wanted([]string{"validatingadmissionpolicies"})
	if !controllers["validatingadmissionpolicy"] || !needed["validatingadmissionpolicies"] {
		t.Fatal("policy write did not select validating admission policy status")
	}
}

func TestWantedSelectsServiceCIDR(t *testing.T) {
	controllers, needed := wanted([]string{"servicecidrs"})
	if !controllers["servicecidr"] || !needed["ipaddresses"] {
		t.Fatal("servicecidr write did not select the controller")
	}
}

func TestWantedSelectsDeviceTaintEviction(t *testing.T) {
	controllers, needed := wanted([]string{"resourceslices"})
	if !controllers["devicetainteviction"] || !needed["deviceclasses"] || !needed["pods"] {
		t.Fatal("resource slice write did not select device taint eviction")
	}
}

func TestWantedSelectsResourceClaim(t *testing.T) {
	controllers, needed := wanted([]string{"pods"})
	if !controllers["resourceclaim"] || !needed["resourceclaims"] || !needed["resourceclaimtemplates"] {
		t.Fatal("pod write did not select resource claim")
	}
}

func TestWantedSelectsNodeTTL(t *testing.T) {
	controllers, needed := wanted([]string{"nodes"})
	if !controllers["ttl"] || !needed["nodes"] {
		t.Fatal("node write did not select ttl")
	}
}

func TestWantedSelectsLegacyTokenCleaner(t *testing.T) {
	controllers, needed := wanted([]string{"secrets"})
	if !controllers["legacytoken"] || !needed["serviceaccounts"] || !needed["pods"] {
		t.Fatal("secret write did not select legacy token cleaner")
	}
}

func TestWantedSelectsVolumeExpand(t *testing.T) {
	controllers, needed := wanted([]string{"persistentvolumeclaims"})
	if !controllers["volumeexpand"] || !needed["persistentvolumes"] {
		t.Fatal("pvc write did not select volume expand")
	}
}

func TestWantedSelectsPodGC(t *testing.T) {
	controllers, needed := wanted([]string{"pods"})
	if !controllers["podgc"] || !needed["pods"] || !needed["nodes"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
	mapped, _ := wanted([]string{"minions"})
	if !mapped["podgc"] {
		t.Fatalf("mapped = %v", mapped)
	}
}

func TestWantedSelectsNodeIPAM(t *testing.T) {
	controllers, needed := wanted([]string{"nodes"})
	if !controllers["nodeipam"] || !needed["nodes"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
}

func TestWantedSelectsTaintEviction(t *testing.T) {
	controllers, needed := wanted([]string{"nodes"})
	if !controllers["tainteviction"] || !needed["nodes"] || !needed["pods"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
	mapped, _ := wanted([]string{"minions"})
	if !mapped["tainteviction"] {
		t.Fatalf("mapped = %v", mapped)
	}
}

func TestWantedSelectsEphemeralVolume(t *testing.T) {
	controllers, needed := wanted([]string{"pods"})
	if !controllers["ephemeralvolume"] || !needed["pods"] || !needed["persistentvolumeclaims"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
}

func TestWantedSelectsVolumeProtection(t *testing.T) {
	pvc, needed := wanted([]string{"persistentvolumeclaims"})
	if !pvc["pvcprotection"] || !needed["persistentvolumeclaims"] || !needed["pods"] {
		t.Fatalf("pvc = %v needed = %v", pvc, needed)
	}
	pv, _ := wanted([]string{"persistentvolumes"})
	if !pv["pvprotection"] {
		t.Fatalf("pv = %v", pv)
	}
}

func TestWantedSelectsQuotaAndJobDisruption(t *testing.T) {
	quota, needed := wanted([]string{"jobs"})
	if !quota["resourcequota"] || !needed["jobs"] {
		t.Fatalf("quota controllers=%v needed=%v", quota, needed)
	}
	if !quota["disruption"] || !needed["poddisruptionbudgets"] {
		t.Fatalf("disruption controllers=%v needed=%v", quota, needed)
	}
	cron, _ := wanted([]string{"cronjobs"})
	if !cron["resourcequota"] {
		t.Fatalf("cron controllers=%v", cron)
	}
	accounts, neededAccounts := wanted([]string{"serviceaccounts"})
	if !accounts["resourcequota"] || !neededAccounts["serviceaccounts"] {
		t.Fatalf("accounts controllers=%v needed=%v", accounts, neededAccounts)
	}
	if _, ok := quotaExample(v1.SchemeGroupVersion.WithResource("serviceaccounts")); !ok {
		t.Fatal("serviceaccounts")
	}
	net, neededNet := wanted([]string{"networking.k8s.io"})
	if !net["resourcequota"] || !neededNet["ingresses"] || !neededNet["networkpolicies"] {
		t.Fatalf("net controllers=%v needed=%v", net, neededNet)
	}
	rbac, neededRBAC := wanted([]string{"roles"})
	if !rbac["resourcequota"] || !neededRBAC["roles"] || !neededRBAC["rolebindings"] {
		t.Fatalf("rbac controllers=%v needed=%v", rbac, neededRBAC)
	}
	limits, neededLimits := wanted([]string{"limitranges"})
	if !limits["resourcequota"] || !neededLimits["limitranges"] || !neededLimits["endpoints"] {
		t.Fatalf("limits controllers=%v needed=%v", limits, neededLimits)
	}
}

func TestWantedSelectsTTLAfterFinished(t *testing.T) {
	controllers, needed := wanted([]string{"jobs"})
	if !controllers["ttlafterfinished"] || !needed["jobs"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
}

func TestNextJobTTLUsesFinishTime(t *testing.T) {
	ttl := int32(30)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: "default"},
		Spec:       batchv1.JobSpec{TTLSecondsAfterFinished: &ttl},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
			Type:               batchv1.JobComplete,
			Status:             v1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-5 * time.Second)),
		}}},
	}
	got, ok := nextJobTTL([]runtime.Object{job})
	if !ok || got < 20*time.Second || got > 30*time.Second {
		t.Fatalf("next = %v ok = %v", got, ok)
	}
}

func TestWantedSelectsClusterRoleAggregation(t *testing.T) {
	controllers, needed := wanted([]string{"clusterroles"})
	if !controllers["clusterroleaggregation"] {
		t.Fatalf("controllers = %v", controllers)
	}
	if !needed["clusterroles"] {
		t.Fatalf("needed = %v", needed)
	}
	mapped, _ := wanted([]string{"rbac.authorization.k8s.io"})
	if mapped["clusterroleaggregation"] {
		t.Fatalf("group token should not run clusterrole aggregation: %v", mapped)
	}
	roles, _ := wanted([]string{"roles"})
	if !roles["resourcequota"] || roles["clusterroleaggregation"] {
		t.Fatalf("roles = %v", roles)
	}
}

func TestFollowUpIncludesDaemonSetStatus(t *testing.T) {
	controllers, _ := wanted([]string{"daemonsets"})
	if !needsFollowUp(controllers) {
		t.Fatal("daemonset sync must follow up so status can observe created pods")
	}
	got := followUpChanged(controllers)
	for _, want := range []string{"pods", "daemonsets", "nodes"} {
		found := false
		for _, name := range got {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("follow-up missing %s: %v", want, got)
		}
	}
}

func TestFollowUpIncludesJobStatus(t *testing.T) {
	controllers, _ := wanted([]string{"jobs"})
	if !needsFollowUp(controllers) {
		t.Fatal("job sync must follow up so completion status can land")
	}
	got := followUpChanged(controllers)
	for _, want := range []string{"pods", "jobs"} {
		found := false
		for _, name := range got {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("follow-up missing %s: %v", want, got)
		}
	}
}

func TestWantedSelectsPersistentVolumeBinder(t *testing.T) {
	controllers, needed := wanted([]string{"persistentvolumeclaims"})
	if !controllers["persistentvolume"] || controllers["deployment"] {
		t.Fatalf("controllers = %v", controllers)
	}
	for _, want := range []string{"persistentvolumes", "persistentvolumeclaims", "storageclasses"} {
		if !needed[want] {
			t.Fatalf("needed is missing %s: %v", want, needed)
		}
	}
	grouped, _ := wanted([]string{"storage.k8s.io"})
	if !grouped["persistentvolume"] {
		t.Fatalf("storage.k8s.io should start the binder: %v", grouped)
	}
	csr, neededCSR := wanted([]string{"certificatesigningrequests"})
	if !csr["csrapproving"] || !csr["csrsigning"] || !csr["csrcleaner"] || !neededCSR["certificatesigningrequests"] {
		t.Fatalf("csr controllers = %v needed = %v", csr, neededCSR)
	}
}

func TestWantedSelectsCSRCleaner(t *testing.T) {
	controllers, needed := wanted([]string{"certificatesigningrequests"})
	if !controllers["csrcleaner"] || !needed["certificatesigningrequests"] {
		t.Fatalf("controllers = %v needed = %v", controllers, needed)
	}
}

func TestQueueMetricsTrackInFlight(t *testing.T) {
	work.reset(func(string) bool { return true })
	q := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: "replicationmanager"},
	)
	defer q.ShutDown()
	q.Add("default/pause")
	if work.idle(func(string) bool { return true }) {
		t.Fatal("queued key was invisible to drain")
	}
	key, quit := q.Get()
	if quit || key != "default/pause" {
		t.Fatalf("get=%q quit=%v", key, quit)
	}
	if !work.inFlight(func(string) bool { return true }) {
		t.Fatal("dequeued key was not in flight")
	}
	q.Done(key)
	if work.inFlight(func(string) bool { return true }) {
		t.Fatal("done key stayed in flight")
	}
}

func TestDrainWaitsForInFlightPastDeadline(t *testing.T) {
	work.reset(func(string) bool { return true })
	c := work.of("replicationmanager")
	c.inFlight.Store(1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		c.inFlight.Store(0)
	}()
	started := time.Now()
	drained := drain(func(string) bool { return true }, 50*time.Millisecond, 2*time.Second, time.Time{})
	elapsed := time.Since(started)
	if elapsed < 400*time.Millisecond {
		t.Fatalf("returned after %s while a create was in flight", elapsed)
	}
	if !drained {
		t.Fatal("finished create reported not drained")
	}
}

func TestDrainStopsQueuedWorkAtDeadline(t *testing.T) {
	work.reset(func(string) bool { return true })
	work.of("replicationmanager").depth.Store(1)
	started := time.Now()
	if drain(func(string) bool { return true }, 50*time.Millisecond, 2*time.Second, time.Time{}) {
		t.Fatal("queued work reported drained")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("waited %s for work that had not started", time.Since(started))
	}
}

func TestDrainYieldsToAWaitingSync(t *testing.T) {
	work.reset(func(string) bool { return true })
	work.of("replicationmanager").depth.Store(1)
	yieldRequested.Store(true)
	defer yieldRequested.Store(false)
	started := time.Now()
	if drain(func(string) bool { return true }, time.Minute, time.Minute, time.Time{}) {
		t.Fatal("queued work reported drained")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("held the pass %s after another sync asked for it", time.Since(started))
	}
}

func TestBusySyncAsksTheRunningPassToYield(t *testing.T) {
	syncMu.Lock()
	defer syncMu.Unlock()
	yieldRequested.Store(false)
	if _, err := SyncWithin(context.Background(), fake.NewSimpleClientset(), []byte("ca"), nil, nil, []string{"pods"}, 4*time.Second, nil); err != nil {
		t.Fatal(err)
	}
	if !yieldRequested.Load() {
		t.Fatal("a sync turned away as busy did not ask the running pass to yield")
	}
}

func TestReplicaGapRetriesWhileStatusLags(t *testing.T) {
	replicas := int32(40)
	matched := int32(2)
	client := fake.NewSimpleClientset(
		&v1.ReplicationController{
			ObjectMeta: metav1.ObjectMeta{Name: "burst", Namespace: "default"},
			Spec:       v1.ReplicationControllerSpec{Replicas: &replicas},
			Status:     v1.ReplicationControllerStatus{Replicas: 18},
		},
		&v1.ReplicationController{
			ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: "default"},
			Spec:       v1.ReplicationControllerSpec{Replicas: &matched},
			Status:     v1.ReplicationControllerStatus{Replicas: matched},
		},
	)
	controllers, _ := wanted([]string{"replicationcontrollers"})
	gap, ok := replicaGap(context.Background(), client, controllers)
	if !ok || gap != replicaRetry {
		t.Fatalf("gap=%s ok=%v", gap, ok)
	}
	settled := fake.NewSimpleClientset(&v1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: "default"},
		Spec:       v1.ReplicationControllerSpec{Replicas: &matched},
		Status:     v1.ReplicationControllerStatus{Replicas: matched},
	})
	if _, ok := replicaGap(context.Background(), settled, controllers); ok {
		t.Fatal("converged replication controller asked for another pass")
	}
}

func TestClientScaleReadsDeploymentReplicas(t *testing.T) {
	replicas := int32(3)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "dep"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	})
	got, err := clientScales{client}.Scales("default").Get(context.Background(), schema.GroupResource{Group: "apps", Resource: "deployments"}, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Replicas != 3 || got.UID != "dep" {
		t.Fatalf("scale=%+v", got)
	}
	updated, err := clientScales{client}.Scales("default").Patch(context.Background(), schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "web", types.MergePatchType, []byte(`{"spec":{"replicas":5}}`), metav1.PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Replicas != 5 {
		t.Fatalf("patched=%+v", updated)
	}
}

func TestClientScaleReadsJobParallelism(t *testing.T) {
	parallel := int32(2)
	client := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "batch", Namespace: "default", UID: "job"},
		Spec:       batchv1.JobSpec{Parallelism: &parallel},
	})
	got, err := clientScales{client}.Scales("default").Get(context.Background(), schema.GroupResource{Group: "batch", Resource: "jobs"}, "batch", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Replicas != 2 || got.UID != "job" {
		t.Fatalf("scale=%+v", got)
	}
	kinds, err := scaleMapper().KindsFor(schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"})
	if err != nil || len(kinds) != 1 || kinds[0].Kind != "Job" {
		t.Fatal(kinds, err)
	}
}

func TestDrainStopsWithinTheCallerBudget(t *testing.T) {
	work.reset(workloadQueue)
	defer work.reset(workloadQueue)
	work.of("endpoint_slice").inFlight.Add(1)

	started := time.Now()
	budget := 600 * time.Millisecond
	if drain(workloadQueue, 100*time.Millisecond, time.Hour, started.Add(budget)) {
		t.Fatal("drain reported a drained queue while an item was in flight")
	}
	if waited := time.Since(started); waited > budget+2*time.Second {
		t.Fatalf("drain waited %s, past the %s budget the caller allowed", waited, budget)
	}
}

func TestDrainWithoutBudgetKeepsItsOwnWindow(t *testing.T) {
	work.reset(workloadQueue)
	defer work.reset(workloadQueue)

	if !drain(workloadQueue, time.Second, time.Second, time.Time{}) {
		t.Fatal("drain on an idle queue should report drained")
	}
}

func TestSyncWithinAnswersBusyInsteadOfBlocking(t *testing.T) {
	syncMu.Lock()
	defer syncMu.Unlock()

	started := time.Now()
	result, err := SyncWithin(context.Background(), fake.NewSimpleClientset(), []byte("ca"), nil, nil, []string{"pods"}, 4*time.Second, nil)
	if err != nil {
		t.Fatalf("a busy sync should answer, not fail: %v", err)
	}
	if result == nil || result.NextMs == 0 {
		t.Fatalf("a busy sync should ask to be retried, got %+v", result)
	}
	if waited := time.Since(started); waited > 3*time.Second {
		t.Fatalf("waited %s for a lock another pass holds; the caller would have given up", waited)
	}
}

func TestSyncWithinHoldsOnlyWhileAPassRuns(t *testing.T) {
	held, released := 0, 0
	hold := func(context.Context) func() {
		held++
		return func() { released++ }
	}

	syncMu.Lock()
	_, err := SyncWithin(context.Background(), fake.NewSimpleClientset(), []byte("ca"), nil, nil, []string{"pods"}, 4*time.Second, hold)
	syncMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if held != 0 {
		t.Fatal("a sync turned away as busy held the window")
	}

	if _, err := SyncWithin(context.Background(), fake.NewSimpleClientset(), []byte("ca"), nil, nil, []string{"pods"}, 30*time.Second, hold); err != nil {
		t.Fatal(err)
	}
	if held != 1 || released != 1 {
		t.Fatalf("held=%d released=%d, want one hold released once", held, released)
	}
}
