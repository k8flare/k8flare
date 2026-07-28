package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime/schema"

	k8flarev1alpha1 "github.com/k8flare/k8flare/pkg/apis/k8flare/v1alpha1"
	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// assetsDir is where packages/k8flare-worker/wrangler.jsonc's "assets.directory"
// points -- Cloudflare serves any file placed here directly at the matching
// URL path, without invoking the Worker at all (confirmed via wrangler
// dev; see cmd/k8flare-gen's git history / final report for the
// verification note), so no Go route handling is needed for /openapi/*.
// (The wasm/ subtree next to these is Loader chunk supply, owned by
// `make wasm` and excluded via run_worker_first.)
const assetsDir = "packages/k8flare-worker/assets"

// ownOpenAPIGroups lists the apidef.Table GroupVersions that are k8flare's
// own, not upstream Kubernetes', and therefore have no document in
// k8s.io/kubernetes/api/openapi-spec to copy. Their v3 documents are
// generated instead (ownGroupOpenAPIV3, openapi_own.go); they are staged
// and indexed exactly like the copied upstream ones.
//
// Correction (2026-07-28): this used to be called noUpstreamOpenAPI and
// genOpenAPI skipped these groups entirely -- no v3 file, no v3 index
// entry. P1 of docs/cluster-api-design.md recorded the accepted tradeoff as
// "kubectl just does no client-side validation for this group". That was
// wrong, and it broke plain `kubectl apply` for k8flare.com/v1alpha1
// outright: a GroupVersion missing from the v3 index makes kubectl fall
// back to the OpenAPI *v2* verifier, which fetches /openapi/v2 as protobuf,
// which our Static Assets copy is not. See ownGroupOpenAPIV3's doc comment
// for the full mechanism.
var ownOpenAPIGroups = map[schema.GroupVersion]bool{
	k8flarev1alpha1.SchemeGroupVersion: true,
}

// genOpenAPI copies the real upstream OpenAPI v2 (Swagger 2.0) and v3
// documents for every apidef.Table GroupVersion out of the k8s.io/kubernetes
// module (resolved via go.mod's replace to the k3s-io/kubernetes fork) into
// packages/k8flare-worker's Static Assets directory, plus a generated v3
// discovery index. This is real upstream content, copied byte-for-byte
// (the v2 document verbatim; the v3 documents verbatim per-file) -- nothing
// here hand-writes or hand-filters an OpenAPI schema, matching the "don't
// hand-write OpenAPI" rule in this project's Phase 3 spec. Only the v3
// discovery index (a small {"paths": {...}} map keyed by each copied file's
// URL and content hash) is itself generated, from real file content, not
// hand-authored.
//
// The bare "/openapi/v3" discovery index can't be staged as a file called
// "v3" in outDir, because per-group-version documents are also served
// under "/openapi/v3/...", which needs "v3" to be a directory -- a single
// path can't be both on a real filesystem (or in Cloudflare's asset
// manifest, which is built by walking this directory). So the (small,
// ~1-2KB) index is instead generated as a Go byte literal
// (zz_generated_openapi.go) that pkg/apiserver serves directly for the one
// exact "/openapi/v3" route; every per-group-version document, and all of
// v2, stay pure static assets outside the WASM binary. See
// pkg/apiserver/discovery.go's RegisterOpenAPIDiscovery, which registers that one route.
func genOpenAPI(root string) error {
	specDir, err := goListModuleDir(root, "k8s.io/kubernetes")
	if err != nil {
		return err
	}
	specDir = filepath.Join(specDir, "api", "openapi-spec")

	outDir := filepath.Join(root, assetsDir, "openapi")
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clear %s: %w", outDir, err)
	}

	// v2 (Swagger 2.0): one full-surface document, copied verbatim to
	// /openapi/v2. Real kube-apiserver doesn't filter this per served
	// group either -- it's one file describing everything the apiserver
	// binary knows how to decode, so serving upstream's copy whole (it
	// includes groups this project doesn't implement, e.g. RBAC) matches
	// upstream's own shape rather than a k8flare-specific subset.
	v2Src := filepath.Join(specDir, "swagger.json")
	v2Data, err := os.ReadFile(v2Src)
	if err != nil {
		return fmt.Errorf("read %s (is the k8s.io/kubernetes module downloaded? try 'go mod download'): %w", v2Src, err)
	}
	if err := writeFile(filepath.Join(outDir, "v2"), v2Data); err != nil {
		return err
	}

	// v3: one document per GroupVersion this apiserver actually serves
	// (apidef.Table), each copied verbatim under outDir/v3/..., plus a
	// small generated discovery index (see doc comment above).
	discovery := struct {
		Paths map[string]struct {
			ServerRelativeURL string `json:"serverRelativeURL"`
		} `json:"paths"`
	}{Paths: map[string]struct {
		ServerRelativeURL string `json:"serverRelativeURL"`
	}{}}

	for _, gv := range apidef.GroupVersions() {
		var data []byte
		if ownOpenAPIGroups[gv] {
			data, err = ownGroupOpenAPIV3(gv)
			if err != nil {
				return fmt.Errorf("generate OpenAPI v3 doc for %s: %w", gv, err)
			}
		} else {
			srcName := v3UpstreamFileName(gv)
			src := filepath.Join(specDir, "v3", srcName)
			data, err = os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("read %s (upstream OpenAPI v3 doc for %s not found -- has the group/version name changed upstream?): %w", src, gv, err)
			}
		}

		servedPath := v3ServedPath(gv)
		if err := writeFile(filepath.Join(outDir, "v3", filepath.FromSlash(servedPath)), data); err != nil {
			return err
		}

		sum := sha256.Sum256(data)
		discovery.Paths[servedPath] = struct {
			ServerRelativeURL string `json:"serverRelativeURL"`
		}{ServerRelativeURL: fmt.Sprintf("/openapi/v3/%s?hash=%s", servedPath, hex.EncodeToString(sum[:])[:12])}
	}

	indexData, err := json.Marshal(discovery)
	if err != nil {
		return fmt.Errorf("marshal openapi v3 discovery index: %w", err)
	}
	if err := genOpenAPIV3Index(root, indexData); err != nil {
		return err
	}

	// Plain-text marker for humans browsing the directory -- JSON has no
	// comment syntax to carry the usual "DO NOT EDIT" header, and this
	// directory is fully removed and rewritten on every run regardless
	// (see os.RemoveAll above), so this file is a courtesy, not the
	// enforcement mechanism (that's ci.yml's regen-and-diff check).
	marker := "# " + generatedHeader + "\nRegenerate with: go run ./cmd/k8flare-gen\n"
	return writeFile(filepath.Join(outDir, "_generated.txt"), []byte(marker))
}

// genOpenAPIV3Index writes pkg/apiserver/zz_generated_openapi.go, embedding
// indexData (the "/openapi/v3" discovery document) as a Go byte literal --
// see genOpenAPI's doc comment for why this one small document is embedded
// in the WASM binary rather than staged as a static asset like everything
// else under openapi/.
func genOpenAPIV3Index(root string, indexData []byte) error {
	src := fmt.Sprintf(`// %s
package apiserver

// OpenAPIV3Discovery is the JSON body served at GET /openapi/v3 (see
// RegisterOpenAPIDiscovery, openapi.go): a {"paths": {...}} map from each
// served GroupVersion's OpenAPI v3 document path to its Static-Assets URL
// and content hash. Generated from the real per-group-version documents
// copied into packages/k8flare-worker/assets/openapi/v3/ -- see
// cmd/k8flare-gen/openapi.go.
var OpenAPIV3Discovery = []byte(%s)
`, generatedHeader, goRawOrQuoted(indexData))
	return writeGoFile(root+"/pkg/apiserver/zz_generated_openapi.go", []byte(src))
}

// goRawOrQuoted renders data as a Go string literal: a raw `...` literal
// when strconv.CanBackquote says it's safe (true for compact JSON, which
// is what this is always actually called with -- json.Marshal, not
// MarshalIndent), otherwise a quoted "..." literal via %q as a fallback.
func goRawOrQuoted(data []byte) string {
	s := string(data)
	if strconv.CanBackquote(s) {
		return "`" + s + "`"
	}
	return fmt.Sprintf("%q", s)
}

func v3UpstreamFileName(gv schema.GroupVersion) string {
	if gv.Group == "" {
		return fmt.Sprintf("api__%s_openapi.json", gv.Version)
	}
	return fmt.Sprintf("apis__%s__%s_openapi.json", gv.Group, gv.Version)
}

func v3ServedPath(gv schema.GroupVersion) string {
	if gv.Group == "" {
		return fmt.Sprintf("api/%s", gv.Version)
	}
	return fmt.Sprintf("apis/%s/%s", gv.Group, gv.Version)
}
