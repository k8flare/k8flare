package authz

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"k8s.io/apiserver/pkg/authorization/authorizer"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
)

type knownDifference struct {
	name        string
	direction   string
	upstream    authorizer.Decision
	project     authorizer.Decision
	upstreamAt  string
	projectAt   string
	matches     func(q question) bool
	reachable   int
	unreachable int
}

type predicate func(q question) bool

func all(ps ...predicate) predicate {
	return func(q question) bool {
		for _, p := range ps {
			if !p(q) {
				return false
			}
		}
		return true
	}
}

func anyOf(ps ...predicate) predicate {
	return func(q question) bool {
		for _, p := range ps {
			if p(q) {
				return true
			}
		}
		return false
	}
}

func resourceIn(names ...string) predicate {
	return func(q question) bool {
		for _, n := range names {
			if q.resource == n {
				return true
			}
		}
		return false
	}
}

func verbIn(verbs ...string) predicate {
	return func(q question) bool {
		for _, v := range verbs {
			if q.verb == v {
				return true
			}
		}
		return false
	}
}

func namePrefix(prefixes ...string) predicate {
	return func(q question) bool {
		for _, p := range prefixes {
			if q.name != "" && strings.HasPrefix(q.name, p) {
				return true
			}
		}
		return false
	}
}

func subresourceIs(sub string) predicate {
	return func(q question) bool { return q.subresource == sub }
}

var (
	anySubresource = func(q question) bool { return q.subresource != "" }
	noSubresource  = func(q question) bool { return q.subresource == "" }
	unnamed        = func(q question) bool { return q.name == "" }
	selectorName   = func(q question) bool { return selectorKind(q.selector) == "metadata.name" }
	correctGroup   = func(q question) bool { want, ok := upstreamGroups[q.resource]; return ok && q.group == want }
	wrongGroup     = func(q question) bool { want, ok := upstreamGroups[q.resource]; return ok && q.group != want }
)

var upstreamGroups = map[string]string{
	"nodes": "", "pods": "", "secrets": "", "configmaps": "", "persistentvolumeclaims": "", "persistentvolumes": "",
	"serviceaccounts": "", "endpoints": "", "resourceclaims": "resource.k8s.io", "resourceslices": "resource.k8s.io",
	"volumeattachments": "storage.k8s.io", "csinodes": "storage.k8s.io", "leases": "coordination.k8s.io",
}

const (
	allows  = "project-allows"
	denies  = "project-denies"
	blocks  = "project-blocks-rbac"
	allow   = authorizer.DecisionAllow
	deny    = authorizer.DecisionDeny
	noOpine = authorizer.DecisionNoOpinion
)

var knownDifferences = []knownDifference{
	{
		name: "api-group-ignored/allows", reachable: 989, unreachable: 1096, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:123-124,165 (GroupResource switch, then NodeRules)", projectAt: "node.go:42 (switch on GetResource() only)",
		matches: wrongGroup,
	},
	{
		name: "api-group-ignored/denies", reachable: 7699, unreachable: 13677, direction: blocks, upstream: noOpine, project: deny,
		upstreamAt: "node_authorizer.go:123-124,165 (GroupResource switch, then NodeRules)", projectAt: "node.go:42 (switch on GetResource() only)",
		matches: wrongGroup,
	},
	{
		name: "subresource-ignored-on-reads", reachable: 135, unreachable: 280, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:195-198,213-216,181-184,388-390 (subresource rejected)", projectAt: "node.go:243-276,284-297 (authorizeRelated and VolumeAttachment never look at the subresource)",
		matches: all(correctGroup, anySubresource, verbIn("get", "list", "watch"), resourceIn("secrets", "persistentvolumeclaims", "resourceclaims", "volumeattachments", "serviceaccounts", "nodes")),
	},
	{
		name: "node-status-create", reachable: 6, unreachable: 16, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:437-443 (status: update/patch only)", projectAt: "node.go:83-92 (create allowed for the status subresource)",
		matches: all(correctGroup, subresourceIs("status"), resourceIn("nodes"), verbIn("create")),
	},
	{
		name: "selector-derived-name", reachable: 0, unreachable: 287, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:225-228,426-434 (only GetName() is read)", projectAt: "node.go:253,390-404 (nameFromAttrs falls back to a metadata.name field selector)",
		matches: all(correctGroup, unnamed, selectorName),
	},
	{
		name: "mirror-pod-references", reachable: 60, unreachable: 24, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "graph.go:377-379 (mirror pods get no edges)", projectAt: "node.go:430-454 (podReferences has no mirror pod exception)",
		matches: all(correctGroup, namePrefix("secret-mirror-", "cm-mirror-", "pvc-mirror-", "pv-mirror-", "claim-mirror-", "sa-mirror-"), anyOf(
			all(noSubresource, verbIn("get", "list", "watch")),
			all(subresourceIs("status"), verbIn("update", "patch")),
		)),
	},
	{
		name: "default-service-account-fallback", reachable: 6, unreachable: 0, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "graph.go:384-388 (edge only when serviceAccountName is set)", projectAt: "node.go:446-451 (empty serviceAccountName counts as default)",
		matches: all(correctGroup, noSubresource, resourceIn("serviceaccounts"), verbIn("get"), namePrefix("default")),
	},
	{
		name: "pv-bound-by-pvc-volumeName", reachable: 6, unreachable: 0, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "graph.go:512-520 (PV edge needs spec.claimRef)", projectAt: "node.go:313-323 (PV found through pvc.spec.volumeName)",
		matches: all(resourceIn("persistentvolumes"), namePrefix("pv-claimless-")),
	},
	{
		name: "pv-bound-by-claimRef", reachable: 6, unreachable: 0, direction: denies, upstream: allow, project: deny,
		upstreamAt: "graph.go:512-520 (PV edge from spec.claimRef)", projectAt: "node.go:313-323 (needs pvc.spec.volumeName)",
		matches: all(resourceIn("persistentvolumes"), namePrefix("pv-prebound-")),
	},
	{
		name: "ephemeral-volume-claims", reachable: 18, unreachable: 0, direction: denies, upstream: allow, project: deny,
		upstreamAt: "graph.go:404-410 (ephemeral.VolumeClaimName)", projectAt: "node.go:546-554 (podClaimNames reads persistentVolumeClaim volumes only)",
		matches: all(resourceIn("persistentvolumeclaims"), namePrefix("p-node0-scratch", "p-node1-scratch")),
	},
	{
		name: "ephemeral-container-references", reachable: 20, unreachable: 16, direction: denies, upstream: allow, project: deny,
		upstreamAt: "graph.go:390-402 (podutil visitors include ephemeral containers)", projectAt: "node.go:474-533 (Containers and InitContainers only)",
		matches: all(resourceIn("secrets", "configmaps"), namePrefix("secret-eph-", "cm-eph-")),
	},
	{
		name: "csi-volume-secret-ref", reachable: 10, unreachable: 8, direction: denies, upstream: allow, project: deny,
		upstreamAt: "graph.go:390-395 (VisitPodSecretNames: CSI NodePublishSecretRef)", projectAt: "node.go:474-504 (podSecretNames knows secret and projected volumes only)",
		matches: all(resourceIn("secrets"), namePrefix("secret-csi-")),
	},
	{
		name: "pv-secret-edge", reachable: 36, unreachable: 28, direction: denies, upstream: allow, project: deny,
		upstreamAt: "graph.go:527-535 (secret edge from the bound PV)", projectAt: "node.go:430-454 (no PV to secret traversal)",
		matches: all(resourceIn("secrets"), namePrefix("secret-pv-", "secret-pvcsi-", "secret-pv0-")),
	},
	{
		name: "unscoped-namespace-read", reachable: 82, unreachable: 0, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:225-228 (No Object name found)", projectAt: "node.go:253-259 (list/watch without a name allowed in a namespace; node_test.go:97-104 asserts it)",
		matches: all(correctGroup, noSubresource, resourceIn("secrets", "configmaps"), verbIn("list", "watch"), unnamed),
	},
	{
		name: "pvc-list-watch", reachable: 66, unreachable: 52, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:133,190-194 (authorizeGet: get only)", projectAt: "node.go:244-245,253-259 (get, list and watch)",
		matches: all(correctGroup, noSubresource, resourceIn("persistentvolumeclaims"), verbIn("list", "watch")),
	},
	{
		name: "pvc-status-unnamed-write", reachable: 0, unreachable: 40, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:225-228 (No Object name found)", projectAt: "node.go:246-259 (update/patch of pvc status without a name allowed in a namespace)",
		matches: all(correctGroup, subresourceIs("status"), resourceIn("persistentvolumeclaims"), verbIn("update", "patch"), unnamed),
	},
	{
		name: "token-create-unchecked", reachable: 112, unreachable: 96, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:261-282 (needs a pod on the node using the service account)", projectAt: "node.go:60-63 (every serviceaccounts/token create allowed)",
		matches: all(correctGroup, subresourceIs("token"), resourceIn("serviceaccounts"), verbIn("create")),
	},
	{
		name: "lease-list-watch", reachable: 4, unreachable: 8, direction: allows, upstream: noOpine, project: allow,
		upstreamAt: "node_authorizer.go:287-294 (get, create, update, patch, delete only)", projectAt: "node.go:177-182 (list and watch of the own lease)",
		matches: all(correctGroup, resourceIn("leases"), verbIn("list", "watch")),
	},
	{
		name: "endpoints-get-restricted", reachable: 48, unreachable: 90, direction: denies, upstream: allow, project: deny,
		upstreamAt: "node_authorizer.go:165 with NodeRules get endpoints (policy.go: get endpoints)", projectAt: "node.go:328-370 (get only for endpoints reachable from the node's pods and PVs)",
		matches: all(correctGroup, resourceIn("endpoints"), verbIn("get")),
	},
	{
		name: "node-write-other-name", reachable: 52, unreachable: 12, direction: denies, upstream: allow, project: noOpine,
		upstreamAt: "node_authorizer.go:420-422,439-441 (name checked by NodeRestriction admission)", projectAt: "node.go:88-92 (name must be empty or the node's own)",
		matches: all(correctGroup, resourceIn("nodes"), verbIn("create", "update", "patch")),
	},
	{
		name: "project-denies-where-upstream-abstains", reachable: 10253, unreachable: 20589, direction: blocks, upstream: noOpine, project: deny,
		upstreamAt: "node_authorizer.go (every restriction returns NoOpinion, never Deny)", projectAt: "node.go:49-67,244-296 (authorizeRelated, PV, VolumeAttachment, endpoints, CSINode return Deny)",
		matches: all(correctGroup, resourceIn("secrets", "configmaps", "persistentvolumeclaims", "persistentvolumes", "resourceclaims", "volumeattachments", "serviceaccounts", "endpoints", "csinodes")),
	},
}

func TestNodeAuthorizerMatchesUpstream(t *testing.T) {
	outcomes := runOutcomes(t)
	t.Logf("questions asked: %d", len(outcomes))

	reachableCounts := make([]int, len(knownDifferences))
	unreachableCounts := make([]int, len(knownDifferences))
	examples := make([]outcome, len(knownDifferences))
	unexplained := map[string][]outcome{}
	reasonOnly := map[string]int{}
	differing := 0

	for _, o := range outcomes {
		if !o.differs() {
			if o.upstream.reason != o.project.reason {
				reasonOnly[reasonKey(o)]++
			}
			continue
		}
		differing++
		hit := -1
		for i, k := range knownDifferences {
			if k.upstream == o.upstream.decision && k.project == o.project.decision && k.matches(o.q) {
				hit = i
				break
			}
		}
		if hit < 0 {
			key := fmt.Sprintf("%s | %s | %s | reachable=%v", o.q.resourceKey(), o.q.verb, o.pair(), o.q.reachable())
			unexplained[key] = append(unexplained[key], o)
			continue
		}
		if o.q.reachable() {
			reachableCounts[hit]++
		} else {
			unreachableCounts[hit]++
		}
		if examples[hit].q.verb == "" {
			examples[hit] = o
		}
	}
	t.Logf("differing questions: %d", differing)

	for i, k := range knownDifferences {
		t.Logf("known difference %q (%s, upstream=%s project=%s): reachable=%d unreachable=%d e.g. %s upstream-reason=%q project-reason=%q", k.name, k.direction, decisionName(k.upstream), decisionName(k.project), reachableCounts[i], unreachableCounts[i], examples[i].q, examples[i].upstream.reason, examples[i].project.reason)
		if reachableCounts[i] != k.reachable || unreachableCounts[i] != k.unreachable {
			t.Errorf("known difference %q: expected reachable=%d unreachable=%d, observed reachable=%d unreachable=%d", k.name, k.reachable, k.unreachable, reachableCounts[i], unreachableCounts[i])
		}
		if got := directionOf(k.upstream, k.project); got != k.direction {
			t.Errorf("known difference %q: declared direction %s but decisions imply %s", k.name, k.direction, got)
		}
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
		t.Errorf("unexplained difference: %s | n=%d | users=%s ns=%s sel=%s names=%s", key, len(unexplained[key]), distinct(users), distinct(namespaces), distinct(selectors), distinct(names))
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

func directionOf(upstream, project authorizer.Decision) string {
	switch {
	case project == authorizer.DecisionAllow:
		return "project-allows"
	case upstream == authorizer.DecisionAllow:
		return "project-denies"
	}
	return "project-blocks-rbac"
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
