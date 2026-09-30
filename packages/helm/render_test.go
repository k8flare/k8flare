package helm

import (
	"context"
	"os"
	"strings"
	"testing"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"helm.sh/helm/v3/pkg/chartutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func pinnedCapabilities() *chartutil.Capabilities {
	caps := *chartutil.DefaultCapabilities
	caps.KubeVersion = chartutil.KubeVersion{Version: "v1.36.0", Major: "1", Minor: "36"}
	return &caps
}

func demoChart(t *testing.T) *helmv1.HelmChart {
	return &helmv1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "kube-system"},
		Spec: helmv1.HelmChartSpec{
			ValuesContent: string(readTestdata(t, "overrides.yaml")),
			Set: map[string]intstr.IntOrString{
				"replicaCount": intstr.FromInt32(3),
				"image.tag":    intstr.FromString("1.2"),
			},
		},
	}
}

func TestRenderMatchesHelmTemplate(t *testing.T) {
	values, err := MergedValues(context.Background(), demoChart(t), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(RenderInput{
		Archive:      readTestdata(t, "demo-0.1.0.tgz"),
		ReleaseName:  "demo",
		Namespace:    "apps",
		Revision:     1,
		Values:       values,
		Capabilities: pinnedCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(readTestdata(t, "demo.expected.yaml")))
	if got := strings.TrimSpace(rendered.Manifest); got != want {
		t.Fatalf("manifest differs from helm template\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if len(rendered.Hooks) != 1 || rendered.Hooks[0].Name != "demo-demo-test" {
		t.Fatalf("hooks = %+v, want the test pod", rendered.Hooks)
	}
	if strings.TrimSpace(rendered.Notes) != "demo demo installed" {
		t.Fatalf("notes = %q", rendered.Notes)
	}
	if len(rendered.CRDs) != 1 || rendered.CRDs[0].Filename != "demo/crds/widget.yaml" {
		t.Fatalf("crds = %+v", rendered.CRDs)
	}
}

func TestRenderSubchartCanBeDisabled(t *testing.T) {
	chart := demoChart(t)
	chart.Spec.Set["cache.enabled"] = intstr.FromString("false")
	values, err := MergedValues(context.Background(), chart, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(RenderInput{Archive: readTestdata(t, "demo-0.1.0.tgz"), ReleaseName: "demo", Namespace: "apps", Revision: 1, Values: values, Capabilities: pinnedCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.Manifest, "demo-cache") {
		t.Fatalf("subchart rendered although cache.enabled=false:\n%s", rendered.Manifest)
	}
}

func TestRenderRequiredFailsWithTheChartsMessage(t *testing.T) {
	chart := demoChart(t)
	chart.Spec.Set["service.port"] = intstr.FromString("null")
	values, err := MergedValues(context.Background(), chart, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Render(RenderInput{Archive: readTestdata(t, "demo-0.1.0.tgz"), ReleaseName: "demo", Namespace: "apps", Revision: 1, Values: values, Capabilities: pinnedCapabilities()})
	if err == nil || !strings.Contains(err.Error(), "service.port is required") {
		t.Fatalf("err = %v, want the required message", err)
	}
}

func TestRenderMarksUpgrades(t *testing.T) {
	values, _ := MergedValues(context.Background(), demoChart(t), nil, nil)
	rendered, err := Render(RenderInput{Archive: readTestdata(t, "demo-0.1.0.tgz"), ReleaseName: "demo", Namespace: "apps", Revision: 2, IsUpgrade: true, Values: values, Capabilities: pinnedCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Manifest, "install: false") {
		t.Fatalf("Release.IsInstall stayed true on an upgrade:\n%s", rendered.Manifest)
	}
}

func TestMergedValuesOrder(t *testing.T) {
	chart := &helmv1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "kube-system"},
		Spec: helmv1.HelmChartSpec{
			ValuesContent: "a: content\nb: content\nnested:\n  keep: 1\n  over: 1\n",
			ValuesSecrets: []helmv1.SecretSpec{{Name: "vals", Keys: []string{"k"}}},
			Set:           map[string]intstr.IntOrString{"c": intstr.FromString("1,2"), "d": intstr.FromString("true")},
		},
	}
	config := &helmv1.HelmChartConfig{Spec: helmv1.HelmChartConfigSpec{ValuesContent: "b: config\nnested:\n  over: 2\n"}}
	secrets := func(_ context.Context, namespace, name string) (map[string][]byte, error) {
		if namespace != "kube-system" || name != "vals" {
			t.Fatalf("secret %s/%s", namespace, name)
		}
		return map[string][]byte{"k": []byte("a: secret\n")}, nil
	}
	got, err := MergedValues(context.Background(), chart, config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	nested := got["nested"].(map[string]any)
	if got["a"] != "secret" || got["b"] != "config" || nested["keep"] != float64(1) || nested["over"] != float64(2) {
		t.Fatalf("merged = %#v", got)
	}
	if got["c"] != "1,2" {
		t.Fatalf("--set-string with a comma = %#v, want the literal string", got["c"])
	}
	if got["d"] != true {
		t.Fatalf("--set true = %#v, want a bool", got["d"])
	}
}
