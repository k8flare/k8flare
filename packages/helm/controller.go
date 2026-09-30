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

func (c *Controller) Reconcile(ctx context.Context) error {
	charts, err := c.Client.Resource(helmChartsGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	configs := map[string]*helmv1.HelmChartConfig{}
	list, err := c.Client.Resource(helmConfigsGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if list != nil {
		for i := range list.Items {
			var config helmv1.HelmChartConfig
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &config); err != nil {
				return err
			}
			configs[config.Namespace+"/"+config.Name] = &config
		}
	}
	var errs []error
	for i := range charts.Items {
		u := &charts.Items[i]
		if err := c.reconcileChart(ctx, u, configs[u.GetNamespace()+"/"+u.GetName()]); err != nil {
			errs = append(errs, fmt.Errorf("HelmChart %s/%s: %w", u.GetNamespace(), u.GetName(), err))
		}
	}
	return errors.Join(errs...)
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

func (c *Controller) reconcileChart(ctx context.Context, u *unstructured.Unstructured, config *helmv1.HelmChartConfig) error {
	var chart helmv1.HelmChart
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &chart); err != nil {
		return err
	}
	if !manages(&chart) {
		return nil
	}
	if chart.DeletionTimestamp != nil {
		return c.uninstall(ctx, u, &chart)
	}
	u, err := c.claim(ctx, u)
	if err != nil {
		return err
	}
	switch chart.Spec.HelmVersion {
	case "", "v3":
	default:
		return c.setStatus(ctx, u, "Unsupported version", "Only Helm v3 charts are supported")
	}
	if err := c.install(ctx, &chart, config); err != nil {
		if statusErr := c.setStatus(ctx, u, "Install failed", err.Error()); statusErr != nil {
			return errors.Join(err, statusErr)
		}
		return err
	}
	return c.setStatus(ctx, u, "", "")
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

func configHash(chart *helmv1.HelmChart, config *helmv1.HelmChartConfig) (string, error) {
	input := struct {
		Chart  helmv1.HelmChartSpec        `json:"chart"`
		Config *helmv1.HelmChartConfigSpec `json:"config,omitempty"`
	}{Chart: chart.Spec}
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

func (c *Controller) install(ctx context.Context, chart *helmv1.HelmChart, config *helmv1.HelmChartConfig) error {
	name, namespace := chart.Name, targetNamespace(chart)
	hash, err := configHash(chart, config)
	if err != nil {
		return err
	}
	releases := c.store()
	history, err := releases.history(ctx, namespace, name)
	if err != nil {
		return err
	}
	var latest *record
	if len(history) > 0 {
		latest = history[len(history)-1]
		if latest.release.Info.Status == release.StatusDeployed && latest.configHash() == hash {
			return nil
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
	rendered, err := Render(RenderInput{
		Archive:      archive,
		ReleaseName:  name,
		Namespace:    namespace,
		Revision:     revision,
		IsUpgrade:    latest != nil,
		Values:       values,
		Capabilities: caps,
		Lookup:       c.Lookup,
	})
	if err != nil {
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
		Info:      &release.Info{FirstDeployed: now, LastDeployed: now, Status: release.StatusDeployed, Description: "Install complete", Notes: rendered.Notes},
		Labels:    map[string]string{KeyConfigHash: hash},
	}
	if latest != nil {
		rel.Info.FirstDeployed = latest.release.Info.FirstDeployed
		rel.Info.Description = "Upgrade complete"
	}
	if applyErr := c.applyRelease(ctx, chart, rendered, history); applyErr != nil {
		rel.Info.Status = release.StatusFailed
		rel.Info.Description = fmt.Sprintf("failed: %v", applyErr)
		if err := c.saveFailed(ctx, rel, latest, retry); err != nil {
			return errors.Join(applyErr, err)
		}
		return applyErr
	}
	return c.saveDeployed(ctx, rel, history, latest, retry)
}

func (c *Controller) applyRelease(ctx context.Context, chart *helmv1.HelmChart, rendered *Rendered, history []*record) error {
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
	keep := map[string]bool{}
	for _, m := range rendered.Manifests {
		objs, err := addons.Decode([]byte(m.Content))
		if err != nil {
			return fmt.Errorf("decode %s: %w", m.Name, err)
		}
		for _, obj := range objs {
			if err := cl.applyOwned(ctx, obj, name, namespace, chart.Spec.TakeOwnership); err != nil {
				return err
			}
			keep[objectKey(obj)] = true
		}
	}
	var previous []*unstructured.Unstructured
	for _, rec := range previousManifests(history) {
		objs, err := addons.Decode([]byte(rec.release.Manifest))
		if err != nil {
			return fmt.Errorf("decode release %d: %w", rec.release.Version, err)
		}
		previous = append(previous, objs...)
	}
	return cl.remove(ctx, previous, namespace, keep)
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

func (c *Controller) saveFailed(ctx context.Context, rel *release.Release, latest *record, retry bool) error {
	releases := c.store()
	if retry {
		latest.release = rel
		return releases.update(ctx, latest)
	}
	return releases.create(ctx, rel)
}

func (c *Controller) saveDeployed(ctx context.Context, rel *release.Release, history []*record, latest *record, retry bool) error {
	releases := c.store()
	if retry {
		latest.release = rel
		if err := releases.update(ctx, latest); err != nil {
			return err
		}
	} else if err := releases.create(ctx, rel); err != nil {
		return err
	}
	for _, rec := range history {
		if rec.release.Info.Status == release.StatusDeployed && rec.release.Version != rel.Version {
			if err := releases.supersede(ctx, rec); err != nil {
				return err
			}
		}
	}
	return releases.trim(ctx, history)
}

func (c *Controller) uninstall(ctx context.Context, u *unstructured.Unstructured, chart *helmv1.HelmChart) error {
	if !slices.Contains(u.GetFinalizers(), Finalizer) {
		return nil
	}
	namespace := targetNamespace(chart)
	releases := c.store()
	history, err := releases.history(ctx, namespace, chart.Name)
	if err != nil {
		return err
	}
	var objs []*unstructured.Unstructured
	for _, rec := range previousManifests(history) {
		decoded, err := addons.Decode([]byte(rec.release.Manifest))
		if err != nil {
			return fmt.Errorf("decode release %d: %w", rec.release.Version, err)
		}
		objs = append(objs, decoded...)
	}
	if err := c.cluster().remove(ctx, objs, namespace, nil); err != nil && !apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		return err
	}
	for _, rec := range history {
		if err := releases.delete(ctx, rec); err != nil {
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
