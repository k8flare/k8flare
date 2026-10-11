package helm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"time"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k8flare/k8flare/packages/addons"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const (
	KeyConfigHash       = "helmcharts.helm.cattle.io/configHash"
	Finalizer           = "wrangler.cattle.io/on-helm-chart-remove"
	ManagedBy           = "helm-controller"
	AnnotationManagedBy = "helmcharts.cattle.io/managed-by"
	AnnotationUnmanaged = "helmcharts.helm.cattle.io/unmanaged"
)

var (
	helmChartsGVR  = schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}
	helmConfigsGVR = schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmchartconfigs"}
)

type Controller struct {
	Client       dynamic.Interface
	Mapper       func() (meta.RESTMapper, error)
	HTTP         *http.Client
	Capabilities func() (*chartutil.Capabilities, error)
	Lookup       *rest.Config
	Wait         time.Duration
	Now          func() time.Time
}

func (c *Controller) Reconcile(ctx context.Context) (time.Time, error) {
	charts, err := c.Client.Resource(helmChartsGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	configs := map[string]*helmv1.HelmChartConfig{}
	list, err := c.Client.Resource(helmConfigsGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return time.Time{}, err
	}
	if list != nil {
		for i := range list.Items {
			var config helmv1.HelmChartConfig
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &config); err != nil {
				return time.Time{}, err
			}
			configs[config.Namespace+"/"+config.Name] = &config
		}
	}
	var errs []error
	var wake time.Time
	for i := range charts.Items {
		u := &charts.Items[i]
		until, err := c.reconcileChart(ctx, u, configs[u.GetNamespace()+"/"+u.GetName()])
		if err != nil {
			errs = append(errs, fmt.Errorf("HelmChart %s/%s: %w", u.GetNamespace(), u.GetName(), err))
		}
		if !until.IsZero() && (wake.IsZero() || until.Before(wake)) {
			wake = until
		}
	}
	return wake, errors.Join(errs...)
}

func (c *Controller) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Controller) store() store {
	return store{client: c.Client, now: c.now}
}

func (c *Controller) cluster() *cluster {
	return &cluster{client: c.Client, mapper: c.Mapper, wait: c.Wait}
}

func (c *Controller) secrets(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	secret, err := c.Client.Resource(secretsGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data := map[string][]byte{}
	encoded, _, _ := unstructured.NestedStringMap(secret.Object, "data")
	for k, v := range encoded {
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, err
		}
		data[k] = raw
	}
	return data, nil
}

func manages(chart *helmv1.HelmChart) bool {
	if chart.Spec.Chart == "" && chart.Spec.ChartContent == "" {
		return false
	}
	if _, unmanaged := chart.Annotations[AnnotationUnmanaged]; unmanaged {
		return false
	}
	owner, claimed := chart.Annotations[AnnotationManagedBy]
	return !claimed || owner == ManagedBy
}

func targetNamespace(chart *helmv1.HelmChart) string {
	if chart.Spec.TargetNamespace != "" {
		return chart.Spec.TargetNamespace
	}
	return chart.Namespace
}

type held struct {
	message string
}

func (h *held) Error() string {
	return h.message
}

func waitingUntil(err error) (time.Time, bool) {
	var w *waiting
	if errors.As(err, &w) {
		return w.until, true
	}
	return time.Time{}, false
}

func (c *Controller) reconcileChart(ctx context.Context, u *unstructured.Unstructured, config *helmv1.HelmChartConfig) (time.Time, error) {
	var chart helmv1.HelmChart
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &chart); err != nil {
		return time.Time{}, err
	}
	if !manages(&chart) {
		return time.Time{}, nil
	}
	if chart.DeletionTimestamp != nil {
		err := c.uninstall(ctx, u, &chart)
		if until, ok := waitingUntil(err); ok {
			return until, nil
		}
		return time.Time{}, err
	}
	u, err := c.claim(ctx, u)
	if err != nil {
		return time.Time{}, err
	}
	switch chart.Spec.HelmVersion {
	case "", "v3":
	default:
		return time.Time{}, c.setStatus(ctx, u, "Unsupported version", "Only Helm v3 charts are supported")
	}
	err = c.install(ctx, &chart, config)
	if until, ok := waitingUntil(err); ok {
		return until, c.setStatus(ctx, u, "", "")
	}
	var hold *held
	if errors.As(err, &hold) {
		return time.Time{}, c.setStatus(ctx, u, "Install failed", hold.message)
	}
	if err != nil {
		if statusErr := c.setStatus(ctx, u, "Install failed", err.Error()); statusErr != nil {
			return time.Time{}, errors.Join(err, statusErr)
		}
		return time.Time{}, err
	}
	return time.Time{}, c.setStatus(ctx, u, "", "")
}

func (c *Controller) claim(ctx context.Context, u *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	annotations := u.GetAnnotations()
	if annotations[AnnotationManagedBy] == ManagedBy && slices.Contains(u.GetFinalizers(), Finalizer) {
		return u, nil
	}
	claimed := u.DeepCopy()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationManagedBy] = ManagedBy
	claimed.SetAnnotations(annotations)
	if !slices.Contains(claimed.GetFinalizers(), Finalizer) {
		claimed.SetFinalizers(append(claimed.GetFinalizers(), Finalizer))
	}
	return c.Client.Resource(helmChartsGVR).Namespace(u.GetNamespace()).Update(ctx, claimed, metav1.UpdateOptions{})
}

func (c *Controller) setStatus(ctx context.Context, u *unstructured.Unstructured, reason, message string) error {
	failed := corev1.ConditionFalse
	if reason != "" {
		failed = corev1.ConditionTrue
	}
	condition := map[string]any{"type": string(helmv1.HelmChartFailed), "status": string(failed)}
	if reason != "" {
		condition["reason"] = reason
		condition["message"] = message
	}
	want := []any{condition}
	have, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	if reflect.DeepEqual(have, want) {
		return nil
	}
	updated := u.DeepCopy()
	if err := unstructured.SetNestedSlice(updated.Object, want, "status", "conditions"); err != nil {
		return err
	}
	_, err := c.Client.Resource(helmChartsGVR).Namespace(u.GetNamespace()).UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	return err
}

func (c *Controller) trackedSecrets(ctx context.Context, chart *helmv1.HelmChart, config *helmv1.HelmChartConfig) ([]map[string][]byte, error) {
	refs := chart.Spec.ValuesSecrets
	if config != nil {
		refs = append(refs[:len(refs):len(refs)], config.Spec.ValuesSecrets...)
	}
	var tracked []map[string][]byte
	for _, ref := range refs {
		if ref.IgnoreUpdates || ref.Name == "chart-values-"+chart.Name {
			continue
		}
		data, err := c.secrets(ctx, chart.Namespace, ref.Name)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("valuesSecrets %s: %w", ref.Name, err)
		}
		tracked = append(tracked, data)
	}
	return tracked, nil
}

func configHash(chart *helmv1.HelmChart, config *helmv1.HelmChartConfig, secrets []map[string][]byte) (string, error) {
	input := struct {
		Chart   helmv1.HelmChartSpec        `json:"chart"`
		Config  *helmv1.HelmChartConfigSpec `json:"config,omitempty"`
		Secrets []map[string][]byte         `json:"secrets,omitempty"`
	}{Chart: chart.Spec, Secrets: secrets}
	if config != nil {
		input.Config = &config.Spec
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:40], nil
}

func latestDeployed(history []*record) *record {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].release.Info.Status == release.StatusDeployed {
			return history[i]
		}
	}
	return nil
}

func failurePolicy(chart *helmv1.HelmChart, config *helmv1.HelmChartConfig) helmv1.FailurePolicy {
	policy := helmv1.FailurePolicyReinstall
	if chart.Spec.FailurePolicy != "" {
		policy = chart.Spec.FailurePolicy
	}
	if config != nil && config.Spec.FailurePolicy != "" {
		policy = config.Spec.FailurePolicy
	}
	return policy
}

func (c *Controller) install(ctx context.Context, chart *helmv1.HelmChart, config *helmv1.HelmChartConfig) error {
	name, namespace := chart.Name, targetNamespace(chart)
	tracked, err := c.trackedSecrets(ctx, chart, config)
	if err != nil {
		return err
	}
	hash, err := configHash(chart, config, tracked)
	if err != nil {
		return err
	}
	releases := c.store()
	history, err := releases.history(ctx, namespace, name)
	if err != nil {
		return err
	}
	timeout := hookTimeout(chart)
	var latest *record
	if len(history) > 0 {
		latest = history[len(history)-1]
		switch latest.release.Info.Status {
		case release.StatusPendingInstall, release.StatusPendingUpgrade:
			return c.resume(ctx, chart, latest, history, timeout)
		case release.StatusUninstalling:
			if err := c.runUninstall(ctx, history, timeout); err != nil {
				return err
			}
			history, latest = nil, nil
		case release.StatusDeployed:
			if latest.configHash() == hash {
				return nil
			}
		case release.StatusFailed:
			switch failurePolicy(chart, config) {
			case helmv1.FailurePolicyAbort:
				return &held{message: "Release status is 'failed' and failure policy is 'abort', not 'reinstall'; waiting for operator intervention"}
			case helmv1.FailurePolicyReinstall:
				if err := c.runUninstall(ctx, history, timeout); err != nil {
					return err
				}
				history, latest = nil, nil
			}
		}
	}
	values, err := MergedValues(ctx, chart, config, c.secrets)
	if err != nil {
		return err
	}
	source := &Source{HTTP: c.HTTP, Secrets: c.secrets}
	archive, err := source.Fetch(ctx, chart)
	if err != nil {
		return err
	}
	caps, err := c.Capabilities()
	if err != nil {
		return err
	}
	revision := 1
	retry := false
	if latest != nil {
		revision = latest.release.Version + 1
		if latest.release.Info.Status == release.StatusFailed && latest.configHash() == hash {
			revision, retry = latest.release.Version, true
		}
	}
	upgrade := latestDeployed(history) != nil
	rendered, err := Render(RenderInput{
		Archive:      archive,
		ReleaseName:  name,
		Namespace:    namespace,
		Revision:     revision,
		IsUpgrade:    upgrade,
		Values:       values,
		Capabilities: caps,
		Lookup:       c.Lookup,
	})
	if err != nil {
		return err
	}
	if err := c.prepare(ctx, chart, rendered); err != nil {
		return err
	}
	now := helmtime.Time{Time: c.now()}
	rel := &release.Release{
		Name:      name,
		Namespace: namespace,
		Version:   revision,
		Chart:     rendered.Chart,
		Config:    rendered.Values,
		Manifest:  rendered.Manifest,
		Hooks:     rendered.Hooks,
		Info:      &release.Info{FirstDeployed: now, LastDeployed: now, Status: release.StatusPendingInstall, Description: "Initial install underway", Notes: rendered.Notes},
		Labels:    map[string]string{KeyConfigHash: hash},
	}
	if upgrade {
		rel.Info.Status = release.StatusPendingUpgrade
		rel.Info.Description = "Preparing upgrade"
	}
	if latest != nil {
		rel.Info.FirstDeployed = latest.release.Info.FirstDeployed
	}
	var rec *record
	if retry {
		latest.release = rel
		if err := releases.update(ctx, latest); err != nil {
			return err
		}
		rec = latest
	} else {
		if rec, err = releases.create(ctx, rel); err != nil {
			return err
		}
		history = append(history[:len(history):len(history)], rec)
	}
	return c.resume(ctx, chart, rec, history, timeout)
}

func (c *Controller) prepare(ctx context.Context, chart *helmv1.HelmChart, rendered *Rendered) error {
	name, namespace := chart.Name, targetNamespace(chart)
	cl := c.cluster()
	if chart.Spec.CreateNamespace {
		ns := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace"}}
		ns.SetName(namespace)
		if err := cl.createIfMissing(ctx, ns); err != nil {
			return fmt.Errorf("create namespace %s: %w", namespace, err)
		}
	}
	for _, crd := range rendered.CRDs {
		objs, err := addons.Decode(crd.File.Data)
		if err != nil {
			return fmt.Errorf("decode %s: %w", crd.Filename, err)
		}
		for _, obj := range objs {
			if err := cl.createIfMissing(ctx, obj); err != nil {
				return fmt.Errorf("create %s %s: %w", obj.GetKind(), obj.GetName(), err)
			}
		}
	}
	if chart.Spec.TakeOwnership {
		return nil
	}
	objs, err := addons.Decode([]byte(rendered.Manifest))
	if err != nil {
		return fmt.Errorf("decode release manifest: %w", err)
	}
	return cl.checkOwnership(ctx, objs, name, namespace)
}

func (c *Controller) resume(ctx context.Context, chart *helmv1.HelmChart, rec *record, history []*record, timeout time.Duration) error {
	rel := rec.release
	pre, post, description := release.HookPreInstall, release.HookPostInstall, "Install complete"
	if rel.Info.Status == release.StatusPendingUpgrade {
		pre, post, description = release.HookPreUpgrade, release.HookPostUpgrade, "Upgrade complete"
	}
	hooks := c.hookRun(rec, timeout)
	if err := hooks.event(ctx, pre); err != nil {
		return c.failRelease(ctx, rec, err)
	}
	if !hooks.started(post) {
		if err := c.applyRelease(ctx, chart, rel, others(history, rec)); err != nil {
			return c.failRelease(ctx, rec, err)
		}
	}
	if err := hooks.event(ctx, post); err != nil {
		return c.failRelease(ctx, rec, err)
	}
	rel.Info.Status = release.StatusDeployed
	rel.Info.Description = description
	rel.Info.LastDeployed = helmtime.Time{Time: c.now()}
	return c.saveDeployed(ctx, rec, history)
}

func (c *Controller) failRelease(ctx context.Context, rec *record, cause error) error {
	if _, ok := waitingUntil(cause); ok {
		return cause
	}
	rec.release.Info.Status = release.StatusFailed
	rec.release.Info.Description = fmt.Sprintf("failed: %v", cause)
	if err := c.store().update(ctx, rec); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func others(history []*record, current *record) []*record {
	var rest []*record
	for _, rec := range history {
		if rec.release.Version != current.release.Version {
			rest = append(rest, rec)
		}
	}
	return rest
}

func (c *Controller) applyRelease(ctx context.Context, chart *helmv1.HelmChart, rel *release.Release, history []*record) error {
	cl := c.cluster()
	objs, err := addons.Decode([]byte(rel.Manifest))
	if err != nil {
		return fmt.Errorf("decode release manifest: %w", err)
	}
	keep := map[string]bool{}
	for _, obj := range objs {
		if err := cl.applyOwned(ctx, obj, rel.Name, rel.Namespace); err != nil {
			return err
		}
		keep[objectKey(obj)] = true
	}
	previous, err := releaseObjects(history)
	if err != nil {
		return err
	}
	return cl.remove(ctx, previous, rel.Namespace, keep)
}

func releaseObjects(history []*record) ([]*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	for _, rec := range previousManifests(history) {
		decoded, err := addons.Decode([]byte(rec.release.Manifest))
		if err != nil {
			return nil, fmt.Errorf("decode release %d: %w", rec.release.Version, err)
		}
		objs = append(objs, decoded...)
	}
	return objs, nil
}

func previousManifests(history []*record) []*record {
	var records []*record
	if len(history) > 0 {
		records = append(records, history[len(history)-1])
	}
	if deployed := latestDeployed(history); deployed != nil && (len(records) == 0 || deployed != records[0]) {
		records = append(records, deployed)
	}
	return records
}

func (c *Controller) saveDeployed(ctx context.Context, rec *record, history []*record) error {
	releases := c.store()
	if err := releases.update(ctx, rec); err != nil {
		return err
	}
	for _, other := range history {
		if other != rec && other.release.Info.Status == release.StatusDeployed {
			if err := releases.supersede(ctx, other); err != nil {
				return err
			}
		}
	}
	return releases.trim(ctx, history)
}

func (c *Controller) runUninstall(ctx context.Context, history []*record, timeout time.Duration) error {
	rec := history[len(history)-1]
	releases := c.store()
	if rec.release.Info.Status != release.StatusUninstalling {
		rec.release.Info.Status = release.StatusUninstalling
		rec.release.Info.Deleted = helmtime.Time{Time: c.now()}
		rec.release.Info.Description = "Deletion in progress (or silently failed)"
		if err := releases.update(ctx, rec); err != nil {
			return err
		}
	}
	hooks := c.hookRun(rec, timeout)
	if err := hooks.event(ctx, release.HookPreDelete); err != nil {
		return err
	}
	if !hooks.started(release.HookPostDelete) {
		objs, err := releaseObjects(history)
		if err != nil {
			return err
		}
		if err := c.cluster().remove(ctx, objs, rec.release.Namespace, nil); err != nil && !apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
			return err
		}
	}
	postErr := hooks.event(ctx, release.HookPostDelete)
	if _, ok := waitingUntil(postErr); ok {
		return postErr
	}
	for _, rec := range history {
		if err := releases.delete(ctx, rec); err != nil {
			return err
		}
	}
	return postErr
}

func (c *Controller) uninstall(ctx context.Context, u *unstructured.Unstructured, chart *helmv1.HelmChart) error {
	if !slices.Contains(u.GetFinalizers(), Finalizer) {
		return nil
	}
	history, err := c.store().history(ctx, targetNamespace(chart), chart.Name)
	if err != nil {
		return err
	}
	if len(history) > 0 {
		var failure *hookFailure
		if err := c.runUninstall(ctx, history, hookTimeout(chart)); err != nil && !errors.As(err, &failure) {
			return err
		}
	}
	released := u.DeepCopy()
	var finalizers []string
	for _, f := range released.GetFinalizers() {
		if f != Finalizer {
			finalizers = append(finalizers, f)
		}
	}
	released.SetFinalizers(finalizers)
	_, err = c.Client.Resource(helmChartsGVR).Namespace(u.GetNamespace()).Update(ctx, released, metav1.UpdateOptions{})
	return err
}
