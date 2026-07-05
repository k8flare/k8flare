#!/usr/bin/env bash
# Builds workers/controllers' two control-plane WASM binaries for the
# ASSETS+LOADER deploy path (workers/controllers/src/index.ts):
#
#   kcm    <- ./workers/controllers            (real kube-controller-manager)
#   sched  <- ./workers/controllers/scheduler  (real kube-scheduler)
#
# They are two separate dynamic workers because the combined binary
# (71.5MB after wasm-opt) exceeds the Worker Loader's hard 64MiB
# total-module-bytes cap, while each fits on its own -- see
# workers/controllers/scheduler/main.go's doc comment and
# docs/platform-verification.md's S14 section for the measurements.
#
# Per binary:
#   1. GOOS=js GOARCH=wasm go build with -ldflags="-s -w" -trimpath
#   2. wasm-opt -Oz -- REQUIRED, not an optimization nicety: the Loader
#      enforces the 64MiB cap (spikes/s14-loader-external-fetch Part 2)
#      and the unoptimized binaries (~76-80MB) exceed it. The four
#      --enable flags are required because Go's wasm output uses those
#      features and wasm-opt rejects them otherwise (S14 Part 4).
#      Takes ~2 minutes per binary.
#   3. Chunk into workers/controllers/assets/ (Static Assets have a 25MiB
#      per-file cap) plus a sha256 manifest.
#
# Also regenerates .build/k8s-js-mirror first (go.mod's k8s.io/kubernetes
# replace target; the scheduler binary additionally depends on its
# scheduler-registry overlay -- see third_party/k8s-js-overlays/README.md).
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v wasm-opt >/dev/null 2>&1; then
  echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2
  exit 1
fi

bash scripts/gen-k8s-js-mirror.sh
bash scripts/gen-clientgo-lean-mirror.sh

go run github.com/syumai/workers/cmd/workers-assets-gen -mode=go -o workers/controllers/build
mkdir -p workers/controllers/assets
cp workers/controllers/build/wasm_exec.js workers/controllers/assets/wasm_exec.js

CAP=67108864 # the Loader's 64MiB total-module-bytes cap (S14 Part 2)

build_one() {
  local name="$1" pkg="$2"
  shift 2
  echo "== $name ($pkg)"
  GOOS=js GOARCH=wasm go build "$@" -ldflags="-s -w" -trimpath \
    -o "workers/controllers/build/$name.wasm" "$pkg"
  wasm-opt -Oz \
    --strip-debug --strip-producers \
    --enable-bulk-memory --enable-nontrapping-float-to-int \
    --enable-sign-ext --enable-mutable-globals \
    "workers/controllers/build/$name.wasm" -o "workers/controllers/build/$name.opt.wasm"

  local raw
  raw=$(wc -c < "workers/controllers/build/$name.opt.wasm" | tr -d ' ')
  if [ "$raw" -ge "$CAP" ]; then
    echo "::error::$name.opt.wasm ($raw bytes) exceeds the Worker Loader's 64MiB cap ($CAP bytes) -- the dynamic worker cannot load. Trim dependencies (see docs/platform-verification.md S14)." >&2
    exit 1
  fi
  echo "$name.opt.wasm: $raw bytes ($(( (CAP - raw) / 1024 ))KiB headroom under the 64MiB Loader cap)"

  node scripts/chunk-wasm.mjs "workers/controllers/build/$name.opt.wasm" workers/controllers/assets "$name"
}

# KCM: built against go.wasm.mod (k8s.io/client-go -> width-pruned
# .build/clientgo-lean-mirror) with -tags leanwidth (narrow
# kubernetes.Interface + narrow leanclient stubs). Interface WIDTH is
# what keeps this binary under the Loader cap -- full width measured
# 98.6MB opt vs 66.1MB narrow (docs/platform-verification.md).
GOFLAGS=-modfile=go.wasm.mod build_one kcm ./workers/controllers -tags leanwidth
# scheduler: NOT built (workers/controllers/src/index.ts tolerates the
# missing sched manifest). Its informer factory is the full-width
# client-go SharedInformerFactory, so the leanwidth trick above does not
# apply, and against the reproducible mirrors the binary measures
# 102.8MB opt -- far over the Loader cap. The earlier 66.4MB figure was
# an artifact of the same unreproducible mirror state as the KCM
# regression (docs/platform-verification.md). kube-scheduler therefore
# remains host-process/BYO-VM (cmd/scheduler) until it gets its own
# width answer; workers/controllers/scheduler/main.go is kept as the
# ready entrypoint for that day. Re-enable with:
#   build_one sched ./workers/controllers/scheduler
