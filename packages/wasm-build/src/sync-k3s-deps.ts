#!/usr/bin/env node
// Syncs this repo's dependency pins to a given k3s release tag, e.g.
//
//   node packages/wasm-build/src/sync-k3s-deps.ts v1.36.3+k3s1
//
// The rule it encodes (see docs/k8s-version-bump.md): this repo does not
// choose its own k8s.io/etcd/spegel/... versions -- it mirrors whatever
// the k3s release itself pins, so `cmd/agent` embeds the exact same
// patched build k3s ships. Concretely, for every `replace` directive in
// go.mod / go.wasm.mod whose replaced module also has a `replace` in the
// k3s release's own go.mod, this script adopts k3s's replacement
// verbatim. Replaces pointing at local ./.build mirrors are left alone
// (their CONTENT is regenerated separately by `make gen-mirrors`, which
// reads the pkg/*-overlays/upstream-module.txt pins this script also
// updates -- and its sha256 drift gates still apply: if an upstream file
// an overlay patches changed, gen-mirrors fails loudly for human review;
// this script deliberately cannot silence that).
//
// What this script does NOT do: run `go get`/`go mod tidy` (the caller
// does -- see .github/workflows/deps-k3s-update.yml), regenerate
// mirrors, or touch anything version-independent.
import * as fs from "node:fs";
import * as path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "../../..");

const tag = process.argv[2];
if (!tag || !/^v\d+\.\d+\.\d+\+k3s\d+$/.test(tag)) {
  console.error("usage: sync-k3s-deps.ts <k3s release tag, e.g. v1.36.3+k3s1>");
  process.exit(2);
}

const upstreamGoModURL = `https://raw.githubusercontent.com/k3s-io/k3s/${encodeURIComponent(tag)}/go.mod`;
const res = await fetch(upstreamGoModURL);
if (!res.ok) {
  throw new Error(`fetching ${upstreamGoModURL}: HTTP ${res.status}`);
}
const upstreamGoMod = await res.text();

/**
 * Parses `replace` directives out of a go.mod text into a map of
 * replaced-module -> "replacement version" (the full right-hand side).
 * Handles both the block form and single-line form; ignores directives
 * whose right-hand side is a local path (no version token).
 */
function parseReplaces(gomod: string): Map<string, string> {
  const out = new Map<string, string>();
  const re = /^\s*(?:replace\s+)?([^\s=]+)\s*=>\s*(\S+)\s+(\S+)\s*(?:\/\/.*)?$/;
  let inBlock = false;
  for (const line of gomod.split("\n")) {
    const t = line.trim();
    if (t.startsWith("replace (")) {
      inBlock = true;
      continue;
    }
    if (inBlock && t === ")") {
      inBlock = false;
      continue;
    }
    if (!inBlock && !t.startsWith("replace ")) continue;
    const m = re.exec(line);
    if (m && !m[2].startsWith("./") && !m[2].startsWith("../")) {
      out.set(m[1], `${m[2]} ${m[3]}`);
    }
  }
  return out;
}

const upstream = parseReplaces(upstreamGoMod);
if (!upstream.has("k8s.io/kubernetes")) {
  throw new Error(`no k8s.io/kubernetes replace found in k3s ${tag} go.mod -- refusing to sync`);
}

let changes = 0;
function rewrite(file: string, transform: (s: string) => string): void {
  const p = path.join(ROOT, file);
  const before = fs.readFileSync(p, "utf8");
  const after = transform(before);
  if (after !== before) {
    fs.writeFileSync(p, after);
    changes++;
    console.log(`sync-k3s-deps: updated ${file}`);
  }
}

// go.mod / go.wasm.mod: adopt upstream's replacement for every module
// upstream also replaces. Local ./.build mirror targets are skipped.
for (const modfile of ["go.mod", "go.wasm.mod"]) {
  rewrite(modfile, (s) =>
    s
      .split("\n")
      .map((line) => {
        const m = /^(\t([^\s=]+) => )(\S+) (\S+)$/.exec(line);
        if (!m || m[3].startsWith("./") || m[3].startsWith("../")) return line;
        const want = upstream.get(m[2]);
        return want ? `${m[1]}${want}` : line;
      })
      .join("\n"),
  );
}

// Overlay source pins: the module@version gen-mirrors copies before
// patching. Derived from the same upstream replace set.
rewrite("pkg/k8s-js-overlays/upstream-module.txt", () => upstream.get("k8s.io/kubernetes")! + "\n");
rewrite("pkg/clientgo-lean-overlays/upstream-module.txt", (s) => {
  const clientgo = upstream.get("k8s.io/client-go");
  return clientgo ? clientgo + "\n" : s;
});

// (e2e-conformance.yml needs no edit: its e2e.test download derives the
// version from pkg/k8s-js-overlays/upstream-module.txt at run time.)
const k8sVersion = upstream
  .get("k8s.io/kubernetes")!
  .split(" ")[1]
  .replace(/-k3s\d+$/, "");

console.log(
  changes === 0
    ? `sync-k3s-deps: already in sync with ${tag}`
    : `sync-k3s-deps: ${changes} file(s) updated to ${tag} (k8s ${k8sVersion})`,
);
