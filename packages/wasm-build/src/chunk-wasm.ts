#!/usr/bin/env node
// Splits a WASM binary into Static-Assets-safe chunks (Cloudflare's ASSETS
// per-file cap is 25MiB) and writes a manifest consumed at Loader-factory
// time by workers/k8flare/src/loader/*.ts. The manifest's sha256 doubles
// as the Loader isolate cache key, so a rebuilt binary naturally busts the
// dynamic worker cache on next load.
import { createHash } from "node:crypto";
import * as fs from "node:fs";
import * as path from "node:path";

const [input, outDir, name] = process.argv.slice(2);
if (!input || !outDir || !name) {
  console.error("usage: chunk-wasm.ts <input.wasm> <out-dir> <name>");
  process.exit(1);
}

const CHUNK_BYTES = 24 * 1024 * 1024; // 24MiB, under the 25MiB ASSETS per-file cap

const bytes = fs.readFileSync(input);
const sha256 = createHash("sha256").update(bytes).digest("hex");
fs.mkdirSync(outDir, { recursive: true });

// Drop stale chunks from a previous (possibly larger) build so the assets
// dir never mixes parts of two different binaries.
for (const f of fs.readdirSync(outDir)) {
  if (f.startsWith(`${name}.wasm.part`)) fs.unlinkSync(path.join(outDir, f));
}

const parts: string[] = [];
for (let off = 0, i = 0; off < bytes.length; off += CHUNK_BYTES, i++) {
  const part = `${name}.wasm.part${i}`;
  fs.writeFileSync(path.join(outDir, part), bytes.subarray(off, off + CHUNK_BYTES));
  parts.push(part);
}

fs.writeFileSync(
  path.join(outDir, `${name}.manifest.json`),
  JSON.stringify({ size: bytes.length, sha256, parts }, null, 2) + "\n",
);
console.log(
  `chunk-wasm: ${input} (${bytes.length} bytes, sha256 ${sha256.slice(0, 16)}…) -> ${parts.length} parts in ${outDir}`,
);
