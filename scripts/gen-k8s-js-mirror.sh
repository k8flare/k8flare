#!/usr/bin/env bash
# Regenerates .build/k8s-js-mirror/: a full local copy of the upstream
# k3s-io/kubernetes module (the fork k8s.io/kubernetes resolves to, see
# go.mod) with exactly two files swapped for a GOOS=js-compatible
# pkg/scheduler/backend/cache/debugger/signal.go.
#
# Why this exists, and why go.mod's `k8s.io/kubernetes` replace points
# here instead of straight at github.com/k3s-io/kubernetes: see
# pkg/k8s-js-overlays/README.md. Short version: `go build
# -overlay` cannot patch files inside GOMODCACHE ("Files beneath
# GOMODCACHE must not be replaced" -- verified by actually trying), so
# the only way to change this one file's content is a module-level
# `replace` to a local directory. This script generates that directory
# without committing a multi-hundred-MB copy of k8s.io/kubernetes to git.
#
# go.mod's replace line depends on .build/k8s-js-mirror existing on disk
# -- run this after every fresh clone and after editing
# pkg/k8s-js-overlays/upstream-module.txt (k8s version bump).
# `npm run build:wasm` runs it automatically; run it by hand for host
# builds (cmd/agent, cmd/scheduler, cmd/controller-manager, go vet, etc.)
# if you haven't run a wasm build yet in a fresh checkout.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OVERLAY_DIR="$ROOT/pkg/k8s-js-overlays"
DST="$ROOT/.build/k8s-js-mirror"

UPSTREAM_MODULE="$(awk '{print $1}' "$OVERLAY_DIR/upstream-module.txt")"
UPSTREAM_VERSION="$(awk '{print $2}' "$OVERLAY_DIR/upstream-module.txt")"

echo "gen-k8s-js-mirror: resolving ${UPSTREAM_MODULE}@${UPSTREAM_VERSION}..."
# Deliberately go.mod-replace-state-independent: this asks about
# github.com/k3s-io/kubernetes directly (not k8s.io/kubernetes, whose
# replace target is this script's own output once installed), so it
# resolves correctly whether go.mod's k8s.io/kubernetes replace currently
# points at the upstream fork or at .build/k8s-js-mirror itself.
SRC="$(go mod download -json "${UPSTREAM_MODULE}@${UPSTREAM_VERSION}" | jq -r '.Dir')"
if [[ -z "$SRC" || "$SRC" == "null" || ! -d "$SRC" ]]; then
  echo "gen-k8s-js-mirror: failed to resolve module dir for ${UPSTREAM_MODULE}@${UPSTREAM_VERSION}" >&2
  exit 1
fi

# Drift check: fail loudly if upstream signal.go changed since the
# overlay was last hand-reviewed, instead of silently re-patching a file
# that may have grown new content the overlay doesn't account for
# (CLAUDE.md rule: k8s bumps must be reviewed, not mechanically pinned).
UPSTREAM_SIGNAL="$SRC/pkg/scheduler/backend/cache/debugger/signal.go"
EXPECTED_SHA="$(awk '{print $1}' "$OVERLAY_DIR/upstream-signal.go.sha256")"
ACTUAL_SHA="$(shasum -a 256 "$UPSTREAM_SIGNAL" | awk '{print $1}')"
if [[ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]]; then
  echo "gen-k8s-js-mirror: upstream signal.go changed since the overlay was last reviewed." >&2
  echo "  expected sha256 $EXPECTED_SHA, got $ACTUAL_SHA" >&2
  echo "  Diff $UPSTREAM_SIGNAL against $OVERLAY_DIR/signal_notjs.go, update" >&2
  echo "  signal_notjs.go/signal_js.go if the change matters for GOOS=js, then" >&2
  echo "  refresh upstream-signal.go.sha256. See docs/k8s-version-bump.md." >&2
  exit 1
fi

echo "gen-k8s-js-mirror: copying $SRC -> $DST..."
rm -rf "$DST"
mkdir -p "$(dirname "$DST")"
# APFS (macOS): clonefile-backed copy-on-write, ~instant and ~0 extra disk.
# Linux (CI): --reflink=auto degrades to a plain copy on filesystems
# without reflink support (e.g. ext4 on GitHub Actions runners) instead
# of erroring -- a few seconds for ~107MB, acceptable for CI.
if [[ "$(uname -s)" == "Darwin" ]]; then
  cp -Rc "$SRC" "$DST"
else
  cp -R --reflink=auto "$SRC" "$DST"
fi
chmod -R u+w "$DST"

TARGET_DIR="$DST/pkg/scheduler/backend/cache/debugger"
cp "$OVERLAY_DIR/signal_notjs.go" "$TARGET_DIR/signal.go"
cp "$OVERLAY_DIR/signal_js.go" "$TARGET_DIR/signal_js.go"

# Same drift-check-then-swap treatment for the scheduler's in-tree plugin
# registry: the js build drops the DynamicResources plugin entry to fit
# the Worker Loader's hard 64MiB cap (see
# pkg/k8s-js-overlays/scheduler-registry_js.go's doc comment for
# the measured numbers); the !js build keeps upstream's registry
# byte-for-byte so cmd/scheduler and the conformance CI are unaffected.
UPSTREAM_REGISTRY="$SRC/pkg/scheduler/framework/plugins/registry.go"
EXPECTED_REG_SHA="$(awk '{print $1}' "$OVERLAY_DIR/upstream-scheduler-registry.go.sha256")"
ACTUAL_REG_SHA="$(shasum -a 256 "$UPSTREAM_REGISTRY" | awk '{print $1}')"
if [[ "$ACTUAL_REG_SHA" != "$EXPECTED_REG_SHA" ]]; then
  echo "gen-k8s-js-mirror: upstream scheduler plugins/registry.go changed since the overlay was last reviewed." >&2
  echo "  expected sha256 $EXPECTED_REG_SHA, got $ACTUAL_REG_SHA" >&2
  echo "  Diff $UPSTREAM_REGISTRY against $OVERLAY_DIR/scheduler-registry_notjs.go, update" >&2
  echo "  both scheduler-registry_*.go if the change matters, then refresh" >&2
  echo "  upstream-scheduler-registry.go.sha256. See docs/k8s-version-bump.md." >&2
  exit 1
fi
REG_DIR="$DST/pkg/scheduler/framework/plugins"
cp "$OVERLAY_DIR/scheduler-registry_notjs.go" "$REG_DIR/registry.go"
cp "$OVERLAY_DIR/scheduler-registry_js.go" "$REG_DIR/registry_js.go"

# Deterministic js-pair transforms for the KCM size budget (see
# docs/platform-verification.md's OPEN REGRESSION resolution): each
# upstream file is sha256-pinned, split into an untouched !js half and a
# js half with exactly one surgical change. Host builds (cmd/agent,
# cmd/controller-manager, conformance CI) are byte-for-byte unaffected.
#   - pkg/controller/controller_utils.go: drop the blank
#     `_ core/install` import on js (pure legacyscheme side effect;
#     nothing in the file references legacyscheme -- verified by grep).
#   - pkg/controller/nodeipam/{ipam/cidr_allocator,node_ipam_controller,
#     nolegacyprovider}.go: replace `cloudprovider.Interface` with
#     `interface{}` on js, severing k8s.io/cloud-provider -> aggregate
#     client-go informers -> all-groups typed clientset (this repo only
#     ever uses the RangeAllocator; callers pass nil).
check_pin() {
  local rel="$1" pin="$2"
  local actual
  actual="$(shasum -a 256 "$SRC/$rel" | awk '{print $1}')"
  local expected
  expected="$(awk '{print $1}' "$OVERLAY_DIR/$pin")"
  if [[ "$actual" != "$expected" ]]; then
    echo "gen-k8s-js-mirror: upstream $rel changed since its js-pair transform was last reviewed (expected $expected, got $actual). Re-review the transform below, then refresh $pin. See docs/k8s-version-bump.md." >&2
    exit 1
  fi
}
check_pin pkg/controller/controller_utils.go upstream-controller-utils.go.sha256
check_pin pkg/controller/nodeipam/ipam/cidr_allocator.go upstream-nodeipam-cidr-allocator.go.sha256
check_pin pkg/controller/nodeipam/node_ipam_controller.go upstream-nodeipam-controller.go.sha256
check_pin pkg/controller/nodeipam/nolegacyprovider.go upstream-nodeipam-nolegacyprovider.go.sha256

python3 - "$DST" <<'PYEOF'
import re, sys
dst = sys.argv[1]

def pair(rel, js_transform):
    p = f"{dst}/{rel}"
    src = open(p).read()
    open(p, "w").write("//go:build !js\n\n" + src)
    js = js_transform(src)
    assert js != src, f"transform was a no-op for {rel}"
    jsp = p[:-3] + "_js.go"
    open(jsp, "w").write("//go:build js\n\n" + js)

pair("pkg/controller/controller_utils.go",
     lambda s: s.replace('\t_ "k8s.io/kubernetes/pkg/apis/core/install"\n', ""))
for rel in ["pkg/controller/nodeipam/ipam/cidr_allocator.go",
            "pkg/controller/nodeipam/node_ipam_controller.go",
            "pkg/controller/nodeipam/nolegacyprovider.go"]:
    pair(rel, lambda s: s.replace('cloudprovider.Interface', 'interface{}')
                         .replace('\tcloudprovider "k8s.io/cloud-provider"\n', ''))
PYEOF

# queue/testing.go is a non-_test.go file (so it's part of the package's
# normal build) whose test-helper exports are confirmed unused by any
# non-test code in pkg/scheduler (grep) but import
# k8s.io/client-go/kubernetes/fake, which pkg/clientgo-lean-overlays' pruning breaks for the five groups it narrows. See
# pkg/k8s-js-overlays/queue_testing_stub.go's doc comment.
cp "$OVERLAY_DIR/queue_testing_stub.go" "$DST/pkg/scheduler/backend/queue/testing.go"

# Same treatment, same reasoning: pkg/controller/nodeipam/ipam/test/utils.go
# is a non-_test.go test-fixture file whose k8s.io/client-go/kubernetes/fake
# + aggregate k8s.io/client-go/informers imports break
# pkg/clientgo-lean-overlays' pruning, for zero functional benefit
# (confirmed unused by non-test code). See
# pkg/k8s-js-overlays/nodeipam_test_utils_stub.go's doc comment.
cp "$OVERLAY_DIR/nodeipam_test_utils_stub.go" "$DST/pkg/controller/nodeipam/ipam/test/utils.go"

echo "gen-k8s-js-mirror: done ($(find "$DST" -type f | wc -l | tr -d ' ') files at $DST)"
