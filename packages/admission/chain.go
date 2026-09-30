package admission

import (
	"context"
	"fmt"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
)

func (h *Handler) run(ctx context.Context, req admit.Request) (admit.Response, error) {
	if req.Phase == "admit" {
		if err := applyComputeClass(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyServiceAccount(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyDefaultTolerationSeconds(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyLimitRanger(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyDefaultStorageClass(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyStorageObjectInUseProtection(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyDefaultIngressClass(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyRuntimeClass(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyPriority(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyTaintNodesByCondition(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		if err := applyPodTopologyLabels(ctx, h.store, &req); err != nil {
			return denyResponse(err), nil
		}
		mutated, err := h.runMutating(ctx, req)
		if err != nil {
			return denyResponse(err), nil
		}
		return admit.Response{Allowed: true, Object: mutated}, nil
	}
	if err := h.runValidating(ctx, req); err != nil {
		return denyResponse(err), nil
	}
	if err := h.runVAP(ctx, req); err != nil {
		return denyResponse(err), nil
	}
	if err := applyCertificateSubjectRestriction(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := applyCertificateApproval(ctx, h.authz, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := applyCertificateSigning(ctx, h.authz, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := applyPersistentVolumeClaimResize(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := applyResourceQuota(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := validateRuntimeClass(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := applyPodSecurity(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := applyNodeRestriction(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := validatePodResize(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	if err := validateNodeDeclaredFeatures(ctx, h.store, &req); err != nil {
		return denyResponse(err), nil
	}
	return admit.Response{Allowed: true}, nil
}

func (h *Handler) runMutating(ctx context.Context, req admit.Request) (map[string]any, error) {
	if exemptAdmissionConfig(req) {
		return req.Object, nil
	}
	cfgs, err := h.store.mutatingConfigs(ctx)
	if err != nil {
		return nil, err
	}
	obj, err := h.runMAP(ctx, req)
	if err != nil {
		return nil, err
	}
	req.Object = obj
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
	out := admit.Response{Allowed: false, Message: err.Error()}
	switch err.(type) {
	case invalidError:
		out.Reason = "Invalid"
	case internalError:
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
