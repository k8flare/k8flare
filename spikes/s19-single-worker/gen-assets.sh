#!/usr/bin/env bash
# Generates this spike's assets/ from the already-built apiserver WASM
# (run `npm run build:wasm:apiserver` at repo root first): ≤24MiB chunks +
# manifest, mirroring scripts/build-controllers-wasm.sh's output shape.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$ROOT/workers/apiserver/build/app.wasm"
DST="$HERE/assets"

[[ -f "$SRC" ]] || { echo "missing $SRC -- run npm run build:wasm:apiserver first" >&2; exit 1; }

rm -rf "$DST"
mkdir -p "$DST"
cp "$ROOT/workers/apiserver/build/wasm_exec.js" "$DST/"

CHUNK=$((24 * 1024 * 1024))
split -b "$CHUNK" "$SRC" "$DST/apiserver.wasm.part."
i=0
parts=()
for f in "$DST"/apiserver.wasm.part.*; do
  mv "$f" "$DST/apiserver.wasm.part$i"
  parts+=("\"apiserver.wasm.part$i\"")
  i=$((i + 1))
done

SIZE=$(stat -f%z "$SRC" 2>/dev/null || stat -c%s "$SRC")
SHA=$(shasum -a 256 "$SRC" | awk '{print $1}')
printf '{"sha256":"%s","size":%s,"parts":[%s]}\n' "$SHA" "$SIZE" "$(IFS=,; echo "${parts[*]}")" \
  > "$DST/apiserver.manifest.json"

printf '*\n!.gitignore\n' > "$DST/.gitignore"
echo "s19 assets: $i parts, $SIZE bytes, sha256=$SHA"
