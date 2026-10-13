//go:build !js

package apiserver

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	fcboot "k8s.io/apiserver/pkg/apis/flowcontrol/bootstrap"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	utilflowcontrol "k8s.io/apiserver/pkg/util/flowcontrol"
	fcrequest "k8s.io/apiserver/pkg/util/flowcontrol/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func differentialFlowControlObjects() ([]flowcontrolv1.PriorityLevelConfiguration, []flowcontrolv1.FlowSchema) {
	var levels []flowcontrolv1.PriorityLevelConfiguration
	for _, pl := range append(append([]*flowcontrolv1.PriorityLevelConfiguration{}, fcboot.MandatoryPriorityLevelConfigurations...), fcboot.SuggestedPriorityLevelConfigurations...) {
		levels = append(levels, *pl.DeepCopy())
	}
	levels = append(levels, flowcontrolv1.PriorityLevelConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "custom"},
		Spec: flowcontrolv1.PriorityLevelConfigurationSpec{
			Type:    flowcontrolv1.PriorityLevelEnablementLimited,
			Limited: &flowcontrolv1.LimitedPriorityLevelConfiguration{NominalConcurrencyShares: ptr.To[int32](1), LimitResponse: flowcontrolv1.LimitResponse{Type: flowcontrolv1.LimitResponseTypeReject}},
		},
	})
	var schemas []flowcontrolv1.FlowSchema
	for _, fs := range append(append([]*flowcontrolv1.FlowSchema{}, fcboot.MandatoryFlowSchemas...), fcboot.SuggestedFlowSchemas...) {
		schemas = append(schemas, *fs.DeepCopy())
	}
	userRule := func(name string, precedence int32, level string, subject flowcontrolv1.Subject, rule flowcontrolv1.PolicyRulesWithSubjects) flowcontrolv1.FlowSchema {
		rule.Subjects = []flowcontrolv1.Subject{subject}
		return flowcontrolv1.FlowSchema{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: flowcontrolv1.FlowSchemaSpec{
				MatchingPrecedence:         precedence,
				PriorityLevelConfiguration: flowcontrolv1.PriorityLevelConfigurationReference{Name: level},
				DistinguisherMethod:        &flowcontrolv1.FlowDistinguisherMethod{Type: flowcontrolv1.FlowDistinguisherMethodByUserType},
				Rules:                      []flowcontrolv1.PolicyRulesWithSubjects{rule},
			},
		}
	}
	everyURL := flowcontrolv1.PolicyRulesWithSubjects{NonResourceRules: []flowcontrolv1.NonResourcePolicyRule{{Verbs: []string{"*"}, NonResourceURLs: []string{"*"}}}}
	configMaps := flowcontrolv1.PolicyRulesWithSubjects{ResourceRules: []flowcontrolv1.ResourcePolicyRule{{Verbs: []string{"get", "list"}, APIGroups: []string{""}, Resources: []string{"configmaps"}, Namespaces: []string{"team"}}}}
	schemas = append(schemas,
		userRule("custom-user", 1000, "custom", flowcontrolv1.Subject{Kind: flowcontrolv1.SubjectKindUser, User: &flowcontrolv1.UserSubject{Name: "noxu"}}, everyURL),
		userRule("custom-sa", 900, "custom", flowcontrolv1.Subject{Kind: flowcontrolv1.SubjectKindServiceAccount, ServiceAccount: &flowcontrolv1.ServiceAccountSubject{Namespace: "team", Name: "*"}}, configMaps),
		userRule("custom-dangling", 10, "missing", flowcontrolv1.Subject{Kind: flowcontrolv1.SubjectKindGroup, Group: &flowcontrolv1.GroupSubject{Name: "*"}}, everyURL),
	)
	return levels, schemas
}

func TestClassificationMatchesUpstreamAPFController(t *testing.T) {
	levels, schemas := differentialFlowControlObjects()
	var objects []runtime.Object
	for i := range levels {
		objects = append(objects, &levels[i])
	}
	for i := range schemas {
		objects = append(objects, &schemas[i])
	}
	clientset := fake.NewSimpleClientset(objects...)
	factory := informers.NewSharedInformerFactory(clientset, 0)
	upstream := utilflowcontrol.New(factory, clientset.FlowcontrolV1(), 600)
	stop := make(chan struct{})
	defer close(stop)
	factory.Start(stop)
	go func() { _ = upstream.Run(stop) }()

	upstreamClassify := func(ri *genericapirequest.RequestInfo, u user.Info) (string, string) {
		var fsName, plName string
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		upstream.Handle(ctx, utilflowcontrol.RequestDigest{RequestInfo: ri, User: u},
			func(fs *flowcontrolv1.FlowSchema, pl *flowcontrolv1.PriorityLevelConfiguration, _ string) {
				fsName, plName = fs.Name, pl.Name
			},
			func() fcrequest.WorkEstimate { return fcrequest.WorkEstimate{InitialSeats: 1} },
			func(bool) {}, func() {})
		return fsName, plName
	}

	resolver := &genericapirequest.RequestInfoFactory{APIPrefixes: sets.NewString("api", "apis"), GrouplessAPIPrefixes: sets.NewString("api")}
	info := func(method, path string) *genericapirequest.RequestInfo {
		ri, err := resolver.NewRequestInfo(httptest.NewRequest(method, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		return ri
	}
	noxu := &user.DefaultInfo{Name: "noxu", Groups: []string{user.AllAuthenticated}}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if fs, _ := upstreamClassify(info("GET", "/version"), noxu); fs == "custom-user" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upstream controller never digested the configuration")
		}
		time.Sleep(50 * time.Millisecond)
	}

	users := []user.Info{
		apfUsers["masters"], apfUsers["node"], apfUsers["sa"], apfUsers["anonymous"], apfUsers["scheduler"], apfUsers["alice"], noxu,
		&user.DefaultInfo{Name: "system:serviceaccount:team:builder", Groups: []string{"system:serviceaccounts", "system:serviceaccounts:team", user.AllAuthenticated}},
		&user.DefaultInfo{Name: "system:serviceaccount:kube-system:generic-garbage-collector", Groups: []string{"system:serviceaccounts", "system:serviceaccounts:kube-system", user.AllAuthenticated}},
		&user.DefaultInfo{Name: user.KubeControllerManager, Groups: []string{user.AllAuthenticated}},
		&user.DefaultInfo{Name: "system:apiserver", Groups: []string{user.AllAuthenticated}},
		&user.DefaultInfo{Name: "someone", Groups: []string{"system:monitoring", user.AllAuthenticated}},
	}
	requests := [][2]string{
		{"GET", "/api/v1/namespaces/default/pods"},
		{"GET", "/api/v1/namespaces/default/pods?watch=true"},
		{"POST", "/api/v1/namespaces/default/pods"},
		{"GET", "/api/v1/nodes/n1"},
		{"PUT", "/api/v1/nodes/n1/status"},
		{"PUT", "/apis/coordination.k8s.io/v1/namespaces/kube-node-lease/leases/n1"},
		{"PUT", "/apis/coordination.k8s.io/v1/namespaces/kube-system/leases/kube-scheduler"},
		{"GET", "/api/v1/namespaces/kube-system/endpoints/kube-controller-manager"},
		{"GET", "/api/v1/namespaces/team/configmaps"},
		{"GET", "/api/v1/namespaces/team/configmaps/c"},
		{"DELETE", "/api/v1/namespaces/team/configmaps/c"},
		{"GET", "/apis/apps/v1/deployments"},
		{"POST", "/apis/authentication.k8s.io/v1/tokenreviews"},
		{"GET", "/apis/flowcontrol.apiserver.k8s.io/v1/flowschemas"},
		{"GET", "/healthz"},
		{"GET", "/readyz/etcd"},
		{"GET", "/livez"},
		{"GET", "/metrics"},
		{"GET", "/version"},
		{"GET", "/api"},
		{"GET", "/openapi/v2"},
	}
	ours := digestFlowControl(levels, schemas, 600)
	for _, u := range users {
		for _, rq := range requests {
			ri := info(rq[0], rq[1])
			wantFS, wantPL := upstreamClassify(ri, u)
			fs, level := ours.classify(ri, u)
			if fs.Name != wantFS || level.config.Name != wantPL {
				t.Errorf("%s %s by %s: got %s/%s, upstream %s/%s", rq[0], rq[1], u.GetName(), fs.Name, level.config.Name, wantFS, wantPL)
			}
		}
	}
}
