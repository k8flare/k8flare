package workloads

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/utils/ptr"
)

func creates(client *fake.Clientset, resource string) int {
	n := 0
	for _, a := range client.Actions() {
		if a.GetVerb() == "create" && a.GetResource().Resource == resource {
			n++
		}
	}
	return n
}

func assignGeneratedNames(client *fake.Clientset) {
	uids := 0
	client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if obj, err := meta.Accessor(action.(k8stesting.CreateAction).GetObject()); err == nil {
			if obj.GetUID() == "" {
				uids++
				obj.SetUID(types.UID(fmt.Sprintf("uid-%d", uids)))
			}
			if obj.GetName() == "" && obj.GetGenerateName() != "" {
				uids++
				obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), uids))
			}
		}
		return false, nil, nil
	})
}

func TestSyncCreatesReplicaSetThenPods(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](2),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	uids := 0
	client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if obj, err := meta.Accessor(action.(k8stesting.CreateAction).GetObject()); err == nil && obj.GetUID() == "" {
			uids++
			obj.SetUID(types.UID(fmt.Sprintf("uid-%d", uids)))
			if obj.GetName() == "" {
				obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), uids))
			}
		}
		return false, nil, nil
	})
	rsCreates, podCreates := 0, 0
	for i := 0; i < 4; i++ {
		client.ClearActions()
		if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		rsCreates += creates(client, "replicasets")
		podCreates += creates(client, "pods")
	}
	if rsCreates != 1 || podCreates != 2 {
		t.Fatalf("replicaset creates = %d, pod creates = %d", rsCreates, podCreates)
	}
	client.ClearActions()
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := creates(client, "pods") + creates(client, "replicasets"); got != 0 {
		t.Fatalf("creates on settled state = %d", got)
	}
}

func TestSyncDeploymentBatchCreatesPods(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](2),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	uids := 0
	client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if obj, err := meta.Accessor(action.(k8stesting.CreateAction).GetObject()); err == nil && obj.GetUID() == "" {
			uids++
			obj.SetUID(types.UID(fmt.Sprintf("uid-%d", uids)))
			if obj.GetName() == "" {
				obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), uids))
			}
		}
		return false, nil, nil
	})
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"deployments"}); err != nil {
		t.Fatal(err)
	}
	if gotRS, gotPods := creates(client, "replicasets"), creates(client, "pods"); gotRS != 1 || gotPods != 2 {
		t.Fatalf("replicaset creates = %d, pod creates = %d", gotRS, gotPods)
	}
}

func TestSyncResourceQuotaCalculatesStatus(t *testing.T) {
	rq := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList{v1.ResourceQuotas: resource.MustParse("2")},
		},
		Status: v1.ResourceQuotaStatus{
			Hard: v1.ResourceList{v1.ResourceQuotas: resource.MustParse("9")},
		},
	}
	client := fake.NewSimpleClientset(rq)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"resourcequotas"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().ResourceQuotas("default").Get(context.Background(), "q", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	hard := got.Status.Hard[v1.ResourceQuotas]
	used := got.Status.Used[v1.ResourceQuotas]
	if hard.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("status.hard resourcequotas = %s", hard.String())
	}
	if used.Cmp(resource.MustParse("1")) != 0 {
		t.Fatalf("status.used resourcequotas = %s", used.String())
	}
}

func TestSyncResourceQuotaCountsReplicaSets(t *testing.T) {
	zero := int32(0)
	rq := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "default"},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList{v1.ResourceName("count/replicasets.apps"): resource.MustParse("2")},
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "default"},
		Spec:       appsv1.ReplicaSetSpec{Replicas: &zero},
	}
	client := fake.NewSimpleClientset(rq, rs)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"resourcequotas", "replicasets"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().ResourceQuotas("default").Get(context.Background(), "q", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	used := got.Status.Used[v1.ResourceName("count/replicasets.apps")]
	if used.Cmp(resource.MustParse("1")) != 0 {
		t.Fatalf("status.used count/replicasets.apps = %s status=%v", used.String(), got.Status.Used)
	}
}

func TestSyncDisruptionUpdatesPDBStatus(t *testing.T) {
	labels := map[string]string{"foo": "bar"}
	min := intstr.FromInt32(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", Generation: 1},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &min,
			Selector:     &metav1.LabelSelector{MatchLabels: labels},
		},
	}
	var pods []runtime.Object
	for i := 0; i < 3; i++ {
		pods = append(pods, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("p%d", i), Namespace: "default", Labels: labels},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
				Conditions: []v1.PodCondition{{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				}},
			},
		})
	}
	objs := append([]runtime.Object{pdb}, pods...)
	client := fake.NewSimpleClientset(objs...)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"poddisruptionbudgets"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.PolicyV1().PodDisruptionBudgets("default").Get(context.Background(), "foo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.ObservedGeneration != 1 {
		t.Fatalf("observedGeneration=%d", got.Status.ObservedGeneration)
	}
	if got.Status.DisruptionsAllowed < 1 {
		t.Fatalf("disruptionsAllowed=%d currentHealthy=%d", got.Status.DisruptionsAllowed, got.Status.CurrentHealthy)
	}
}

func TestSyncStatefulSetReadyReplicasFollowsPodReady(t *testing.T) {
	labels := map[string]string{"app": "ss"}
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss", Namespace: "default", UID: "ss1", Generation: 1},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To[int32](1),
			ServiceName: "ss",
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}},
			},
		},
		Status: appsv1.StatefulSetStatus{Replicas: 1, ReadyReplicas: 1, ObservedGeneration: 1},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ss-0",
			Namespace: "default",
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Name:       "ss",
				UID:        "ss1",
				Controller: ptr.To(true),
			}},
		},
		Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			Conditions: []v1.PodCondition{{
				Type:   v1.PodReady,
				Status: v1.ConditionFalse,
			}},
		},
	}
	client := fake.NewSimpleClientset(ss, pod)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"pods"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.AppsV1().StatefulSets("default").Get(context.Background(), "ss", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.ReadyReplicas != 0 {
		t.Fatalf("readyReplicas=%d", got.Status.ReadyReplicas)
	}
}

func TestSyncCreatesEndpointsForSelectorService(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-sel", Namespace: "default", UID: "s1"},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "none"},
			Ports:    []v1.ServicePort{{Name: "example", Port: 80, TargetPort: intstr.FromInt32(80)}},
		},
	}
	client := fake.NewSimpleClientset(svc)
	assignGeneratedNames(client)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"services"}); err != nil {
		t.Fatal(err)
	}
	if creates(client, "endpoints") == 0 {
		t.Fatal("expected endpoints create")
	}
	if creates(client, "endpointslices") == 0 {
		t.Fatal("expected endpointslice create")
	}
}

func TestSyncCreatesEndpointsForMultiportService(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "default", UID: "m1"},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "none"},
			Ports: []v1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt32(80)},
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(443)},
			},
		},
	}
	client := fake.NewSimpleClientset(svc)
	assignGeneratedNames(client)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"services"}); err != nil {
		t.Fatal(err)
	}
	if creates(client, "endpoints") == 0 {
		t.Fatal("expected endpoints create")
	}
	if creates(client, "endpointslices") == 0 {
		t.Fatal("expected endpointslice create")
	}
}

func TestSyncMirrorsCustomEndpoints(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-ep", Namespace: "default", UID: "s2"},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{Name: "example", Port: 80, Protocol: v1.ProtocolTCP}},
		},
	}
	ep := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-ep", Namespace: "default", UID: "e1"},
		Subsets: []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{IP: "10.1.2.3"}},
			Ports:     []v1.EndpointPort{{Port: 80, Protocol: v1.ProtocolTCP}},
		}},
	}
	client := fake.NewSimpleClientset(svc, ep)
	assignGeneratedNames(client)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"services", "endpoints"}); err != nil {
		t.Fatal(err)
	}
	if creates(client, "endpointslices") == 0 {
		t.Fatal("expected endpointslice create")
	}
}

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

func TestSyncDeletesExpiredBootstrapToken(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-token-prb001", Namespace: metav1.NamespaceSystem, UID: "tok1"},
		Type:       bootstrapapi.SecretTypeBootstrapToken,
		Data: map[string][]byte{
			bootstrapapi.BootstrapTokenIDKey:         []byte("prb001"),
			bootstrapapi.BootstrapTokenSecretKey:     []byte("notasecret0000"),
			bootstrapapi.BootstrapTokenExpirationKey: []byte("2000-01-01T00:00:00Z"),
		},
	}
	client := fake.NewSimpleClientset(secret)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"secrets"}); err != nil {
		t.Fatal(err)
	}
	_, err := client.CoreV1().Secrets(metav1.NamespaceSystem).Get(context.Background(), secret.Name, metav1.GetOptions{})
	if err == nil {
		t.Fatal("expired bootstrap token still present")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatal(err)
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

func TestSyncAssignsNodePodCIDR(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	client := fake.NewSimpleClientset(node)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"nodes"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.PodCIDR == "" || !strings.HasPrefix(got.Spec.PodCIDR, "10.42.") {
		t.Fatalf("podCIDR = %q", got.Spec.PodCIDR)
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

func TestSyncDeletesExpiredIssuedCSR(t *testing.T) {
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "expired-issued", UID: "csr-old", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    []byte("unused"),
			SignerName: certificatesv1.KubeAPIServerClientSignerName,
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{{
				Type:           certificatesv1.CertificateApproved,
				Status:         v1.ConditionTrue,
				LastUpdateTime: metav1.Now(),
			}},
			Certificate: expiredCertPEM(t),
		},
	}
	client := fake.NewSimpleClientset(csr)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	_, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), csr.Name, metav1.GetOptions{})
	if err == nil {
		t.Fatal("expired issued CSR still present")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}

func expiredCertPEM(t *testing.T) []byte {
	t.Helper()
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired"},
		NotBefore:    time.Now().Add(-2 * time.Hour),
		NotAfter:     time.Now().Add(-time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &pk.PublicKey, pk)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestSyncBindsStaticPersistentVolume(t *testing.T) {
	mode := v1.PersistentVolumeFilesystem
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "static", UID: "pv1", ResourceVersion: "1"},
		Spec: v1.PersistentVolumeSpec{
			Capacity:                      v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
			AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			PersistentVolumeSource:        v1.PersistentVolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/tmp/static"}},
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
			VolumeMode:                    &mode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "default", UID: "pvc1", ResourceVersion: "1"},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources:   v1.VolumeResourceRequirements{Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")}},
			VolumeMode:  &mode,
		},
	}
	client := fake.NewSimpleClientset(pv, pvc)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"persistentvolumeclaims"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().PersistentVolumeClaims("default").Get(context.Background(), "claim", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.VolumeName != "static" || got.Status.Phase != v1.ClaimBound {
		t.Fatalf("claim = %s %s", got.Spec.VolumeName, got.Status.Phase)
	}
	bound, err := client.CoreV1().PersistentVolumes().Get(context.Background(), "static", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if bound.Status.Phase != v1.VolumeBound || bound.Spec.ClaimRef == nil || bound.Spec.ClaimRef.Name != "claim" {
		t.Fatalf("volume = %s %+v", bound.Status.Phase, bound.Spec.ClaimRef)
	}
}

func TestSyncApprovesKubeletClientCSR(t *testing.T) {
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "system:node:foo", Organization: []string{"system:nodes"}},
	}, pk)
	if err != nil {
		t.Fatal(err)
	}
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node-foo", UID: "c1", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
			SignerName: certificatesv1.KubeAPIServerClientKubeletSignerName,
			Username:   "system:node:foo",
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
		},
	}
	client := fake.NewSimpleClientset(csr)
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}}, nil
	})
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), "node-foo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	approved := false
	for _, c := range got.Status.Conditions {
		if c.Type == certificatesv1.CertificateApproved && c.Status == v1.ConditionTrue {
			approved = true
		}
	}
	if !approved {
		t.Fatalf("conditions = %+v", got.Status.Conditions)
	}
}

func TestSyncIssuesKubeletClientCSR(t *testing.T) {
	signingCA := testSigningCA(t)
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "system:node:foo", Organization: []string{"system:nodes"}},
	}, pk)
	if err != nil {
		t.Fatal(err)
	}
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node-foo", UID: "c1", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
			SignerName: certificatesv1.KubeAPIServerClientKubeletSignerName,
			Username:   "system:node:foo",
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
		},
	}
	client := fake.NewSimpleClientset(csr)
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}}, nil
	})
	if _, err := Sync(context.Background(), client, []byte("ca"), signingCA, nil, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), "node-foo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Certificate) == 0 {
		t.Fatalf("certificate missing; conditions = %+v", got.Status.Conditions)
	}
}

func TestSyncIssuesKubeletServingCSR(t *testing.T) {
	servingCA := testSigningCA(t)
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:     pkix.Name{CommonName: "system:node:foo", Organization: []string{"system:nodes"}},
		DNSNames:    []string{"foo"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}, pk)
	if err != nil {
		t.Fatal(err)
	}
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node-serve", UID: "c2", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
			SignerName: certificatesv1.KubeletServingSignerName,
			Username:   "system:node:foo",
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageServerAuth},
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{{
				Type:   certificatesv1.CertificateApproved,
				Status: v1.ConditionTrue,
				Reason: "Approved",
			}},
		},
	}
	client := fake.NewSimpleClientset(csr)
	if _, err := Sync(context.Background(), client, []byte("ca"), nil, servingCA, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), "node-serve", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Certificate) == 0 {
		t.Fatalf("certificate missing; conditions = %+v", got.Status.Conditions)
	}
}

func testSigningCA(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return append(out, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})...)
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
