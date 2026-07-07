#!/usr/bin/env bash
# Builds the control-plane Go WASM binaries that run as Loader dynamic
# workers inside the consolidated k8flare Worker (workers/k8flare), and
# ships them as Static Assets chunks under workers/k8flare/assets/wasm/:
#
#   apiserver <- ./cmd/apiserver-wasm   (pkg/apiserver, per-request shape)
#   kcm       <- ./cmd/kcm-wasm         (real kube-controller-manager, resident)
#   sched     <- ./cmd/kcm-wasm/scheduler (NOT built -- see bottom)
#
# Generalizes the former scripts/build-controllers-wasm.sh:
#   - kcm needs go.wasm.mod (width-pruned client-go mirror), -tags
#     leanwidth, and wasm-opt -Oz (REQUIRED: unoptimized it exceeds the
#     Loader's hard 64MiB total-module-bytes cap, S14 Part 2; ~2 min).
#   - apiserver builds from the normal go.mod and needs NO wasm-opt
#     (43MB raw is comfortably under the cap); wasm-opt stays available
#     as an optional cold-start optimization if ever needed.
# Both are gated on the 64MiB cap. Chunks are ≤24MiB (ASSETS per-file
# cap is 25MiB) plus a sha256 manifest (the Loader cache id, so a
# rebuild busts the isolate cache naturally).
#
# Also regenerates the .build/ mirrors first (go.mod's k8s.io/kubernetes
# replace target etc.). NOTE the CLAUDE.md mirror-regeneration caveat:
# gen-*.sh scripts own their own drift-check discipline; do not add
# rm -rf of mirror state here.
set -euo pipefail
cd "$(dirname "$0")/.."

ASSETS=workers/k8flare/assets/wasm
BUILD=.build/wasm
CAP=67108864 # the Loader's 64MiB total-module-bytes cap (S14 Part 2)

if ! command -v wasm-opt >/dev/null 2>&1; then
  echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2
  exit 1
fi

bash scripts/gen-k8s-js-mirror.sh
bash scripts/gen-clientgo-lean-mirror.sh

mkdir -p "$ASSETS" "$BUILD"
# syumai/workers' wasm_exec.js variant (globalThis.Go with runtimeCtx
# support), shared by every dynamic worker bootstrap.
go run github.com/syumai/workers/cmd/workers-assets-gen -mode=go -o "$BUILD/assets-gen"
cp "$BUILD/assets-gen/wasm_exec.js" "$ASSETS/wasm_exec.js"

# build_one <name> <pkg> <opt:yes|no> [extra go build args...]
build_one() {
  local name="$1" pkg="$2" opt="$3"
  shift 3
  echo "== $name ($pkg)"
  GOOS=js GOARCH=wasm go build "$@" -ldflags="-s -w" -trimpath \
    -o "$BUILD/$name.wasm" "$pkg"

  local final="$BUILD/$name.wasm"
  if [ "$opt" = "yes" ]; then
    wasm-opt -Oz \
      --strip-debug --strip-producers \
      --enable-bulk-memory --enable-nontrapping-float-to-int \
      --enable-sign-ext --enable-mutable-globals \
      "$BUILD/$name.wasm" -o "$BUILD/$name.opt.wasm"
    final="$BUILD/$name.opt.wasm"
  fi

  local raw
  raw=$(wc -c < "$final" | tr -d ' ')
  if [ "$raw" -ge "$CAP" ]; then
    echo "::error::$name ($raw bytes) exceeds the Worker Loader's 64MiB cap ($CAP bytes) -- the dynamic worker cannot load. Trim dependencies (see docs/platform-verification.md S14)." >&2
    exit 1
  fi
  echo "$name: $raw bytes ($(( (CAP - raw) / 1024 ))KiB headroom under the 64MiB Loader cap)"

  node scripts/chunk-wasm.mjs "$final" "$ASSETS" "$name"
}

# Optional positional args select components (default: all) -- e.g.
# `bash scripts/build-wasm-chunks.sh apiserver` skips the ~2min KCM
# wasm-opt pass for jobs that only need the apiserver (smoke-nodes CI).
want() {
  [ "$#" -eq 0 ] && return 0
  local c
  for c in "$@"; do [ "$c" = "$WANTED" ] && return 0; done
  return 1
}

# apiserver: normal go.mod, no wasm-opt needed.
WANTED=apiserver
if want "$@"; then
  build_one apiserver ./cmd/apiserver-wasm no
fi

# KCM: built against go.wasm.mod (k8s.io/client-go -> width-pruned
# .build/clientgo-lean-mirror) with -tags leanwidth (narrow
# kubernetes.Interface + narrow leanclient stubs). Interface WIDTH is
# what keeps this binary under the Loader cap -- full width measured
# 98.6MB opt vs 66.1MB narrow (docs/platform-verification.md).
WANTED=kcm
if want "$@"; then
  GOFLAGS=-modfile=go.wasm.mod build_one kcm ./cmd/kcm-wasm yes -tags leanwidth
fi
# sched: NOT built (the Controllers DO tolerates the missing manifest).
# kube-scheduler measures 101.1MB opt against the reproducible mirrors
# (2026-07-07: narrowing informers.SharedInformerFactory from 19 to 6
# groups closed ~1.7MB of the prior 102.8MB figure; still far over the
# Loader cap -- see docs/platform-verification.md's S8
# kube-scheduler-wasm-fork entry for why kubernetes.Interface's global
# width, not the informers aggregate, is the dominant remaining cost,
# and the scoped follow-up needed to close it). It remains
# host-process/BYO-VM (cmd/scheduler). cmd/kcm-wasm/scheduler stays as
# the ready entrypoint for the day it gets its own width answer.
# Re-enable with:
#   build_one sched ./cmd/kcm-wasm/scheduler yes
