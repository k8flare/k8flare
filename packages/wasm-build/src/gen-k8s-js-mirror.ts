#!/usr/bin/env node
// Regenerates .build/k8s-js-mirror/: a full local copy of the upstream
// k3s-io/kubernetes module (the fork k8s.io/kubernetes resolves to, see
// go.mod) with exactly two files swapped for a GOOS=js-compatible
// pkg/scheduler/backend/cache/debugger/signal.go.
//
// Why this exists, and why go.mod's `k8s.io/kubernetes` replace points
// here instead of straight at github.com/k3s-io/kubernetes: see
// pkg/k8s-js-overlays/README.md. Short version: `go build -overlay`
// cannot patch files inside GOMODCACHE ("Files beneath GOMODCACHE must
// not be replaced" -- verified by actually trying), so the only way to
// change this one file's content is a module-level `replace` to a local
// directory. This script generates that directory without committing a
// multi-hundred-MB copy of k8s.io/kubernetes to git.
//
// go.mod's replace line depends on .build/k8s-js-mirror existing on disk
// -- run this after every fresh clone and after editing
// pkg/k8s-js-overlays/upstream-module.txt (k8s version bump). `make wasm`
// runs it automatically; run it by hand for host builds (cmd/agent,
// cmd/scheduler, cmd/controller-manager, go vet, etc.) if you haven't run
// a wasm build yet in a fresh checkout.
import * as fs from "node:fs";
import * as path from "node:path";
import {
  checkPin,
  countFiles,
  readUpstreamModule,
  replaceMirrorDir,
  resolveModuleDir,
} from "./gomod.ts";

const ROOT = path.resolve(import.meta.dirname, "../../..");
const OVERLAY_DIR = path.join(ROOT, "pkg/k8s-js-overlays");
const DST = path.join(ROOT, ".build/k8s-js-mirror");

const { module: upstreamModule, version: upstreamVersion } = readUpstreamModule(
  path.join(OVERLAY_DIR, "upstream-module.txt"),
);

console.log(`gen-k8s-js-mirror: resolving ${upstreamModule}@${upstreamVersion}...`);
const src = resolveModuleDir(upstreamModule, upstreamVersion);

// Drift check: fail loudly if upstream signal.go changed since the
// overlay was last hand-reviewed, instead of silently re-patching a file
// that may have grown new content the overlay doesn't account for
// (CLAUDE.md rule: k8s bumps must be reviewed, not mechanically pinned).
checkPin(
  "gen-k8s-js-mirror",
  path.join(src, "pkg/scheduler/backend/cache/debugger/signal.go"),
  path.join(OVERLAY_DIR, "upstream-signal.go.sha256"),
  `Diff against ${OVERLAY_DIR}/signal_notjs.go, update signal_notjs.go/signal_js.go if the change matters for GOOS=js, then refresh upstream-signal.go.sha256.`,
);

console.log(`gen-k8s-js-mirror: copying ${src} -> ${DST}...`);
replaceMirrorDir(src, DST);

const debuggerDir = path.join(DST, "pkg/scheduler/backend/cache/debugger");
fs.copyFileSync(path.join(OVERLAY_DIR, "signal_notjs.go"), path.join(debuggerDir, "signal.go"));
fs.copyFileSync(path.join(OVERLAY_DIR, "signal_js.go"), path.join(debuggerDir, "signal_js.go"));

// Same drift-check-then-swap treatment for the scheduler's in-tree plugin
// registry: the js build drops the DynamicResources plugin entry to fit
// the Worker Loader's hard 64MiB cap (see
// pkg/k8s-js-overlays/scheduler-registry_js.go's doc comment for the
// measured numbers); the !js build keeps upstream's registry byte-for-
// byte so cmd/scheduler and the conformance CI are unaffected.
checkPin(
  "gen-k8s-js-mirror",
  path.join(src, "pkg/scheduler/framework/plugins/registry.go"),
  path.join(OVERLAY_DIR, "upstream-scheduler-registry.go.sha256"),
  `Diff against ${OVERLAY_DIR}/scheduler-registry_notjs.go, update both scheduler-registry_*.go if the change matters, then refresh upstream-scheduler-registry.go.sha256.`,
);
const registryDir = path.join(DST, "pkg/scheduler/framework/plugins");
fs.copyFileSync(
  path.join(OVERLAY_DIR, "scheduler-registry_notjs.go"),
  path.join(registryDir, "registry.go"),
);
fs.copyFileSync(
  path.join(OVERLAY_DIR, "scheduler-registry_js.go"),
  path.join(registryDir, "registry_js.go"),
);

// Deterministic js-pair transforms for the KCM size budget (see
// docs/platform-verification.md's OPEN REGRESSION resolution): each
// upstream file is sha256-pinned, split into an untouched !js half and a
// js half with exactly one surgical change. Host builds (cmd/agent,
// cmd/controller-manager, conformance CI) are byte-for-byte unaffected.
//   - pkg/controller/controller_utils.go: drop the blank
//     `_ core/install` import on js (pure legacyscheme side effect;
//     nothing in the file references legacyscheme -- verified by grep).
//   - pkg/controller/nodeipam/{ipam/cidr_allocator,node_ipam_controller,
//     nolegacyprovider}.go: replace `cloudprovider.Interface` with
//     `interface{}` on js, severing k8s.io/cloud-provider -> aggregate
//     client-go informers -> all-groups typed clientset (this repo only
//     ever uses the RangeAllocator; callers pass nil).
function checkPinFile(rel: string, pin: string): void {
  checkPin(
    "gen-k8s-js-mirror",
    path.join(src, rel),
    path.join(OVERLAY_DIR, pin),
    `Re-review the js-pair transform below for ${rel}, then refresh ${pin}.`,
  );
}
checkPinFile("pkg/controller/controller_utils.go", "upstream-controller-utils.go.sha256");
checkPinFile(
  "pkg/controller/nodeipam/ipam/cidr_allocator.go",
  "upstream-nodeipam-cidr-allocator.go.sha256",
);
checkPinFile(
  "pkg/controller/nodeipam/node_ipam_controller.go",
  "upstream-nodeipam-controller.go.sha256",
);
checkPinFile(
  "pkg/controller/nodeipam/nolegacyprovider.go",
  "upstream-nodeipam-nolegacyprovider.go.sha256",
);

/** Splits DST/rel into an untouched !js half and a transformed js half. */
function pair(rel: string, jsTransform: (s: string) => string): void {
  const p = path.join(DST, rel);
  const original = fs.readFileSync(p, "utf8");
  fs.writeFileSync(p, "//go:build !js\n\n" + original);
  const transformed = jsTransform(original);
  if (transformed === original) {
    throw new Error(`gen-k8s-js-mirror: transform was a no-op for ${rel}`);
  }
  const jsPath = p.slice(0, -".go".length) + "_js.go";
  fs.writeFileSync(jsPath, "//go:build js\n\n" + transformed);
}

pair("pkg/controller/controller_utils.go", (s) =>
  s.replaceAll('\t_ "k8s.io/kubernetes/pkg/apis/core/install"\n', ""),
);
for (const rel of [
  "pkg/controller/nodeipam/ipam/cidr_allocator.go",
  "pkg/controller/nodeipam/node_ipam_controller.go",
  "pkg/controller/nodeipam/nolegacyprovider.go",
]) {
  pair(rel, (s) =>
    s
      .replaceAll("cloudprovider.Interface", "interface{}")
      .replaceAll('\tcloudprovider "k8s.io/cloud-provider"\n', ""),
  );
}

// queue/testing.go is a non-_test.go file (so it's part of the package's
// normal build) whose test-helper exports are confirmed unused by any
// non-test code in pkg/scheduler (grep) but import
// k8s.io/client-go/kubernetes/fake, which pkg/clientgo-lean-overlays'
// pruning breaks for the five groups it narrows. See
// pkg/k8s-js-overlays/queue_testing_stub.go's doc comment.
fs.copyFileSync(
  path.join(OVERLAY_DIR, "queue_testing_stub.go"),
  path.join(DST, "pkg/scheduler/backend/queue/testing.go"),
);

// Same treatment, same reasoning: pkg/controller/nodeipam/ipam/test/utils.go
// is a non-_test.go test-fixture file whose k8s.io/client-go/kubernetes/fake
// + aggregate k8s.io/client-go/informers imports break
// pkg/clientgo-lean-overlays' pruning, for zero functional benefit
// (confirmed unused by non-test code). See
// pkg/k8s-js-overlays/nodeipam_test_utils_stub.go's doc comment.
fs.copyFileSync(
  path.join(OVERLAY_DIR, "nodeipam_test_utils_stub.go"),
  path.join(DST, "pkg/controller/nodeipam/ipam/test/utils.go"),
);

console.log(`gen-k8s-js-mirror: done (${countFiles(DST)} files at ${DST})`);
