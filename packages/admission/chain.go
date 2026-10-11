package admission

import (
	"context"
	"errors"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type pluginFunc func(*Handler, context.Context, *admit.Request) error

type chainPlugin struct {
	name     string
	admit    pluginFunc
	validate pluginFunc
}

const computeClassPluginName = "ComputeClass"

var chain = []chainPlugin{
	{name: computeClassPluginName, admit: withStore(applyComputeClass)},
	{name: "LimitRanger", admit: withStore(applyLimitRanger), validate: withStore(validateLimitRanger)},
	{name: "ServiceAccount", admit: withStore(applyServiceAccount), validate: withStore(validateServiceAccount)},
	{name: "NodeRestriction", admit: withStore(applyNodeRestriction)},
	{name: "TaintNodesByCondition", admit: withStore(applyTaintNodesByCondition)},
	{name: "PodSecurity", validate: withStore(applyPodSecurity)},
	{name: "Priority", admit: withStore(applyPriority), validate: withStore(validatePriorityClass)},
	{name: "DefaultTolerationSeconds", admit: withStore(applyDefaultTolerationSeconds)},
	{name: "DefaultStorageClass", admit: withStore(applyDefaultStorageClass)},
	{name: "StorageObjectInUseProtection", admit: withStore(applyStorageObjectInUseProtection)},
	{name: "PersistentVolumeClaimResize", validate: withStore(applyPersistentVolumeClaimResize)},
	{name: "RuntimeClass", admit: withStore(applyRuntimeClass), validate: withStore(validateRuntimeClass)},
	{name: "CertificateApproval", validate: withAuthorizer(applyCertificateApproval)},
	{name: "CertificateSigning", validate: withAuthorizer(applyCertificateSigning)},
	{name: "CertificateSubjectRestriction", validate: withStore(applyCertificateSubjectRestriction)},
	{name: "DefaultIngressClass", admit: withStore(applyDefaultIngressClass)},
	{name: "PodTopologyLabels", admit: withStore(applyPodTopologyLabels)},
	{name: "NodeDeclaredFeatureValidator", validate: withStore(validateNodeDeclaredFeatures)},
	{name: "PodResizeValidator", validate: withStore(validatePodResize)},
	{name: "MutatingAdmissionPolicy", admit: (*Handler).admitMAP},
	{name: "MutatingAdmissionWebhook", admit: (*Handler).admitWebhooks},
	{name: "ValidatingAdmissionPolicy", validate: (*Handler).validateVAP},
	{name: "ValidatingAdmissionWebhook", validate: (*Handler).validateWebhooks},
	{name: "ResourceQuota", validate: withStore(applyResourceQuota)},
}

func withStore(f func(context.Context, *store, *admit.Request) error) pluginFunc {
	return func(h *Handler, ctx context.Context, req *admit.Request) error { return f(ctx, h.store, req) }
}

func withAuthorizer(f func(context.Context, authorizer.Authorizer, *admit.Request) error) pluginFunc {
	return func(h *Handler, ctx context.Context, req *admit.Request) error { return f(ctx, h.authz, req) }
}

func (h *Handler) run(ctx context.Context, req admit.Request) (admit.Response, error) {
	ctx, rec := withAdmissionRecord(ctx)
	out := h.runChain(ctx, &req)
	out.Warnings = rec.warnings
	if len(rec.annotations) > 0 {
		out.AuditAnnotations = rec.annotations
	}
	return out, nil
}

func (h *Handler) runChain(ctx context.Context, req *admit.Request) admit.Response {
	mutating := req.Phase == "admit"
	for _, p := range chain {
		f := p.validate
		if mutating {
			f = p.admit
		}
		if f == nil {
			continue
		}
		if err := f(h, ctx, req); err != nil {
			return denyResponse(err)
		}
	}
	if mutating {
		return admit.Response{Allowed: true, Object: req.Object}
	}
	return admit.Response{Allowed: true}
}

func (h *Handler) admitMAP(ctx context.Context, req *admit.Request) error {
	obj, err := h.runMAP(ctx, *req)
	if err != nil {
		return err
	}
	req.Object = obj
	return nil
}

func (h *Handler) admitWebhooks(ctx context.Context, req *admit.Request) error {
	obj, err := h.runMutating(ctx, *req)
	if err != nil {
		return err
	}
	req.Object = obj
	return nil
}

func (h *Handler) validateVAP(ctx context.Context, req *admit.Request) error {
	return h.runVAP(ctx, *req)
}

func (h *Handler) validateWebhooks(ctx context.Context, req *admit.Request) error {
	return h.runValidating(ctx, *req)
}

func (h *Handler) runMutating(ctx context.Context, req admit.Request) (map[string]any, error) {
	if exemptAdmissionConfig(req) {
		return req.Object, nil
	}
	cfgs, err := h.store.mutatingConfigs(ctx)
	if err != nil {
		return nil, err
	}
	obj := req.Object
	for _, cfg := range cfgs {
		for _, hook := range cfg.Webhooks {
			if !matchWebhook(req, hook.Rules, hook.NamespaceSelector, hook.ObjectSelector, h.namespaceLabels(ctx, req.Namespace), hook.MatchPolicy) {
				continue
			}
			ok, err := matchConditions(req, hook.MatchConditions)
			if err != nil {
				if ignoreFailure(hook.FailurePolicy) {
					continue
				}
				return nil, err
			}
			if !ok {
				continue
			}
			next, err := h.callWebhook(ctx, req, cfg.Annotations, hook.Name, hook.ClientConfig, hook.TimeoutSeconds, hook.FailurePolicy, obj)
			if err != nil {
				return nil, err
			}
			obj = next
			req.Object = obj
		}
	}
	return obj, nil
}

func (h *Handler) runValidating(ctx context.Context, req admit.Request) error {
	if exemptAdmissionConfig(req) {
		return nil
	}
	cfgs, err := h.store.validatingConfigs(ctx)
	if err != nil {
		return err
	}
	nsLabels := h.namespaceLabels(ctx, req.Namespace)
	for _, cfg := range cfgs {
		for _, hook := range cfg.Webhooks {
			if !matchWebhook(req, hook.Rules, hook.NamespaceSelector, hook.ObjectSelector, nsLabels, hook.MatchPolicy) {
				continue
			}
			ok, err := matchConditions(req, hook.MatchConditions)
			if err != nil {
				if ignoreFailure(hook.FailurePolicy) {
					continue
				}
				return err
			}
			if !ok {
				continue
			}
			if _, err := h.callWebhook(ctx, req, cfg.Annotations, hook.Name, hook.ClientConfig, hook.TimeoutSeconds, hook.FailurePolicy, req.Object); err != nil {
				return err
			}
		}
	}
	return nil
}

func ignoreFailure(p *admissionregv1.FailurePolicyType) bool {
	return p != nil && *p == admissionregv1.Ignore
}

func failClosed(p *admissionregv1.FailurePolicyType, err error) error {
	if ignoreFailure(p) {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("webhook failed")
	}
	return internalError{err}
}

func denyResponse(err error) admit.Response {
	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		st := apiStatus.Status()
		return admit.Response{
			Allowed: false,
			Message: st.Message,
			Reason:  string(st.Reason),
			Status:  &st,
		}
	}
	out := admit.Response{Allowed: false, Message: err.Error()}
	if _, ok := err.(internalError); ok {
		out.Reason = "InternalError"
	}
	return out
}

type internalError struct{ error }

func (h *Handler) namespaceLabels(ctx context.Context, name string) map[string]string {
	if name == "" {
		return nil
	}
	ns, ok, err := h.store.namespace(ctx, name)
	if err != nil || !ok {
		return nil
	}
	return ns.Labels
}
