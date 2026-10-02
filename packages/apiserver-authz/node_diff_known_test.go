package authz

import (
	"fmt"
	"regexp"
	"sort"
	"testing"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
)

func TestNodeAuthorizerMatchesUpstream(t *testing.T) {
	outcomes := runOutcomes(t)
	t.Logf("questions asked: %d", len(outcomes))

	unexplained := map[string][]outcome{}
	reasonOnly := map[string]int{}

	for _, o := range outcomes {
		if !o.differs() {
			if o.upstream.reason != o.project.reason {
				reasonOnly[reasonKey(o)]++
			}
			continue
		}
		key := fmt.Sprintf("%s | %s | %s | reachable=%v", o.q.resourceKey(), o.q.verb, o.pair(), o.q.reachable())
		unexplained[key] = append(unexplained[key], o)
	}

	keys := make([]string, 0, len(unexplained))
	for key := range unexplained {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var users, namespaces, selectors, names []string
		for _, o := range unexplained[key] {
			users = append(users, o.q.user)
			namespaces = append(namespaces, fmt.Sprintf("%q", o.q.namespace))
			selectors = append(selectors, selectorKind(o.q.selector))
			names = append(names, o.q.name)
		}
		t.Errorf("difference from upstream: %s | n=%d | users=%s ns=%s sel=%s names=%s", key, len(unexplained[key]), distinct(users), distinct(namespaces), distinct(selectors), distinct(names))
	}

	reasonKeys := make([]string, 0, len(reasonOnly))
	for key := range reasonOnly {
		reasonKeys = append(reasonKeys, key)
	}
	sort.Strings(reasonKeys)
	for _, key := range reasonKeys {
		t.Logf("reason-only difference: %s | n=%d", key, reasonOnly[key])
	}
}

var nodeNameInReason = regexp.MustCompile(`'node[^']*'`)

func reasonKey(o outcome) string {
	return fmt.Sprintf("%s | %s | %s | upstream=%q project=%q", o.q.resourceKey(), o.q.verb, decisionName(o.upstream.decision),
		nodeNameInReason.ReplaceAllString(o.upstream.reason, "'<node>'"), o.project.reason)
}

func TestNodeAuthorizerDifferencesUnderOtherGates(t *testing.T) {
	variants := []struct {
		name  string
		gates map[featuregate.Feature]bool
	}{
		{"PodCertificateRequest=true", map[featuregate.Feature]bool{features.PodCertificateRequest: true}},
		{"KubeletServiceAccountTokenForCredentialProviders=false", map[featuregate.Feature]bool{features.KubeletServiceAccountTokenForCredentialProviders: false}},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for gate, enabled := range variant.gates {
				featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, gate, enabled)
			}
			counts := map[string]int{}
			for _, o := range runOutcomes(t) {
				if o.differs() && (o.q.resource == "podcertificaterequests" || (o.q.resource == "serviceaccounts" && o.q.group == "")) {
					counts[fmt.Sprintf("%s | %s | %s", o.q.resourceKey(), o.q.verb, o.pair())]++
				}
			}
			keys := make([]string, 0, len(counts))
			for key := range counts {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				t.Logf("%s | n=%d", key, counts[key])
			}
		})
	}
}
