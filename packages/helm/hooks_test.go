package helm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/release"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

var (
	jobsGVR = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	podsGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
)

const (
	appTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: hooked-app
data:
  install: "{{ .Release.IsInstall }}"
`
	preConfigTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: hooked-pre-config
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-weight": "-1"
`
	preJobTemplate = `apiVersion: batch/v1
kind: Job
metadata:
  name: hooked-pre
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-delete-policy": hook-succeeded
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: pre
          image: busybox
`
	postPodTemplate = `apiVersion: v1
kind: Pod
metadata:
  name: hooked-post
  annotations:
    "helm.sh/hook": post-install
spec:
  restartPolicy: Never
  containers:
    - name: post
      image: busybox
`
	failingJobTemplate = `apiVersion: batch/v1
kind: Job
metadata:
  name: hooked-pre
  annotations:
    "helm.sh/hook": pre-install
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: pre
          image: busybox
`
	preDeleteJobTemplate = `apiVersion: batch/v1
kind: Job
metadata:
  name: hooked-cleanup
  annotations:
    "helm.sh/hook": pre-delete
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: cleanup
          image: busybox
`
)

func hookedArchive(t *testing.T, templates map[string]string) string {
	t.Helper()
	files := map[string]string{"hooked/Chart.yaml": "apiVersion: v2\nname: hooked\nversion: 0.1.0\n"}
	for name, content := range templates {
		files["hooked/templates/"+name] = content
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func hookedChart(t *testing.T, templates map[string]string, spec map[string]any) *unstructured.Unstructured {
	base := map[string]any{"chart": "", "repo": "", "chartContent": hookedArchive(t, templates)}
	for k, v := range spec {
		base[k] = v
	}
	return helmChartObject("", base)
}

type clock struct {
	now time.Time
}

func newHookController(client *fake.FakeDynamicClient) (*Controller, *clock) {
	c := newController(client)
	at := &clock{now: time.Unix(1_700_000_000, 0)}
	c.Now = func() time.Time { return at.now }
	return c, at
}

func reconcile(t *testing.T, c *Controller) time.Time {
	t.Helper()
	wake, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return wake
}

func setStatus(t *testing.T, client *fake.FakeDynamicClient, gvr schema.GroupVersionResource, name string, status map[string]any) {
	t.Helper()
	obj := getObject(t, client, gvr, "apps", name)
	obj.Object["status"] = status
	if err := client.Tracker().Update(gvr, obj, "apps"); err != nil {
		t.Fatal(err)
	}
}

func jobCondition(kind, reason string) map[string]any {
	return map[string]any{"conditions": []any{map[string]any{"type": kind, "status": "True", "reason": reason}}}
}

func lastRelease(t *testing.T, c *Controller) *release.Release {
	t.Helper()
	history := releaseHistory(t, c, "apps")
	if len(history) == 0 {
		t.Fatal("no release recorded")
	}
	return history[len(history)-1].release
}

func markDeleted(t *testing.T, client *fake.FakeDynamicClient) {
	t.Helper()
	stored := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	now := metav1.NewTime(time.Now())
	stored.SetDeletionTimestamp(&now)
	if _, err := client.Resource(helmChartsGVR).Namespace("kube-system").Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestInstallHooksRunInWeightOrderAndWaitAcrossPasses(t *testing.T) {
	client := newFakeClient(hookedChart(t, map[string]string{
		"app.yaml":        appTemplate,
		"pre-config.yaml": preConfigTemplate,
		"pre-job.yaml":    preJobTemplate,
		"post-pod.yaml":   postPodTemplate,
	}, nil))
	c, at := newHookController(client)
	started := at.now

	if wake := reconcile(t, c); !wake.Equal(started.Add(defaultHookTimeout)) {
		t.Fatalf("wake = %v, want the pre-install Job's timeout", wake)
	}
	if !exists(client, configMapsGVR, "apps", "hooked-pre-config") || !exists(client, jobsGVR, "apps", "hooked-pre") {
		t.Fatal("pre-install hooks were not created")
	}
	if exists(client, configMapsGVR, "apps", "hooked-app") {
		t.Fatal("release objects applied before the pre-install Job finished")
	}
	if rel := lastRelease(t, c); rel.Info.Status != release.StatusPendingInstall {
		t.Fatalf("status = %v while a pre-install hook runs", rel.Info.Status)
	}

	at.now = started.Add(time.Minute)
	setStatus(t, client, jobsGVR, "hooked-pre", jobCondition("Complete", ""))
	if wake := reconcile(t, c); !wake.Equal(at.now.Add(defaultHookTimeout)) {
		t.Fatalf("wake = %v, want the post-install Pod's timeout", wake)
	}
	if exists(client, jobsGVR, "apps", "hooked-pre") {
		t.Fatal("hook-succeeded Job was not deleted")
	}
	if !exists(client, configMapsGVR, "apps", "hooked-app") || !exists(client, podsGVR, "apps", "hooked-post") {
		t.Fatal("release objects or post-install hook missing after the pre-install Job completed")
	}

	setStatus(t, client, podsGVR, "hooked-post", map[string]any{"phase": "Succeeded"})
	if wake := reconcile(t, c); !wake.IsZero() {
		t.Fatalf("wake = %v after every hook finished", wake)
	}
	rel := lastRelease(t, c)
	if rel.Info.Status != release.StatusDeployed || rel.Info.Description != "Install complete" {
		t.Fatalf("release = %+v", rel.Info)
	}
	for _, h := range rel.Hooks {
		if h.LastRun.Phase != release.HookPhaseSucceeded {
			t.Fatalf("hook %s phase = %v", h.Name, h.LastRun.Phase)
		}
	}
	if !exists(client, podsGVR, "apps", "hooked-post") || !exists(client, configMapsGVR, "apps", "hooked-pre-config") {
		t.Fatal("hooks without a delete policy were removed")
	}
}

func TestAnAPIErrorWhileCheckingAHookLeavesTheReleasePending(t *testing.T) {
	client := newFakeClient(hookedChart(t, map[string]string{"app.yaml": appTemplate, "pre-job.yaml": preJobTemplate}, nil))
	c, _ := newHookController(client)
	reconcile(t, c)
	client.PrependReactor("get", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("etcd hiccup"))
	})
	if _, err := c.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "etcd hiccup") {
		t.Fatalf("err = %v, want the API error", err)
	}
	rel := lastRelease(t, c)
	if rel.Info.Status != release.StatusPendingInstall || rel.Hooks[0].LastRun.Phase != release.HookPhaseRunning {
		t.Fatalf("release = %v, hook = %v; want the hook still running", rel.Info.Status, rel.Hooks[0].LastRun.Phase)
	}
}

func TestHookTimeoutFailsTheRelease(t *testing.T) {
	client := newFakeClient(hookedChart(t, map[string]string{"app.yaml": appTemplate, "pre-job.yaml": preJobTemplate}, map[string]any{"timeout": "30s"}))
	c, at := newHookController(client)
	if wake := reconcile(t, c); !wake.Equal(at.now.Add(30 * time.Second)) {
		t.Fatalf("wake = %v, want spec.timeout after the Job started", wake)
	}
	at.now = at.now.Add(30 * time.Second)
	_, err := c.Reconcile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for the condition") {
		t.Fatalf("err = %v, want the hook timeout", err)
	}
	if rel := lastRelease(t, c); rel.Info.Status != release.StatusFailed {
		t.Fatalf("status = %v after the hook timed out", rel.Info.Status)
	}
	if exists(client, configMapsGVR, "apps", "hooked-app") {
		t.Fatal("release objects applied although the pre-install hook timed out")
	}
}

func failedInstall(t *testing.T, spec map[string]any) (*fake.FakeDynamicClient, *Controller) {
	t.Helper()
	client := newFakeClient(hookedChart(t, map[string]string{"app.yaml": appTemplate, "pre-job.yaml": failingJobTemplate}, spec))
	c, _ := newHookController(client)
	reconcile(t, c)
	setStatus(t, client, jobsGVR, "hooked-pre", jobCondition("Failed", "BackoffLimitExceeded"))
	_, err := c.Reconcile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "job hooked-pre failed: BackoffLimitExceeded") {
		t.Fatalf("err = %v, want the failed Job", err)
	}
	return client, c
}

func TestAbortPolicyLeavesAFailedReleaseForTheOperator(t *testing.T) {
	client, c := failedInstall(t, map[string]any{"failurePolicy": "abort"})
	before := len(client.Actions())
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("err = %v, want the chart held without a retry", err)
	}
	for _, a := range client.Actions()[before:] {
		if a.GetVerb() != "list" && a.GetVerb() != "get" && a.GetResource() != helmChartsGVR {
			t.Fatalf("abort policy acted on the cluster: %v", a)
		}
	}
	if rel := lastRelease(t, c); rel.Info.Status != release.StatusFailed {
		t.Fatalf("status = %v", rel.Info.Status)
	}
	if cond := condition(t, getObject(t, client, helmChartsGVR, "kube-system", "demo")); cond["status"] != "True" || !strings.Contains(cond["message"].(string), "waiting for operator intervention") {
		t.Fatalf("condition = %v", cond)
	}
}

func TestReinstallPolicyUninstallsAFailedUpgradeAndInstallsAgain(t *testing.T) {
	client := newFakeClient(hookedChart(t, map[string]string{"app.yaml": appTemplate, "pre-job.yaml": strings.Replace(failingJobTemplate, "pre-install", "pre-upgrade", 1)}, nil))
	c, _ := newHookController(client)
	reconcile(t, c)
	stored := getObject(t, client, helmChartsGVR, "kube-system", "demo")
	unstructured.SetNestedField(stored.Object, "replicaCount: 2\n", "spec", "valuesContent")
	if _, err := client.Resource(helmChartsGVR).Namespace("kube-system").Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	reconcile(t, c)
	setStatus(t, client, jobsGVR, "hooked-pre", jobCondition("Failed", "BackoffLimitExceeded"))
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("want the failed pre-upgrade hook")
	}
	reconcile(t, c)
	history := releaseHistory(t, c, "apps")
	if len(history) != 1 || history[0].release.Version != 1 || history[0].release.Info.Description != "Install complete" {
		t.Fatalf("history = %d records, last %+v; want a fresh install as revision 1", len(history), history[len(history)-1].release.Info)
	}
	if getObject(t, client, configMapsGVR, "apps", "hooked-app").Object["data"].(map[string]any)["install"] != "true" {
		t.Fatal("reinstall rendered as an upgrade")
	}
}

func TestHookWaitsForItsPreviousObjectToGo(t *testing.T) {
	client := newFakeClient(hookedChart(t, map[string]string{"app.yaml": appTemplate, "post-pod.yaml": strings.Replace(postPodTemplate, "post-install", "pre-install", 1)}, nil))
	terminating := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod"}}
	terminating.SetName("hooked-post")
	terminating.SetNamespace("apps")
	if err := client.Tracker().Create(podsGVR, terminating, "apps"); err != nil {
		t.Fatal(err)
	}
	client.PrependReactor("delete", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	c, _ := newHookController(client)
	if wake := reconcile(t, c); !wake.IsZero() {
		t.Fatalf("wake = %v, want to wait for the deletion without a deadline", wake)
	}
	rel := lastRelease(t, c)
	if rel.Info.Status != release.StatusPendingInstall || rel.Hooks[0].LastRun.Phase != "" {
		t.Fatalf("release = %+v hook = %+v", rel.Info, rel.Hooks[0].LastRun)
	}
	if exists(client, configMapsGVR, "apps", "hooked-app") {
		t.Fatal("release objects applied before the pre-install hook ran")
	}
}

func TestUninstallRunsPreDeleteHooksBeforeRemovingObjects(t *testing.T) {
	client := newFakeClient(hookedChart(t, map[string]string{"app.yaml": appTemplate, "cleanup.yaml": preDeleteJobTemplate}, nil))
	c, _ := newHookController(client)
	reconcile(t, c)
	if exists(client, jobsGVR, "apps", "hooked-cleanup") {
		t.Fatal("pre-delete hook ran on install")
	}
	markDeleted(t, client)

	if wake := reconcile(t, c); wake.IsZero() {
		t.Fatal("no wake booked while the pre-delete Job runs")
	}
	if !exists(client, jobsGVR, "apps", "hooked-cleanup") || !exists(client, configMapsGVR, "apps", "hooked-app") {
		t.Fatal("want the pre-delete Job running and the release objects still present")
	}
	if !containsString(getObject(t, client, helmChartsGVR, "kube-system", "demo").GetFinalizers(), Finalizer) {
		t.Fatal("finalizer removed before the pre-delete hook finished")
	}

	setStatus(t, client, jobsGVR, "hooked-cleanup", jobCondition("Complete", ""))
	reconcile(t, c)
	if exists(client, configMapsGVR, "apps", "hooked-app") || len(releaseHistory(t, c, "apps")) != 0 {
		t.Fatal("release objects or history survived the uninstall")
	}
	if containsString(getObject(t, client, helmChartsGVR, "kube-system", "demo").GetFinalizers(), Finalizer) {
		t.Fatal("finalizer still set")
	}
}

func TestFailedPreDeleteHookStillReleasesTheChart(t *testing.T) {
	client := newFakeClient(hookedChart(t, map[string]string{"app.yaml": appTemplate, "cleanup.yaml": preDeleteJobTemplate}, nil))
	c, _ := newHookController(client)
	reconcile(t, c)
	markDeleted(t, client)
	reconcile(t, c)
	setStatus(t, client, jobsGVR, "hooked-cleanup", jobCondition("Failed", "BackoffLimitExceeded"))
	reconcile(t, c)
	if containsString(getObject(t, client, helmChartsGVR, "kube-system", "demo").GetFinalizers(), Finalizer) {
		t.Fatal("a failed pre-delete hook blocked the HelmChart deletion")
	}
	if !exists(client, configMapsGVR, "apps", "hooked-app") {
		t.Fatal("helm uninstall stops at a failed pre-delete hook and deletes nothing")
	}
}
