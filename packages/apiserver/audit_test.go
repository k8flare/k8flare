//go:build !js

package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var auditTestUsers = map[string]*user.DefaultInfo{
	"admin-token":    {Name: "admin", Groups: []string{user.SystemPrivilegedGroup, user.AllAuthenticated}},
	"readonly-token": {Name: "readonly", Groups: []string{user.AllAuthenticated}},
	"node-token":     {Name: "system:node:n1", Groups: []string{user.NodesGroup, user.AllAuthenticated}},
}

var auditTestAuthn = authenticator.RequestFunc(func(r *http.Request) (*authenticator.Response, bool, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return nil, false, nil
	}
	if u, ok := auditTestUsers[strings.TrimPrefix(header, "Bearer ")]; ok {
		return &authenticator.Response{User: u}, true, nil
	}
	return nil, false, http.ErrNoCookie
})

var auditTestAuthz = authorizer.AuthorizerFunc(func(_ context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	for _, g := range attrs.GetUser().GetGroups() {
		if g == user.SystemPrivilegedGroup {
			return authorizer.DecisionAllow, "masters", nil
		}
	}
	switch attrs.GetVerb() {
	case "get", "list", "watch":
		return authorizer.DecisionAllow, "readable", nil
	}
	if attrs.GetUser().GetName() == "system:node:n1" {
		return authorizer.DecisionAllow, "node", nil
	}
	return authorizer.DecisionDeny, "not allowed", nil
})

type auditRig struct {
	out     *lockedBuffer
	handler http.Handler
}

func newAuditRig(t *testing.T, policy string, inner http.Handler) *auditRig {
	t.Helper()
	out := &lockedBuffer{}
	a, err := newAuditor(policy, out)
	if err != nil {
		t.Fatal(err)
	}
	return &auditRig{out: out, handler: withFrontFilters(inner, auditTestAuthn, auditTestAuthz, a, 10, 10)}
}

func (r *auditRig) do(method, target, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = "203.0.113.7:4242"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func (r *auditRig) events(t *testing.T) []auditv1.Event {
	t.Helper()
	var events []auditv1.Event
	for _, line := range strings.Split(strings.TrimSpace(r.out.String()), "\n") {
		if line == "" {
			continue
		}
		var ev auditv1.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("audit line is not JSON: %v: %s", err, line)
		}
		events = append(events, ev)
	}
	return events
}

func okInner() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"Status","status":"Success"}`))
	})
}

func TestAuditDefaultPolicyLevels(t *testing.T) {
	cases := []struct {
		name, method, target, token string
		want                        auditv1.Level
	}{
		{"pod read", http.MethodGet, "/api/v1/namespaces/default/pods", "admin-token", auditv1.LevelMetadata},
		{"secret write", http.MethodPost, "/api/v1/namespaces/default/secrets", "admin-token", auditv1.LevelMetadata},
		{"configmap write", http.MethodPut, "/api/v1/namespaces/default/configmaps/c", "admin-token", auditv1.LevelMetadata},
		{"non-resource", http.MethodGet, "/metrics", "admin-token", auditv1.LevelMetadata},
		{"events", http.MethodPost, "/api/v1/namespaces/default/events", "admin-token", auditv1.LevelNone},
		{"events.k8s.io", http.MethodPost, "/apis/events.k8s.io/v1/namespaces/default/events", "admin-token", auditv1.LevelNone},
		{"healthz", http.MethodGet, "/healthz", "admin-token", auditv1.LevelNone},
		{"readyz", http.MethodGet, "/readyz", "admin-token", auditv1.LevelNone},
		{"livez", http.MethodGet, "/livez/ping", "admin-token", auditv1.LevelNone},
		{"version", http.MethodGet, "/version", "admin-token", auditv1.LevelNone},
		{"queue plan", http.MethodPost, "/internal/queue/plan", "admin-token", auditv1.LevelNone},
		{"node lease renewal", http.MethodPut, "/apis/coordination.k8s.io/v1/namespaces/kube-node-lease/leases/n1", "node-token", auditv1.LevelNone},
		{"node get node", http.MethodGet, "/api/v1/nodes/n1", "node-token", auditv1.LevelNone},
		{"node reads secret", http.MethodGet, "/api/v1/namespaces/default/secrets/s", "node-token", auditv1.LevelMetadata},
		{"user writes lease", http.MethodPut, "/apis/coordination.k8s.io/v1/namespaces/default/leases/x", "admin-token", auditv1.LevelMetadata},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newAuditRig(t, "", okInner())
			rig.do(tc.method, tc.target, tc.token, "")
			events := rig.events(t)
			if tc.want == auditv1.LevelNone {
				if len(events) != 0 {
					t.Fatalf("expected no events, got %s", rig.out.String())
				}
				return
			}
			if len(events) == 0 {
				t.Fatalf("expected an event at %s, got none", tc.want)
			}
			for _, ev := range events {
				if ev.Level != tc.want {
					t.Fatalf("level = %s, want %s", ev.Level, tc.want)
				}
			}
		})
	}
}

func TestAuditSecretBodiesAreNotLoggedByDefault(t *testing.T) {
	rig := newAuditRig(t, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"Secret","data":{"password":"aHVudGVyMg=="}}`))
	}))
	rig.do(http.MethodPost, "/api/v1/namespaces/default/secrets", "admin-token", `{"kind":"Secret","stringData":{"password":"hunter2"}}`)
	out := rig.out.String()
	if out == "" {
		t.Fatal("secret write was not audited")
	}
	for _, leaked := range []string{"hunter2", "aHVudGVyMg==", "requestObject", "responseObject"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("audit output contains %q: %s", leaked, out)
		}
	}
}

func TestAuditDefaultPolicyOmitsRequestReceived(t *testing.T) {
	rig := newAuditRig(t, "", okInner())
	rig.do(http.MethodGet, "/api/v1/namespaces/default/pods", "admin-token", "")
	events := rig.events(t)
	if len(events) != 1 || events[0].Stage != auditv1.StageResponseComplete {
		t.Fatalf("want a single ResponseComplete event, got %s", rig.out.String())
	}
}

const allStagesPolicy = `apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: Metadata
`

func TestAuditStages(t *testing.T) {
	t.Run("regular request", func(t *testing.T) {
		rig := newAuditRig(t, allStagesPolicy, okInner())
		rig.do(http.MethodGet, "/api/v1/namespaces/default/pods", "admin-token", "")
		events := rig.events(t)
		if len(events) != 2 || events[0].Stage != auditv1.StageRequestReceived || events[1].Stage != auditv1.StageResponseComplete {
			t.Fatalf("want RequestReceived then ResponseComplete, got %s", rig.out.String())
		}
		if events[0].AuditID != events[1].AuditID || events[0].AuditID == "" {
			t.Fatalf("stages must share an audit ID: %s", rig.out.String())
		}
	})
	t.Run("watch", func(t *testing.T) {
		rig := newAuditRig(t, allStagesPolicy, okInner())
		rig.do(http.MethodGet, "/api/v1/namespaces/default/pods?watch=true", "admin-token", "")
		var stages []auditv1.Stage
		for _, ev := range rig.events(t) {
			stages = append(stages, ev.Stage)
		}
		want := []auditv1.Stage{auditv1.StageRequestReceived, auditv1.StageResponseStarted, auditv1.StageResponseComplete}
		if len(stages) != len(want) {
			t.Fatalf("stages = %v, want %v", stages, want)
		}
		for i := range want {
			if stages[i] != want[i] {
				t.Fatalf("stages = %v, want %v", stages, want)
			}
		}
	})
	t.Run("panic", func(t *testing.T) {
		rig := newAuditRig(t, allStagesPolicy, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
		func() {
			defer func() { _ = recover() }()
			rig.do(http.MethodGet, "/api/v1/namespaces/default/pods", "admin-token", "")
		}()
		events := rig.events(t)
		if len(events) == 0 || events[len(events)-1].Stage != auditv1.StagePanic {
			t.Fatalf("want a Panic stage last, got %s", rig.out.String())
		}
		if events[len(events)-1].ResponseStatus == nil || events[len(events)-1].ResponseStatus.Code != http.StatusInternalServerError {
			t.Fatalf("panic event must carry a 500 status: %s", rig.out.String())
		}
	})
}

func TestAuditAuthenticationFailure(t *testing.T) {
	rig := newAuditRig(t, "", okInner())
	rec := rig.do(http.MethodGet, "/api/v1/namespaces/default/pods", "wrong-token", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	events := rig.events(t)
	if len(events) != 1 {
		t.Fatalf("want one event for the failed request, got %s", rig.out.String())
	}
	ev := events[0]
	if ev.ResponseStatus == nil || ev.ResponseStatus.Code != http.StatusUnauthorized || ev.ResponseStatus.Reason != "Unauthorized" {
		t.Fatalf("event must record the 401: %s", rig.out.String())
	}
	if ev.User.Username != "" || ev.Verb != "list" || ev.RequestURI != "/api/v1/namespaces/default/pods" {
		t.Fatalf("event must identify the request but no user: %s", rig.out.String())
	}
	if id := rec.Header().Get("Audit-ID"); id == "" || id != string(ev.AuditID) {
		t.Fatalf("Audit-ID header %q must match the event %q", id, ev.AuditID)
	}
}

func TestAuditAnonymousRequest(t *testing.T) {
	rig := newAuditRig(t, "", okInner())
	rig.do(http.MethodGet, "/api/v1/namespaces/default/pods", "", "")
	events := rig.events(t)
	if len(events) != 1 || events[0].User.Username != user.Anonymous {
		t.Fatalf("anonymous request must be audited as %s: %s", user.Anonymous, rig.out.String())
	}
}

func TestAuditForbiddenRequest(t *testing.T) {
	rig := newAuditRig(t, "", okInner())
	rec := rig.do(http.MethodDelete, "/api/v1/namespaces/default/pods/p", "readonly-token", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	events := rig.events(t)
	if len(events) != 1 {
		t.Fatalf("want one event, got %s", rig.out.String())
	}
	ev := events[0]
	if ev.User.Username != "readonly" || ev.ResponseStatus.Code != http.StatusForbidden {
		t.Fatalf("event must record the user and the 403: %s", rig.out.String())
	}
	if ev.Annotations["authorization.k8s.io/decision"] != "forbid" || ev.Annotations["authorization.k8s.io/reason"] != "not allowed" {
		t.Fatalf("event must carry the authorization decision: %s", rig.out.String())
	}
}

func TestAuditImpersonation(t *testing.T) {
	rig := newAuditRig(t, "", okInner())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/pods", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Impersonate-User", "bob")
	rig.handler.ServeHTTP(httptest.NewRecorder(), req)
	events := rig.events(t)
	if len(events) != 1 {
		t.Fatalf("want one event, got %s", rig.out.String())
	}
	if events[0].User.Username != "admin" || events[0].ImpersonatedUser == nil || events[0].ImpersonatedUser.Username != "bob" {
		t.Fatalf("event must record admin impersonating bob: %s", rig.out.String())
	}
}

func TestAuditEventFormat(t *testing.T) {
	rig := newAuditRig(t, "", okInner())
	rig.do(http.MethodGet, "/api/v1/namespaces/default/pods/p?x=1", "admin-token", "")
	lines := strings.Split(strings.TrimSpace(rig.out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want one JSON line, got %q", rig.out.String())
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["kind"] != "Event" || raw["apiVersion"] != "audit.k8s.io/v1" {
		t.Fatalf("not an audit.k8s.io/v1 Event: %s", lines[0])
	}
	if !strings.HasPrefix(lines[0], `{"kind":"Event","apiVersion":"audit.k8s.io/v1"`) {
		t.Fatalf("line must start with the Event type meta: %s", lines[0])
	}
	ev := rig.events(t)[0]
	ref := ev.ObjectRef
	if ev.Level != auditv1.LevelMetadata || ev.Verb != "get" || ev.RequestURI != "/api/v1/namespaces/default/pods/p?x=1" || ev.AuditID == "" {
		t.Fatalf("unexpected event: %s", lines[0])
	}
	if ref == nil || ref.Resource != "pods" || ref.Namespace != "default" || ref.Name != "p" || ref.APIVersion != "v1" {
		t.Fatalf("unexpected objectRef: %s", lines[0])
	}
	if ev.User.Username != "admin" || len(ev.SourceIPs) != 1 || ev.SourceIPs[0] != "203.0.113.7" {
		t.Fatalf("unexpected user or source IPs: %s", lines[0])
	}
	if ev.ResponseStatus == nil || ev.ResponseStatus.Code != http.StatusOK || ev.RequestReceivedTimestamp.IsZero() || ev.StageTimestamp.IsZero() {
		t.Fatalf("unexpected status or timestamps: %s", lines[0])
	}
}

func TestAuditCustomPolicy(t *testing.T) {
	const policy = `apiVersion: audit.k8s.io/v1
kind: Policy
omitStages: ["RequestReceived"]
rules:
- level: None
  resources:
  - group: ""
    resources: ["pods"]
- level: Metadata
`
	rig := newAuditRig(t, policy, okInner())
	rig.do(http.MethodGet, "/api/v1/namespaces/default/pods", "admin-token", "")
	rig.do(http.MethodGet, "/api/v1/namespaces/default/services", "admin-token", "")
	events := rig.events(t)
	if len(events) != 1 || events[0].ObjectRef.Resource != "services" {
		t.Fatalf("custom policy must silence pods and keep services: %s", rig.out.String())
	}
}

func TestAuditRejectsInvalidPolicy(t *testing.T) {
	for name, policy := range map[string]string{
		"not yaml":     "{{{",
		"no rules":     "apiVersion: audit.k8s.io/v1\nkind: Policy\nrules: []\n",
		"unknown kind": "apiVersion: v1\nkind: Pod\n",
		"bad level":    "apiVersion: audit.k8s.io/v1\nkind: Policy\nrules:\n- level: Everything\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newAuditor(policy, &lockedBuffer{}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
