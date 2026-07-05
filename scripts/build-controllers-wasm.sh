#!/usr/bin/env bash
# Builds workers/controllers' kube-controller-manager WASM for the
# ASSETS+LOADER deploy path (workers/controllers/src/index.ts):
#
#   1. GOOS=js GOARCH=wasm go build with -ldflags="-s -w"
#   2. wasm-opt -Oz -- REQUIRED, not an optimization nicety: the Worker
#      Loader enforces a hard 64MiB cap on total module bytes
#      (spikes/s14-loader-external-fetch/FINDINGS.md Part 2). The stripped
#      but unoptimized binary (~76MB) exceeds it; the optimized one
#      (~60MiB) fits. The four --enable flags are required because Go's
#      wasm output uses those features and wasm-opt rejects them otherwise
#      (same flag set verified in S14 Part 4). Takes ~2 minutes.
#   3. Chunk into workers/controllers/assets/ (Static Assets have a 25MiB
#      per-file cap) plus a sha256 manifest, and copy the syumai/workers
#      wasm_exec.js alongside -- everything the Controllers DO needs to
#      assemble and LOADER.get() the dynamic worker at runtime.
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v wasm-opt >/dev/null 2>&1; then
  echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2
  exit 1
fi

go run github.com/syumai/workers/cmd/workers-assets-gen -mode=go -o workers/controllers/build
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o workers/controllers/build/app.wasm ./workers/controllers
wasm-opt -Oz \
  --enable-bulk-memory --enable-nontrapping-float-to-int \
  --enable-sign-ext --enable-mutable-globals \
  workers/controllers/build/app.wasm -o workers/controllers/build/app.opt.wasm

RAW=$(wc -c < workers/controllers/build/app.opt.wasm | tr -d ' ')
CAP=67108864 # the Loader's 64MiB total-module-bytes cap (S14 Part 2)
if [ "$RAW" -ge "$CAP" ]; then
  echo "::error::app.opt.wasm ($RAW bytes) exceeds the Worker Loader's 64MiB cap ($CAP bytes) -- the dynamic worker cannot load. Trim controllers or dependencies." >&2
  exit 1
fi
echo "app.opt.wasm: $RAW bytes ($(( (CAP - RAW) / 1024 / 1024 ))MiB headroom under the 64MiB Loader cap)"

node scripts/chunk-wasm.mjs workers/controllers/build/app.opt.wasm workers/controllers/assets kcm
cp workers/controllers/build/wasm_exec.js workers/controllers/assets/wasm_exec.js
