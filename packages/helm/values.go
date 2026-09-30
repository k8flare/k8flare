package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"helm.sh/helm/v3/pkg/strvals"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
)

type SecretGetter func(ctx context.Context, namespace, name string) (map[string][]byte, error)

var unescapedComma = regexp.MustCompile(`\\*,`)

func MergedValues(ctx context.Context, chart *helmv1.HelmChart, config *helmv1.HelmChartConfig, secrets SecretGetter) (map[string]any, error) {
	merged := map[string]any{}
	layers := []layer{{chart.Spec.ValuesContent, chart.Spec.Values, chart.Spec.ValuesSecrets}}
	if config != nil {
		layers = append(layers, layer{config.Spec.ValuesContent, config.Spec.Values, config.Spec.ValuesSecrets})
	}
	for _, l := range layers {
		docs, err := l.documents(ctx, chart, secrets)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			parsed := map[string]any{}
			if err := yaml.Unmarshal(doc, &parsed); err != nil {
				return nil, fmt.Errorf("parse values: %w", err)
			}
			merged = mergeMaps(merged, parsed)
		}
	}
	if err := applySet(merged, chart.Spec.Set); err != nil {
		return nil, err
	}
	return merged, nil
}

type layer struct {
	content string
	values  *apiextv1.JSON
	secrets []helmv1.SecretSpec
}

func (l layer) documents(ctx context.Context, chart *helmv1.HelmChart, secrets SecretGetter) ([][]byte, error) {
	var docs [][]byte
	if l.content != "" {
		docs = append(docs, []byte(l.content))
	}
	if raw, ok := rawValues(l.values); ok {
		docs = append(docs, raw)
	}
	for _, ref := range l.secrets {
		if len(ref.Keys) == 0 || ref.Name == "chart-values-"+chart.Name {
			continue
		}
		if secrets == nil {
			return nil, fmt.Errorf("valuesSecrets %s: no secret reader", ref.Name)
		}
		data, err := secrets(ctx, chart.Namespace, ref.Name)
		if err != nil {
			if ref.IgnoreUpdates {
				continue
			}
			return nil, fmt.Errorf("valuesSecrets %s: %w", ref.Name, err)
		}
		for _, key := range ref.Keys {
			value, ok := data[key]
			if !ok {
				return nil, fmt.Errorf("valuesSecrets %s: no key %q", ref.Name, key)
			}
			docs = append(docs, value)
		}
	}
	return docs, nil
}

func rawValues(values *apiextv1.JSON) ([]byte, bool) {
	if values == nil {
		return nil, false
	}
	raw := string(values.Raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return nil, false
	}
	return values.Raw, true
}

func applySet(values map[string]any, set map[string]intstr.IntOrString) error {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := set[k]
		var err error
		if isTyped(v) {
			err = strvals.ParseInto(fmt.Sprintf("%s=%s", k, v.String()), values)
		} else {
			err = strvals.ParseIntoString(fmt.Sprintf("%s=%s", k, unescapedComma.ReplaceAllStringFunc(v.String(), escapeComma)), values)
		}
		if err != nil {
			return fmt.Errorf("set %s: %w", k, err)
		}
	}
	return nil
}

func isTyped(v intstr.IntOrString) bool {
	if v.Type == intstr.Int {
		return true
	}
	switch strings.ToLower(v.StrVal) {
	case "true", "false", "null":
		return true
	}
	return false
}

func escapeComma(match string) string {
	if len(match)%2 == 1 {
		return `\` + match
	}
	return match
}

func mergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if next, ok := v.(map[string]any); ok {
			if prev, ok := out[k].(map[string]any); ok {
				out[k] = mergeMaps(prev, next)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func deepCopy(values map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	return out, json.Unmarshal(raw, &out)
}
