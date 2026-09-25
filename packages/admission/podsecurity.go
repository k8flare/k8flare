package admission

import (
	"context"
	"fmt"
	"sync"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	psapi "k8s.io/pod-security-admission/api"
	"k8s.io/pod-security-admission/policy"
)

var (
	psaOnce sync.Once
	psaEval policy.Evaluator
	psaErr  error
)

func podSecurityEvaluator() (policy.Evaluator, error) {
	psaOnce.Do(func() {
		psaEval, psaErr = policy.NewEvaluator(policy.DefaultChecks(), nil)
	})
	return psaEval, psaErr
}

func privilegedPodSecurity() psapi.Policy {
	latest := psapi.LatestVersion()
	lv := psapi.LevelVersion{Level: psapi.LevelPrivileged, Version: latest}
	return psapi.Policy{Enforce: lv, Audit: lv, Warn: lv}
}

func applyPodSecurity(ctx context.Context, s *store, req *admit.Request) error {
	if req.Resource.Resource != "pods" || req.Object == nil {
		return nil
	}
	if req.Subresource != "" && req.Subresource != "ephemeralcontainers" {
		return nil
	}
	if req.Operation == "DELETE" {
		return nil
	}
	eval, err := podSecurityEvaluator()
	if err != nil {
		return err
	}
	defaults := privilegedPodSecurity()
	labels := map[string]string{}
	if s != nil && req.Namespace != "" {
		ns, ok, err := s.namespace(ctx, req.Namespace)
		if err != nil {
			return err
		}
		if ok {
			labels = ns.Labels
		}
	}
	nsPolicy, _ := psapi.PolicyToEvaluate(labels, defaults)
	if nsPolicy.Enforce.Level == psapi.LevelPrivileged {
		return nil
	}
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(req.Object, &pod); err != nil {
		return err
	}
	result := policy.AggregateCheckResults(eval.EvaluatePod(nsPolicy.Enforce, &pod.ObjectMeta, &pod.Spec))
	if result.Allowed {
		return nil
	}
	return fmt.Errorf("violates PodSecurity %q: %s", nsPolicy.Enforce.String(), result.ForbiddenDetail())
}
