package helm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k8flare/k8flare/packages/addons"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const defaultHookTimeout = 300 * time.Second

var errHookTimeout = errors.New("timed out waiting for the condition")

type waiting struct {
	until time.Time
}

func (w *waiting) Error() string {
	return "waiting for hooks"
}

type hookFailure struct {
	err error
}

func (f *hookFailure) Error() string {
	return f.err.Error()
}

func (f *hookFailure) Unwrap() error {
	return f.err
}

func hookTimeout(chart *helmv1.HelmChart) time.Duration {
	if chart.Spec.Timeout != nil {
		return chart.Spec.Timeout.Duration
	}
	return defaultHookTimeout
}

type hookByWeight []*release.Hook

func (x hookByWeight) Len() int      { return len(x) }
func (x hookByWeight) Swap(i, j int) { x[i], x[j] = x[j], x[i] }
func (x hookByWeight) Less(i, j int) bool {
	if x[i].Weight == x[j].Weight {
		return x[i].Name < x[j].Name
	}
	return x[i].Weight < x[j].Weight
}

type hookRun struct {
	cluster *cluster
	store   store
	rec     *record
	timeout time.Duration
	now     func() time.Time
}

func (c *Controller) hookRun(rec *record, timeout time.Duration) *hookRun {
	return &hookRun{cluster: c.cluster(), store: c.store(), rec: rec, timeout: timeout, now: c.now}
}

func (r *hookRun) hooks(event release.HookEvent) []*release.Hook {
	var hooks []*release.Hook
	for _, h := range r.rec.release.Hooks {
		if slices.Contains(h.Events, event) {
			hooks = append(hooks, h)
		}
	}
	sort.Stable(hookByWeight(hooks))
	return hooks
}

func (r *hookRun) started(event release.HookEvent) bool {
	for _, h := range r.hooks(event) {
		if !h.LastRun.StartedAt.IsZero() {
			return true
		}
	}
	return false
}

func (r *hookRun) event(ctx context.Context, event release.HookEvent) error {
	hooks := r.hooks(event)
	progressed := false
	for i, h := range hooks {
		if h.LastRun.Phase == release.HookPhaseSucceeded {
			continue
		}
		if h.LastRun.Phase != release.HookPhaseRunning {
			if len(h.DeletePolicies) == 0 {
				h.DeletePolicies = []release.HookDeletePolicy{release.HookBeforeHookCreation}
			}
			created, createErr, err := r.start(ctx, h)
			if err != nil {
				return r.keep(ctx, progressed, err)
			}
			if createErr != nil {
				return r.fail(ctx, hooks, i, event, createErr)
			}
			if !created {
				return r.keep(ctx, progressed, &waiting{})
			}
			progressed = true
		}
		done, failed, err := r.done(ctx, h)
		if err != nil {
			return r.keep(ctx, progressed, err)
		}
		deadline := h.LastRun.StartedAt.Time.Add(r.timeout)
		if failed == nil && !done && !r.now().Before(deadline) {
			failed = errHookTimeout
		}
		if failed != nil {
			return r.fail(ctx, hooks, i, event, failed)
		}
		if !done {
			return r.keep(ctx, progressed, &waiting{until: deadline})
		}
		h.LastRun.CompletedAt = helmtime.Time{Time: r.now()}
		h.LastRun.Phase = release.HookPhaseSucceeded
		progressed = true
	}
	if !progressed {
		return nil
	}
	if err := r.store.update(ctx, r.rec); err != nil {
		return err
	}
	for i := len(hooks) - 1; i >= 0; i-- {
		if err := r.deleteByPolicy(ctx, hooks[i], release.HookSucceeded); err != nil {
			return err
		}
	}
	return nil
}

func (r *hookRun) keep(ctx context.Context, progressed bool, result error) error {
	if progressed {
		if err := r.store.update(ctx, r.rec); err != nil {
			return errors.Join(result, err)
		}
	}
	return result
}

func (r *hookRun) fail(ctx context.Context, hooks []*release.Hook, i int, event release.HookEvent, cause error) error {
	h := hooks[i]
	h.LastRun.CompletedAt = helmtime.Time{Time: r.now()}
	h.LastRun.Phase = release.HookPhaseFailed
	failure := &hookFailure{err: fmt.Errorf("%s hook %s: %w", event, h.Path, cause)}
	errs := []error{failure}
	if err := r.deleteByPolicy(ctx, h, release.HookFailed); err != nil {
		errs = append(errs, err)
	}
	for _, succeeded := range hooks[:i] {
		if err := r.deleteByPolicy(ctx, succeeded, release.HookSucceeded); err != nil {
			errs = append(errs, err)
		}
	}
	if err := r.store.update(ctx, r.rec); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (r *hookRun) object(ctx context.Context, h *release.Hook) (*unstructured.Unstructured, *meta.RESTMapping, error) {
	objs, err := addons.Decode([]byte(h.Manifest))
	if err != nil {
		return nil, nil, fmt.Errorf("decode hook %s: %w", h.Path, err)
	}
	if len(objs) != 1 {
		return nil, nil, fmt.Errorf("hook %s holds %d objects, want 1", h.Path, len(objs))
	}
	obj := objs[0]
	mapping, err := r.cluster.mapping(ctx, obj.GroupVersionKind())
	if err != nil {
		return nil, nil, err
	}
	if obj.GetNamespace() == "" && mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		obj.SetNamespace(r.rec.release.Namespace)
	}
	return obj, mapping, nil
}

func (r *hookRun) start(ctx context.Context, h *release.Hook) (bool, error, error) {
	if err := r.deleteByPolicy(ctx, h, release.HookBeforeHookCreation); err != nil {
		return false, nil, err
	}
	obj, mapping, err := r.object(ctx, h)
	if err != nil {
		return false, nil, err
	}
	h.LastRun = release.HookExecution{StartedAt: helmtime.Time{Time: r.now()}, Phase: release.HookPhaseRunning}
	_, err = r.cluster.resource(mapping, obj.GetNamespace()).Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) && slices.Contains(h.DeletePolicies, release.HookBeforeHookCreation) {
		h.LastRun = release.HookExecution{}
		return false, nil, nil
	}
	return err == nil, err, nil
}

func (r *hookRun) done(ctx context.Context, h *release.Hook) (bool, error, error) {
	if h.Kind != "Job" && h.Kind != "Pod" {
		return true, nil, nil
	}
	obj, mapping, err := r.object(ctx, h)
	if err != nil {
		return false, nil, err
	}
	current, err := r.cluster.resource(mapping, obj.GetNamespace()).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if h.Kind == "Pod" {
		switch phase, _, _ := unstructured.NestedString(current.Object, "status", "phase"); phase {
		case "Succeeded":
			return true, nil, nil
		case "Failed":
			return false, fmt.Errorf("pod %s failed", h.Name), nil
		}
		return false, nil, nil
	}
	conditions, _, _ := unstructured.NestedSlice(current.Object, "status", "conditions")
	for _, c := range conditions {
		condition, _ := c.(map[string]any)
		if condition["status"] != "True" {
			continue
		}
		switch condition["type"] {
		case "Complete":
			return true, nil, nil
		case "Failed":
			return false, fmt.Errorf("job %s failed: %v", h.Name, condition["reason"]), nil
		}
	}
	return false, nil, nil
}

func (r *hookRun) deleteByPolicy(ctx context.Context, h *release.Hook, policy release.HookDeletePolicy) error {
	if h.Kind == "CustomResourceDefinition" || !slices.Contains(h.DeletePolicies, policy) {
		return nil
	}
	obj, mapping, err := r.object(ctx, h)
	if err != nil {
		return err
	}
	background := metav1.DeletePropagationBackground
	err = r.cluster.resource(mapping, obj.GetNamespace()).Delete(ctx, obj.GetName(), metav1.DeleteOptions{PropagationPolicy: &background})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete %s hook %s: %w", policy, h.Path, err)
	}
	return nil
}
