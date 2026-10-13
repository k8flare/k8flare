//go:build !js

package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fcboot "k8s.io/apiserver/pkg/apis/flowcontrol/bootstrap"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/utils/ptr"
)

var apfUsers = map[string]user.Info{
	"masters":   &user.DefaultInfo{Name: "admin", Groups: []string{user.SystemPrivilegedGroup, user.AllAuthenticated}},
	"node":      &user.DefaultInfo{Name: "system:node:n1", Groups: []string{user.NodesGroup, user.AllAuthenticated}},
	"sa":        &user.DefaultInfo{Name: "system:serviceaccount:default:app", Groups: []string{"system:serviceaccounts", "system:serviceaccounts:default", user.AllAuthenticated}},
	"anonymous": &user.DefaultInfo{Name: user.Anonymous, Groups: []string{user.AllUnauthenticated}},
	"scheduler": &user.DefaultInfo{Name: user.KubeScheduler, Groups: []string{user.AllAuthenticated}},
	"alice":     &user.DefaultInfo{Name: "alice", Groups: []string{user.AllAuthenticated}},
}

type seatServer struct {
	mu       sync.Mutex
	requests []kine.SeatRequest
	held     int
	reject   int
}

func (s *seatServer) last() kine.SeatRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[len(s.requests)-1]
}

func (s *seatServer) count() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests), s.held
}

func (s *seatServer) wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/apf/release" {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.held--
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil)), Header: http.Header{}, Request: r}, nil
		}
		if r.URL.Path != "/apf/seat" {
			return next.RoundTrip(r)
		}
		var req kine.SeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests = append(s.requests, req)
		if s.reject > 0 {
			data, _ := json.Marshal(map[string]int{"retryAfter": s.reject})
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}, Request: r}, nil
		}
		s.held++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"seat":1}`)), Header: http.Header{}, Request: r}, nil
	})
}

type apfRig struct {
	store   *kine.Client
	seats   *seatServer
	fc      *flowControl
	now     time.Time
	handler http.Handler
	served  chan *seatServer
}

func putFlowControlObject(t *testing.T, store *kine.Client, key string, obj runtime.Object) {
	t.Helper()
	data, err := runtime.Encode(flowControlCodec, obj)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "/registry"+key, data, 0); err != nil {
		t.Fatal(err)
	}
}

func seedBootstrapFlowControl(t *testing.T, store *kine.Client) {
	t.Helper()
	for _, pl := range append(append([]*flowcontrolv1.PriorityLevelConfiguration{}, fcboot.MandatoryPriorityLevelConfigurations...), fcboot.SuggestedPriorityLevelConfigurations...) {
		obj := pl.DeepCopy()
		obj.UID = types.UID("pl-" + obj.Name)
		putFlowControlObject(t, store, "/prioritylevelconfigurations/"+obj.Name, obj)
	}
	for _, fs := range append(append([]*flowcontrolv1.FlowSchema{}, fcboot.MandatoryFlowSchemas...), fcboot.SuggestedFlowSchemas...) {
		obj := fs.DeepCopy()
		obj.UID = types.UID("fs-" + obj.Name)
		putFlowControlObject(t, store, "/flowschemas/"+obj.Name, obj)
	}
}

func newAPFRig(t *testing.T) *apfRig {
	t.Helper()
	store := fakeVaultStore()
	seedBootstrapFlowControl(t, store)
	rig := &apfRig{store: store, seats: &seatServer{}, now: time.Unix(1000, 0), served: make(chan *seatServer, 1)}
	store.HTTP.Transport = rig.seats.wrap(store.HTTP.Transport)
	rig.fc = newFlowControl(store, 600)
	rig.fc.now = func() time.Time { return rig.now }
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") == "true" {
			w.WriteHeader(http.StatusOK)
			rig.served <- rig.seats
			return
		}
		_, held := rig.seats.count()
		w.Header().Set("X-Test-Held", string(rune('0'+held)))
	})
	limited := auth.WithRequestInfo(withPriorityAndFairness(inner, rig.fc))
	rig.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limited.ServeHTTP(w, r.WithContext(genericapirequest.WithUser(r.Context(), apfUsers[r.Header.Get("X-Test-User")])))
	})
	return rig
}

func (r *apfRig) do(who, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Test-User", who)
	req.Header.Set("CF-Connecting-IP", "203.0.113.9")
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func TestRequestsAreClassifiedByTheBootstrapFlowSchemas(t *testing.T) {
	rig := newAPFRig(t)
	for _, c := range []struct {
		who, path, schema, level string
	}{
		{"masters", "/api/v1/namespaces/default/pods", "exempt", "exempt"},
		{"node", "/api/v1/nodes/n1", "system-node-high", "node-high"},
		{"node", "/api/v1/namespaces/default/pods", "system-nodes", "system"},
		{"sa", "/api/v1/namespaces/default/configmaps", "service-accounts", "workload-low"},
		{"anonymous", "/healthz", "probes", "exempt"},
		{"anonymous", "/api", "global-default", "global-default"},
		{"scheduler", "/api/v1/pods", "kube-scheduler", "workload-high"},
	} {
		rec := rig.do(c.who, http.MethodGet, c.path)
		if got := rec.Header().Get(flowcontrolv1.ResponseHeaderMatchedFlowSchemaUID); got != "fs-"+c.schema {
			t.Errorf("%s %s: flow schema %q, want fs-%s", c.who, c.path, got, c.schema)
		}
		if got := rec.Header().Get(flowcontrolv1.ResponseHeaderMatchedPriorityLevelConfigurationUID); got != "pl-"+c.level {
			t.Errorf("%s %s: priority level %q, want pl-%s", c.who, c.path, got, c.level)
		}
	}
}

func TestExemptRequestsNeverTakeASeat(t *testing.T) {
	rig := newAPFRig(t)
	rig.seats.reject = 5
	for _, c := range []struct{ who, path string }{
		{"masters", "/api/v1/namespaces/default/pods"},
		{"anonymous", "/livez"},
	} {
		if rec := rig.do(c.who, http.MethodGet, c.path); rec.Code != http.StatusOK {
			t.Errorf("%s %s: %d", c.who, c.path, rec.Code)
		}
	}
	if n, _ := rig.seats.count(); n != 0 {
		t.Fatalf("exempt requests asked for %d seats", n)
	}
}

func TestALevelAtItsLimitAnswers429WithRetryAfter(t *testing.T) {
	rig := newAPFRig(t)
	rig.seats.reject = 3
	rec := rig.do("alice", http.MethodGet, "/api/v1/namespaces/default/pods")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "3" {
		t.Fatalf("got %d retry-after=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec.Header().Get(flowcontrolv1.ResponseHeaderMatchedPriorityLevelConfigurationUID) != "pl-global-default" {
		t.Errorf("rejection lacks the priority level header: %v", rec.Header())
	}
}

func TestASeatIsHeldWhileTheRequestRunsAndReleasedAfter(t *testing.T) {
	rig := newAPFRig(t)
	rec := rig.do("sa", http.MethodGet, "/api/v1/namespaces/default/configmaps")
	if rec.Header().Get("X-Test-Held") != "1" {
		t.Fatalf("seats held while serving: %q", rec.Header().Get("X-Test-Held"))
	}
	if n, held := rig.seats.count(); n != 1 || held != 0 {
		t.Fatalf("seats asked=%d still held=%d", n, held)
	}
	seat := rig.seats.last()
	want := fcboot.SuggestedPriorityLevelConfigurationWorkloadLow.Spec.Limited.LimitResponse.Queuing
	if seat.Level != "workload-low" || seat.Queues != int(want.Queues) || seat.QueueLengthLimit != int(want.QueueLengthLimit) || len(seat.Hand) != int(want.HandSize) || seat.WaitMs != 15000 {
		t.Fatalf("seat request %+v", seat)
	}
}

func TestAWatchReleasesItsSeatOnceTheResponseStarts(t *testing.T) {
	rig := newAPFRig(t)
	rig.do("alice", http.MethodGet, "/api/v1/namespaces/default/pods?watch=true")
	if n, held := (<-rig.served).count(); n != 1 || held != 0 {
		t.Fatalf("watch: seats asked=%d held after the response started=%d", n, held)
	}
}

func TestLongRunningRequestsOtherThanWatchesSkipFlowControl(t *testing.T) {
	rig := newAPFRig(t)
	rig.seats.reject = 1
	for _, path := range []string{
		"/api/v1/namespaces/default/pods/p/exec",
		"/api/v1/namespaces/default/pods/p/log",
		"/api/v1/namespaces/default/pods/p/portforward",
		"/api/v1/namespaces/default/services/s/proxy/x",
	} {
		if rec := rig.do("alice", http.MethodGet, path); rec.Code != http.StatusOK {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	if n, _ := rig.seats.count(); n != 0 {
		t.Fatalf("long-running requests asked for %d seats", n)
	}
}

func sharesOfBootstrap() float64 {
	var sum float64
	for _, pl := range append(append([]*flowcontrolv1.PriorityLevelConfiguration{}, fcboot.MandatoryPriorityLevelConfigurations...), fcboot.SuggestedPriorityLevelConfigurations...) {
		sum += float64(nominalConcurrencyShares(pl))
	}
	return sum
}

func TestAnEditedPriorityLevelChangesTheLimitWithoutARestart(t *testing.T) {
	rig := newAPFRig(t)
	sum := sharesOfBootstrap()
	rig.do("sa", http.MethodGet, "/api/v1/namespaces/default/configmaps")
	if got, want := rig.seats.last().Limit, int(math.Ceil(600*100/sum)); got != want {
		t.Fatalf("workload-low limit %d, want %d", got, want)
	}
	edited := fcboot.SuggestedPriorityLevelConfigurationWorkloadLow.DeepCopy()
	edited.UID = "pl-workload-low"
	edited.Spec.Limited.NominalConcurrencyShares = ptr.To[int32](200)
	putFlowControlObject(t, rig.store, "/prioritylevelconfigurations/workload-low", edited)
	rig.do("sa", http.MethodGet, "/api/v1/namespaces/default/configmaps")
	if got, want := rig.seats.last().Limit, int(math.Ceil(600*100/sum)); got != want {
		t.Fatalf("limit changed inside the cache window: %d, want %d", got, want)
	}
	rig.now = rig.now.Add(flowControlConfigTTL)
	rig.do("sa", http.MethodGet, "/api/v1/namespaces/default/configmaps")
	if got, want := rig.seats.last().Limit, int(math.Ceil(600*200/(sum+100))); got != want {
		t.Fatalf("edited workload-low limit %d, want %d", got, want)
	}
}

func TestANewFlowSchemaTakesEffectWithoutARestart(t *testing.T) {
	rig := newAPFRig(t)
	putFlowControlObject(t, rig.store, "/prioritylevelconfigurations/e2e", &flowcontrolv1.PriorityLevelConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e", UID: "pl-e2e"},
		Spec: flowcontrolv1.PriorityLevelConfigurationSpec{
			Type:    flowcontrolv1.PriorityLevelEnablementLimited,
			Limited: &flowcontrolv1.LimitedPriorityLevelConfiguration{NominalConcurrencyShares: ptr.To[int32](1), LimitResponse: flowcontrolv1.LimitResponse{Type: flowcontrolv1.LimitResponseTypeReject}},
		},
	})
	putFlowControlObject(t, rig.store, "/flowschemas/e2e", &flowcontrolv1.FlowSchema{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e", UID: "fs-e2e"},
		Spec: flowcontrolv1.FlowSchemaSpec{
			MatchingPrecedence:         1000,
			PriorityLevelConfiguration: flowcontrolv1.PriorityLevelConfigurationReference{Name: "e2e"},
			DistinguisherMethod:        &flowcontrolv1.FlowDistinguisherMethod{Type: flowcontrolv1.FlowDistinguisherMethodByUserType},
			Rules: []flowcontrolv1.PolicyRulesWithSubjects{{
				Subjects:         []flowcontrolv1.Subject{{Kind: flowcontrolv1.SubjectKindUser, User: &flowcontrolv1.UserSubject{Name: "alice"}}},
				NonResourceRules: []flowcontrolv1.NonResourcePolicyRule{{Verbs: []string{"*"}, NonResourceURLs: []string{"*"}}},
			}},
		},
	})
	rig.now = rig.now.Add(flowControlConfigTTL)
	rec := rig.do("alice", http.MethodGet, "/version")
	if rec.Header().Get(flowcontrolv1.ResponseHeaderMatchedFlowSchemaUID) != "fs-e2e" || rec.Header().Get(flowcontrolv1.ResponseHeaderMatchedPriorityLevelConfigurationUID) != "pl-e2e" {
		t.Fatalf("headers %v", rec.Header())
	}
	if seat := rig.seats.last(); seat.Level != "e2e" || seat.Queues != 0 || len(seat.Hand) != 0 {
		t.Fatalf("seat request %+v", seat)
	}
}

func TestTheRateLimitKeyIsTheUserOrTheClientAddressAndComponentsHaveNone(t *testing.T) {
	rig := newAPFRig(t)
	for _, c := range []struct{ who, path, key string }{
		{"alice", "/api/v1/namespaces/default/pods", "user:alice"},
		{"sa", "/api/v1/namespaces/default/configmaps", "user:system:serviceaccount:default:app"},
		{"anonymous", "/api", "addr:203.0.113.9"},
		{"scheduler", "/api/v1/pods", ""},
	} {
		rig.do(c.who, http.MethodGet, c.path)
		if got := rig.seats.last().User; got != c.key {
			t.Errorf("%s: rate limit key %q, want %q", c.who, got, c.key)
		}
	}
}

func TestAnUnreachableSeatEndpointServesTheRequest(t *testing.T) {
	store := fakeVaultStore()
	seedBootstrapFlowControl(t, store)
	served := false
	limited := auth.WithRequestInfo(withPriorityAndFairness(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true }), newFlowControl(store, 600)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/pods", nil)
	rec := httptest.NewRecorder()
	limited.ServeHTTP(rec, req.WithContext(genericapirequest.WithUser(req.Context(), apfUsers["alice"])))
	if !served || rec.Code != http.StatusOK {
		t.Fatalf("served=%v code=%d", served, rec.Code)
	}
}
