package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/k8flare/k8flare/pkg/apiserver"
	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
	corev1 "k8s.io/api/core/v1"
)

// genDiscoveryAssets writes the per-group-version discovery documents
// (APIResourceList) as Static Assets under packages/k8flare-worker/assets/, so
// kubectl's parallel discovery fan-out (~30 concurrent requests on a
// cold cache) is served by Cloudflare's asset handler without ever
// touching the 43MB apiserver WASM -- instantiating that under the burst
// intermittently blew the isolate startup CPU budget and surfaced as
// kubectl's `couldn't get resource list ... ("unknown")` (see the
// gateway's cold-start-retry comment in packages/k8flare-worker/src/loader/apiserver.ts;
// this generator is the root fix, the retry stays as belt-and-braces).
//
// Only two-plus-segment paths are generated: `apis/<group>/<version>`
// and `api/v1`. The roots (/api, /apis) and the per-group APIGroup docs
// (/apis/<group>) CANNOT be assets -- `apis` would have to be both a
// file and a directory -- so those stay served by the Go worker
// (pkg/apiserver/discovery.go), which also keeps serving everything
// generated here as a fallback for dev setups without assets.
//
// The content mirrors pkg/apiserver/discovery.go's mux registrations,
// built from the same apidef.Table + APIResourcesForGroupVersion source
// of truth (plus the same hand-written authorization.k8s.io exception
// documented there).
func genDiscoveryAssets(root string) error {
	// assetsDir (openapi.go) is the real, current asset root. Until
	// 2026-07-27 this line instead joined a stale "workers/k8flare/assets"
	// path left over from the pre-2026-07-08 layout: every `make gen` run
	// silently created a fresh untracked workers/ tree and left the real,
	// committed assets/apis documents untouched, so a resource added to
	// apidef.Table never reached the served discovery documents (and
	// ci.yml's regen-and-diff check couldn't see it, since git diff
	// ignores untracked files). Found while adding k8flare.com/v1alpha1.
	assetsRoot := filepath.Join(root, assetsDir)

	docs := map[string]metav1.APIResourceList{
		filepath.Join("api", "v1"): {
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "v1",
			APIResources: apiserver.APIResourcesForGroupVersion(corev1.SchemeGroupVersion),
		},
	}
	for _, gv := range apidef.GroupVersions() {
		if gv.Group == "" {
			continue
		}
		docs[filepath.Join("apis", gv.Group, gv.Version)] = metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: gv.String(),
			APIResources: apiserver.APIResourcesForGroupVersion(gv),
		}
	}
	// authorization.k8s.io/v1: same one-off as discovery.go (no
	// ResourceStore behind selfsubjectaccessreviews).
	docs[filepath.Join("apis", "authorization.k8s.io", "v1")] = metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
		GroupVersion: "authorization.k8s.io/v1",
		APIResources: []metav1.APIResource{{
			Name: "selfsubjectaccessreviews", SingularName: "selfsubjectaccessreview",
			Namespaced: false, Kind: "SelfSubjectAccessReview", Verbs: []string{"create"},
		}},
	}

	// These two directories are generator-owned: wipe and rewrite, same
	// contract as openapi.go's treatment of assets/openapi.
	for _, d := range []string{"api", "apis"} {
		if err := os.RemoveAll(filepath.Join(assetsRoot, d)); err != nil {
			return err
		}
	}
	for rel, doc := range docs {
		path := filepath.Join(assetsRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		data, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", rel, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
