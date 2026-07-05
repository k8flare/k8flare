#!/usr/bin/env bash
# Regenerates .build/k8s-js-mirror/: a full local copy of the upstream
# k3s-io/kubernetes module (the fork k8s.io/kubernetes resolves to, see
# go.mod) with exactly two files swapped for a GOOS=js-compatible
# pkg/scheduler/backend/cache/debugger/signal.go.
#
# Why this exists, and why go.mod's `k8s.io/kubernetes` replace points
# here instead of straight at github.com/k3s-io/kubernetes: see
# third_party/k8s-js-overlays/README.md. Short version: `go build
# -overlay` cannot patch files inside GOMODCACHE ("Files beneath
# GOMODCACHE must not be replaced" -- verified by actually trying), so
# the only way to change this one file's content is a module-level
# `replace` to a local directory. This script generates that directory
# without committing a multi-hundred-MB copy of k8s.io/kubernetes to git.
#
# go.mod's replace line depends on .build/k8s-js-mirror existing on disk
# -- run this after every fresh clone and after editing
# third_party/k8s-js-overlays/upstream-module.txt (k8s version bump).
# `npm run build:wasm` runs it automatically; run it by hand for host
# builds (cmd/agent, cmd/scheduler, cmd/controller-manager, go vet, etc.)
# if you haven't run a wasm build yet in a fresh checkout.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OVERLAY_DIR="$ROOT/third_party/k8s-js-overlays"
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

# queue/testing.go is a non-_test.go file (so it's part of the package's
# normal build) whose test-helper exports are confirmed unused by any
# non-test code in pkg/scheduler (grep) but import
# k8s.io/client-go/kubernetes/fake, which third_party/clientgo-lean-
# overlays' pruning breaks for the five groups it narrows. See
# third_party/k8s-js-overlays/queue_testing_stub.go's doc comment.
cp "$OVERLAY_DIR/queue_testing_stub.go" "$DST/pkg/scheduler/backend/queue/testing.go"

# Same treatment, same reasoning: pkg/controller/nodeipam/ipam/test/utils.go
# is a non-_test.go test-fixture file whose k8s.io/client-go/kubernetes/fake
# + aggregate k8s.io/client-go/informers imports break
# third_party/clientgo-lean-overlays' pruning, for zero functional benefit
# (confirmed unused by non-test code). See
# third_party/k8s-js-overlays/nodeipam_test_utils_stub.go's doc comment.
cp "$OVERLAY_DIR/nodeipam_test_utils_stub.go" "$DST/pkg/controller/nodeipam/ipam/test/utils.go"

echo "gen-k8s-js-mirror: done ($(find "$DST" -type f | wc -l | tr -d ' ') files at $DST)"
