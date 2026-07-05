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

go run github.com/syumai/workers/cmd/workers-assets-gen -mode=go -o workers/controllers/build
mkdir -p workers/controllers/assets
cp workers/controllers/build/wasm_exec.js workers/controllers/assets/wasm_exec.js

CAP=67108864 # the Loader's 64MiB total-module-bytes cap (S14 Part 2)

build_one() {
  local name="$1" pkg="$2"
  echo "== $name ($pkg)"
  GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath \
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

build_one kcm ./workers/controllers
build_one sched ./workers/controllers/scheduler
