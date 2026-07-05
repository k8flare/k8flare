#!/usr/bin/env bash
# Regenerates .build/clientgo-lean-mirror/: a full local copy of the
# upstream k3s-io/kubernetes-forked k8s.io/client-go module, with
# kubernetes/typed/<group>/<version>, applyconfigurations/<group>/<version>
# and kubernetes/clientset.go replaced by third_party/clientgo-lean-overlays/'s
# hand-curated, pruned versions.
#
# Why this exists, and what it prunes: see
# third_party/clientgo-lean-overlays/README.md. Short version: importing
# client-go's own generated PodInterface (etc.) -- required to satisfy
# upstream controller code's exact client parameter type -- costs ~44MiB
# of linked GOOS=js/wasm code per type, almost entirely from *unrelated*
# sibling files (other core/v1 types' generated code) in the same package
# being linked despite never being referenced. go build -overlay can't fix
# this (client-go resolves into GOMODCACHE; overlay refuses to touch it,
# same restriction as third_party/k8s-js-overlays/), so -- same lever as
# that sibling mirror -- this generates a pruned local copy and go.mod
# replaces k8s.io/client-go with it.
#
# go.mod's k8s.io/client-go replace line depends on this directory
# existing on disk -- same operational note as
# scripts/gen-k8s-js-mirror.sh: run this (or `npm run build:wasm`, which
# calls it) after every fresh clone before any Go build works.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OVERLAY_DIR="$ROOT/third_party/clientgo-lean-overlays"
DST="$ROOT/.build/clientgo-lean-mirror"

# Deliberately go.mod-replace-state-independent, same reasoning as
# gen-k8s-js-mirror.sh: `go list -m k8s.io/client-go` would resolve to
# this script's own previous output once go.mod's replace points here,
# making regeneration self-referential. Asking about the upstream fork
# module directly (kept in upstream-module.txt, independent of go.mod's
# replace target) sidesteps that.
UPSTREAM_MODULE="$(awk '{print $1}' "$OVERLAY_DIR/upstream-module.txt")"
UPSTREAM_VERSION="$(awk '{print $2}' "$OVERLAY_DIR/upstream-module.txt")"

echo "gen-clientgo-lean-mirror: resolving ${UPSTREAM_MODULE}@${UPSTREAM_VERSION}..."
SRC="$(go mod download -json "${UPSTREAM_MODULE}@${UPSTREAM_VERSION}" | jq -r '.Dir')"
if [[ -z "$SRC" || "$SRC" == "null" || ! -d "$SRC" ]]; then
  echo "gen-clientgo-lean-mirror: failed to resolve module dir for ${UPSTREAM_MODULE}@${UPSTREAM_VERSION}" >&2
  exit 1
fi

echo "gen-clientgo-lean-mirror: copying $SRC -> $DST..."
rm -rf "$DST"
mkdir -p "$(dirname "$DST")"
if [[ "$(uname -s)" == "Darwin" ]]; then
  cp -Rc "$SRC" "$DST"
else
  cp -R --reflink=auto "$SRC" "$DST"
fi
chmod -R u+w "$DST"

# kubernetes/typed/<group>/<version> and applyconfigurations/<group>/<version>
# are NOT pruned, deliberately -- see this repo's honest-correction note
# (docs/platform-verification.md, Phase 10) and
# third_party/clientgo-lean-overlays/README.md. Short version: pruning
# them broke the moment pkg/scheduler.New's informerFactory parameter
# (fixed to the real k8s.io/client-go/informers.SharedInformerFactory,
# which imports all ~54 groups' typed/applyconfigurations packages
# directly in its own factory.go) entered the same binary -- those
# packages cross-reference each other extensively (e.g. autoscaling's
# HorizontalPodAutoscalerApplyConfiguration references core's
# ObjectReferenceApplyConfiguration), so partial pruning cascades into
# compile errors across unrelated groups. The per-type overlay files
# (kubernetes/typed/<group>/<version>/*.go, applyconfigurations/<group>/
# <version>/*.go) are kept in third_party/clientgo-lean-overlays/ for the
# record and because pkg/leanclient/gen's generated clients still
# implement their (now merely redundant-with-upstream, not
# size-saving) interfaces, but this script no longer swaps them in.

cp "$OVERLAY_DIR/kubernetes/clientset.go" "$DST/kubernetes/clientset.go"

# kubernetes/scheme/register.go: THE size lever for the GOOS=js binaries.
# Upstream init()-registers all ~55 group-versions, and init side effects
# defeat dead-code elimination -- importing client-go/discovery (reached
# via applyconfigurations/meta/v1 from every typed client) dragged ~21MiB
# of generated API-type code into the wasm builds. The overlay registers
# only the group-versions the wasm control-plane binaries actually
# serialize; see its doc comment. Drift-checked like the k8s-js-mirror
# overlays: a client-go bump that changes upstream's register.go must be
# human-reviewed here.
UPSTREAM_SCHEME_REG="$SRC/kubernetes/scheme/register.go"
EXPECTED_SR_SHA="$(awk '{print $1}' "$OVERLAY_DIR/upstream-scheme-register.go.sha256")"
ACTUAL_SR_SHA="$(shasum -a 256 "$UPSTREAM_SCHEME_REG" | awk '{print $1}')"
if [[ "$ACTUAL_SR_SHA" != "$EXPECTED_SR_SHA" ]]; then
  echo "gen-clientgo-lean-mirror: upstream kubernetes/scheme/register.go changed since the overlay was last reviewed." >&2
  echo "  expected sha256 $EXPECTED_SR_SHA, got $ACTUAL_SR_SHA" >&2
  echo "  Review the new upstream file, update kubernetes/scheme/register.go in" >&2
  echo "  $OVERLAY_DIR if group-versions were added/renamed, then refresh" >&2
  echo "  upstream-scheme-register.go.sha256." >&2
  exit 1
fi
cp "$OVERLAY_DIR/kubernetes/scheme/register.go" "$DST/kubernetes/scheme/register.go"

# informers/<group>/<version> and listers/<group>/<version> are NOT
# pruned, deliberately -- see third_party/clientgo-lean-overlays/README.md's
# "correction" note. pkg/scheduler.New's informerFactory parameter is
# fixed to k8s.io/client-go/informers.SharedInformerFactory (an external
# interface spanning all ~54 groups); passing anything through that
# parameter requires the *real*, unpruned informers/listers tree (and,
# above, kubernetes.Interface's full width) regardless of how few groups
# pkg/controllers' own code touches directly.

echo "gen-clientgo-lean-mirror: done ($(find "$DST" -type f | wc -l | tr -d ' ') files at $DST)"
