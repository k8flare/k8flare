package helm

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"k8s.io/client-go/rest"
)

const notesFileSuffix = "NOTES.txt"

type RenderInput struct {
	Archive      []byte
	ReleaseName  string
	Namespace    string
	Revision     int
	IsUpgrade    bool
	Values       map[string]any
	Capabilities *chartutil.Capabilities
	Lookup       *rest.Config
}

type Rendered struct {
	Chart     *chart.Chart
	Manifest  string
	Manifests []releaseutil.Manifest
	Hooks     []*release.Hook
	Notes     string
	CRDs      []chart.CRD
	Values    map[string]any
}

func Render(in RenderInput) (*Rendered, error) {
	ch, err := loader.LoadArchive(bytes.NewReader(in.Archive))
	if err != nil {
		return nil, fmt.Errorf("load chart: %w", err)
	}
	if ch.Metadata.Type != "" && ch.Metadata.Type != "application" {
		return nil, fmt.Errorf("%s charts are not installable", ch.Metadata.Type)
	}
	if err := checkDependencies(ch); err != nil {
		return nil, err
	}
	caps := in.Capabilities
	if caps == nil {
		caps = chartutil.DefaultCapabilities
	}
	if ch.Metadata.KubeVersion != "" && !chartutil.IsCompatibleRange(ch.Metadata.KubeVersion, caps.KubeVersion.String()) {
		return nil, fmt.Errorf("chart requires kubeVersion: %s which is incompatible with Kubernetes %s", ch.Metadata.KubeVersion, caps.KubeVersion.String())
	}
	user, err := deepCopy(in.Values)
	if err != nil {
		return nil, err
	}
	processed, err := deepCopy(in.Values)
	if err != nil {
		return nil, err
	}
	if err := chartutil.ProcessDependenciesWithMerge(ch, processed); err != nil {
		return nil, err
	}
	options := chartutil.ReleaseOptions{
		Name:      in.ReleaseName,
		Namespace: in.Namespace,
		Revision:  in.Revision,
		IsInstall: !in.IsUpgrade,
		IsUpgrade: in.IsUpgrade,
	}
	renderValues, err := chartutil.ToRenderValues(ch, processed, options, caps)
	if err != nil {
		return nil, err
	}
	var files map[string]string
	if in.Lookup != nil {
		files, err = engine.New(in.Lookup).Render(ch, renderValues)
	} else {
		files, err = engine.Engine{}.Render(ch, renderValues)
	}
	if err != nil {
		return nil, err
	}
	var notes strings.Builder
	for name, content := range files {
		if strings.HasSuffix(name, notesFileSuffix) {
			if name == path.Join(ch.Name(), "templates", notesFileSuffix) {
				notes.WriteString(content)
			}
			delete(files, name)
		}
	}
	hooks, manifests, err := releaseutil.SortManifests(files, nil, releaseutil.InstallOrder)
	if err != nil {
		return nil, err
	}
	var manifest strings.Builder
	for _, m := range manifests {
		fmt.Fprintf(&manifest, "---\n# Source: %s\n%s\n", m.Name, m.Content)
	}
	return &Rendered{
		Chart:     ch,
		Manifest:  manifest.String(),
		Manifests: manifests,
		Hooks:     hooks,
		Notes:     notes.String(),
		CRDs:      ch.CRDObjects(),
		Values:    user,
	}, nil
}

func checkDependencies(ch *chart.Chart) error {
	have := map[string]bool{}
	for _, dep := range ch.Dependencies() {
		have[dep.Name()] = true
	}
	var missing []string
	for _, dep := range ch.Metadata.Dependencies {
		if !have[dep.Name] {
			missing = append(missing, dep.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("found in Chart.yaml, but missing in charts/ directory: %s", strings.Join(missing, ", "))
	}
	return nil
}
