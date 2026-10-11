package apiserver

import (
	"bytes"
	"context"
	"io"
	"net/http"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/filters"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	auditlog "k8s.io/apiserver/plugin/pkg/audit/log"
	"k8s.io/client-go/kubernetes/scheme"
)

const DefaultAuditPolicy = `apiVersion: audit.k8s.io/v1
kind: Policy
omitStages:
- RequestReceived
rules:
- level: None
  users: ["system:kube-proxy"]
  verbs: ["watch"]
  resources:
  - group: ""
    resources: ["endpoints", "services", "services/status"]
- level: None
  userGroups: ["system:nodes"]
  verbs: ["get"]
  resources:
  - group: ""
    resources: ["nodes", "nodes/status"]
- level: None
  userGroups: ["system:nodes"]
  verbs: ["get", "update", "patch"]
  resources:
  - group: "coordination.k8s.io"
    resources: ["leases"]
- level: None
  nonResourceURLs:
  - /healthz*
  - /livez*
  - /readyz*
  - /version
  - /swagger*
  - /internal/queue/*
- level: None
  resources:
  - group: ""
    resources: ["events"]
  - group: "events.k8s.io"
    resources: ["events"]
- level: Metadata
  resources:
  - group: ""
    resources: ["secrets", "configmaps", "serviceaccounts/token"]
  - group: "authentication.k8s.io"
    resources: ["tokenreviews"]
- level: Metadata
`

type auditor struct {
	sink   audit.Sink
	policy audit.PolicyRuleEvaluator
}

func newAuditor(policyYAML string, out io.Writer) (*auditor, error) {
	if policyYAML == "" {
		policyYAML = DefaultAuditPolicy
	}
	loaded, err := policy.LoadPolicyFromBytes([]byte(policyYAML))
	if err != nil {
		return nil, err
	}
	return &auditor{
		sink:   auditlog.NewBackend(out, auditlog.FormatJson, auditv1.SchemeGroupVersion),
		policy: policy.NewPolicyRuleEvaluator(loaded),
	}, nil
}

func withFrontFilters(inner http.Handler, authn authenticator.Request, authorization authorizer.Authorizer, a *auditor, nonMutating, mutating int) http.Handler {
	authorized := withInflightLimit(auth.WithAuthorization(inner, authorization), nonMutating, mutating)
	audited := filters.WithAudit(withAuditConfig(auth.WithImpersonation(authorized), a.policy), a.sink, a.policy, longRunningRequest)
	authenticated := auth.WithAuthentication(audited, authn, func(failed http.Handler) http.Handler {
		preserveFailure := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac := audit.AuditContextFrom(r.Context())
			if ac.Enabled() {
				if status := ac.GetEventResponseStatus(); status != nil {
					audit.AddAuditAnnotation(r.Context(), "k8flare.com/authentication-failure", status.Message)
				}
			}
			failed.ServeHTTP(w, r)
		})
		return filters.WithFailedAuthenticationAudit(preserveFailure, a.sink, a.policy)
	})
	return filters.WithAuditInit(auth.WithRequestInfo(authenticated))
}

type auditConfigKey struct{}

func withAuditConfig(next http.Handler, evaluator audit.PolicyRuleEvaluator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attribs, _ := filters.GetAuthorizerAttributes(r.Context())
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), auditConfigKey{}, evaluator.EvaluatePolicyRule(attribs))))
	})
}

func forwardAuditConfig(r *http.Request, h http.Header) {
	rac, _ := r.Context().Value(auditConfigKey{}).(audit.RequestAuditConfig)
	if info, ok := genericapirequest.RequestInfoFrom(r.Context()); ok && longRunningRequest(r, info) {
		rac = audit.RequestAuditConfig{}
	}
	auth.ForwardAuditConfig(h, rac)
}

func logForwardedObjects(r *http.Request, h http.Header) {
	request, response, truncated := auth.TakeForwardedAuditObjects(h)
	ac := audit.AuditContextFrom(r.Context())
	if !ac.Enabled() {
		return
	}
	if truncated {
		audit.AddAuditAnnotation(r.Context(), "audit.k8s.io/truncated", "true")
	}
	if len(request) > 0 {
		ac.LogRequestPatch(request)
	}
	if len(response) > 0 {
		ac.LogResponseObject(nil, &runtime.Unknown{Raw: response, ContentType: runtime.ContentTypeJSON})
	}
}

func withBypassAudit(inner http.Handler, a *auditor) http.Handler {
	return filters.WithAuditInit(auth.WithRequestInfo(filters.WithAudit(inner, a.sink, a.policy, longRunningRequest)))
}

func logForwardedResponseStatus(r *http.Request, resp *http.Response) {
	ac := audit.AuditContextFrom(r.Context())
	if !ac.Enabled() || resp.StatusCode < http.StatusBadRequest {
		return
	}
	const maxStatusBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxStatusBytes+1))
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(body), resp.Body), resp.Body}
	if err != nil || len(body) > maxStatusBytes {
		return
	}
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(body, nil, nil)
	if err != nil {
		return
	}
	if status, ok := obj.(*metav1.Status); ok {
		ac.LogResponseObject(status, nil)
	}
}
