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

// ---- DRA/CEL severing for the scheduler wasm build (task #4, 2026-07-10) --
// The scheduler binary's remaining size anchor past schedwidth is the
// DRA/CEL chain (~15MB code, measured by wasm name-section attribution:
// cel-go 4.3MB + protobuf/genproto/grpc 4.2MB + gnostic 2.5MB + antlr
// 0.66MB + apiserver/pkg/cel 0.71MB + dynamic-resource-allocation 0.75MB
// + resource_v1/v1beta2 types 1.9MB). Only two DRA-library subpackages
// anchor it: k8s.io/dynamic-resource-allocation/{structured,cel} (the
// resourceclaim / resourceslice/tracker / extendedresourcecache
// subpackages are apimachinery-only -- verified by reading their
// imports -- so scheduler.go/eventhandlers.go's tracker wiring stays
// untouched). The js halves below sever exactly:
//   - scheduler.go: the plugins/dynamicresources import + the one
//     NewDRAManager call (draManager stays nil; the DRA plugin is
//     already absent from registry_js.go, so nothing consumes it).
//   - schedule_one.go: the plugins/dynamicresources import + the one
//     ExtractPodNodeAllocatableResourceClaimStatus call (-> nil).
//   - noderesources/{fit,resource_allocation,balanced_allocation}.go:
//     the structured/cel imports are re-pointed at the drastub/celstub
//     packages written below. drastub ALIASES the cheap real types
//     (schedulerapi.AllocatedState/DeviceID, internal.Features -- both
//     packages are apimachinery-only) so values produced by
//     fwk.SharedDRAManager's interface methods still type-check; only
//     the cel-anchored FUNCTIONS (NodeMatches, IsDeviceAllocated, the
//     CEL cache) are stubbed. All stub paths are gated at runtime on
//     EnableDRAExtendedResource + actual DeviceClass objects, which
//     this apiserver serves only as permanently-empty stub types.
checkPinFile("pkg/scheduler/scheduler.go", "upstream-sched-scheduler.go.sha256");
checkPinFile("pkg/scheduler/schedule_one.go", "upstream-sched-schedule-one.go.sha256");
checkPinFile("pkg/scheduler/framework/plugins/noderesources/fit.go", "upstream-nr-fit.go.sha256");
checkPinFile(
  "pkg/scheduler/framework/plugins/noderesources/resource_allocation.go",
  "upstream-nr-resource-allocation.go.sha256",
);
checkPinFile(
  "pkg/scheduler/framework/plugins/noderesources/balanced_allocation.go",
  "upstream-nr-balanced-allocation.go.sha256",
);

const NR = "pkg/scheduler/framework/plugins/noderesources";
const DRASTUB_IMPORT = `structured "k8s.io/kubernetes/${NR}/drastub"`;
const CELSTUB_IMPORT = `cel "k8s.io/kubernetes/${NR}/celstub"`;

fs.mkdirSync(path.join(DST, NR, "drastub"), { recursive: true });
fs.writeFileSync(
  path.join(DST, NR, "drastub", "drastub.go"),
  `//go:build js

// Package drastub is k8flare's js-only stand-in for
// k8s.io/dynamic-resource-allocation/structured, generated into this
// mirror by gen-k8s-js-mirror.ts (see the DRA/CEL severing comment
// there). Types are ALIASES to the real, apimachinery-only
// schedulerapi/internal packages so fwk.SharedDRAManager's interface
// methods still type-check; only the CEL-anchored functions are
// stubbed (their call sites are unreachable when no DRA objects exist,
// which is always true against k8flare's apiserver).
package drastub

import (
	v1 "k8s.io/api/core/v1"
	schedulerapi "k8s.io/dynamic-resource-allocation/structured/schedulerapi"
)

// Features is a concrete empty struct, NOT an alias to the real
// structured/internal.Features: that package is Go-internal to the DRA
// module and cannot be imported from here, and no code outside the
// noderesources js halves produces or consumes this type (the one real
// producer, dynamicresources.AllocatorFeatures, is replaced with a
// zero literal by fit.go's js transform).
type Features struct{}

type DeviceID = schedulerapi.DeviceID
type AllocatedState = schedulerapi.AllocatedState

func MakeDeviceID(driver, pool, device string) DeviceID {
	return schedulerapi.MakeDeviceID(driver, pool, device)
}

// NodeMatches: no ResourceSlice can ever match -- k8flare serves DRA
// types as permanently-empty stubs, so this is unreachable in practice.
func NodeMatches(_ Features, _ *v1.Node, _ string, _ bool, _ *v1.NodeSelector) (bool, error) {
	return false, nil
}

func IsDeviceAllocated(_ DeviceID, _ *AllocatedState) bool { return false }
`,
);
fs.mkdirSync(path.join(DST, NR, "celstub"), { recursive: true });
fs.writeFileSync(
  path.join(DST, NR, "celstub", "celstub.go"),
  `//go:build js

// Package celstub is k8flare's js-only stand-in for
// k8s.io/dynamic-resource-allocation/cel (the cel-go anchor), generated
// into this mirror by gen-k8s-js-mirror.ts -- see the DRA/CEL severing
// comment there. GetOrCompile/DeviceMatches report "no match" instead
// of evaluating: reachable only for a DeviceClass with CEL selectors,
// which cannot exist against k8flare's stub DRA types.
package celstub

import (
	"context"

	resourceapi "k8s.io/api/resource/v1"
)

type Features struct {
	EnableConsumableCapacity bool
	EnableListTypeAttributes bool
}

type Cache struct{}

func NewCache(_ int, _ Features) *Cache { return &Cache{} }

type CompilationResult struct {
	Error error
}

func (c *Cache) GetOrCompile(_ string) CompilationResult { return CompilationResult{} }

type Device struct {
	Driver     string
	Attributes map[resourceapi.QualifiedName]resourceapi.DeviceAttribute
	Capacity   map[resourceapi.QualifiedName]resourceapi.DeviceCapacity
}

func (r CompilationResult) DeviceMatches(_ context.Context, _ Device) (bool, any, error) {
	return false, nil, nil
}
`,
);

pair("pkg/scheduler/scheduler.go", (s) =>
  s
    .replaceAll('\t"k8s.io/kubernetes/pkg/scheduler/framework/plugins/dynamicresources"\n', "")
    .replaceAll(
      "\t\tdraManager = dynamicresources.NewDRAManager(ctx, resourceClaimCache, resourceSliceTracker, informerFactory)\n",
      "\t\t// k8flare js: DRA manager severed (gen-k8s-js-mirror.ts); draManager stays nil.\n",
    ),
);
pair("pkg/scheduler/schedule_one.go", (s) =>
  s
    .replaceAll('\t"k8s.io/kubernetes/pkg/scheduler/framework/plugins/dynamicresources"\n', "")
    .replaceAll(
      "= dynamicresources.ExtractPodNodeAllocatableResourceClaimStatus(logger, state, host)",
      "= nil // k8flare js: DRA severed (gen-k8s-js-mirror.ts)",
    ),
);
pair(`${NR}/fit.go`, (s) =>
  s
    .replaceAll('\t"k8s.io/dynamic-resource-allocation/cel"\n', `\t${CELSTUB_IMPORT}\n`)
    .replaceAll(
      '\t"k8s.io/kubernetes/pkg/scheduler/framework/plugins/dynamicresources"\n',
      `\t${DRASTUB_IMPORT}\n`,
    )
    .replaceAll("dynamicresources.AllocatorFeatures(fts)", "structured.Features{}"),
);
pair(`${NR}/resource_allocation.go`, (s) =>
  s
    .replaceAll('\t"k8s.io/dynamic-resource-allocation/cel"\n', `\t${CELSTUB_IMPORT}\n`)
    .replaceAll('\t"k8s.io/dynamic-resource-allocation/structured"\n', `\t${DRASTUB_IMPORT}\n`),
);
pair(`${NR}/balanced_allocation.go`, (s) =>
  s.replaceAll('\t"k8s.io/dynamic-resource-allocation/structured"\n', `\t${DRASTUB_IMPORT}\n`),
);

// pkg/kubelet/types/types.go imports k8s.io/cri-client/pkg/logs for TWO
// time-format string constants -- and cri-client/pkg links grpc +
// k8s.io/cri-api + otelgrpc into every binary that transitively touches
// pkg/apis/core/validation -> pkg/capabilities -> pkg/kubelet/types
// (which includes the scheduler; found via go list -deps tracing during
// task #4's size hunt, 2026-07-10). The js half inlines the two literal
// values (pinned above via the file's own sha256; the cri-client values
// are stable RFC3339 layouts).
checkPinFile("pkg/kubelet/types/types.go", "upstream-kubelet-types.go.sha256");
pair("pkg/kubelet/types/types.go", (s) =>
  s
    .replaceAll(
      '\t"k8s.io/cri-client/pkg/logs"\n',
      "",
    )
    .replaceAll('logs.RFC3339NanoLenient', '"2006-01-02T15:04:05.999999999Z07:00" /* logs.RFC3339NanoLenient, inlined (k8flare js) */')
    .replaceAll('logs.RFC3339NanoFixed', '"2006-01-02T15:04:05.000000000Z07:00" /* logs.RFC3339NanoFixed, inlined (k8flare js) */'),
);

// ---- kube-scheduler-mirror: the third module mirror (task #4) ----
// k8s.io/kube-scheduler/framework/listers.go imports
// k8s.io/dynamic-resource-allocation/structured for exactly two TYPE
// references (sets.Set[structured.DeviceID], *structured.AllocatedState
// -- both are aliases into the apimachinery-only structured/schedulerapi
// subpackage), but the import links structured's three CEL-backed
// allocator implementations into every scheduler build regardless
// (cel-go + protobuf/genproto + antlr, ~10MB of code -- the dominant
// remaining anchor past schedwidth, measured by wasm name-section
// attribution 2026-07-10). Rewriting that one import to schedulerapi
// severs the whole chain. kube-scheduler is its own staging module, so
// this needs its own mirror: go.wasm.mod's k8s.io/kube-scheduler
// replace points here; go.mod (host builds, cmd/scheduler,
// conformance CI) keeps pointing at the untouched upstream staging
// module and is byte-for-byte unaffected.
const KSCHED_DST = path.join(ROOT, ".build/kube-scheduler-mirror");
const kschedSrc = resolveModuleDir(
  `${upstreamModule}/staging/src/k8s.io/kube-scheduler`,
  upstreamVersion,
);
console.log(`gen-k8s-js-mirror: copying ${kschedSrc} -> ${KSCHED_DST}...`);
replaceMirrorDir(kschedSrc, KSCHED_DST);
checkPin(
  "gen-k8s-js-mirror",
  path.join(kschedSrc, "framework/listers.go"),
  path.join(OVERLAY_DIR, "upstream-ksched-listers.go.sha256"),
  "Re-review the structured -> schedulerapi import rewrite below for framework/listers.go, then refresh upstream-ksched-listers.go.sha256.",
);
{
  const listersPath = path.join(KSCHED_DST, "framework/listers.go");
  const before = '\t"k8s.io/dynamic-resource-allocation/structured"\n';
  const after = '\tstructured "k8s.io/dynamic-resource-allocation/structured/schedulerapi"\n';
  const listers = fs.readFileSync(listersPath, "utf8");
  if (!listers.includes(before)) {
    throw new Error("gen-k8s-js-mirror: structured import not found in kube-scheduler listers.go");
  }
  fs.writeFileSync(listersPath, listers.replace(before, after));
}
console.log(`gen-k8s-js-mirror: kube-scheduler mirror done (${countFiles(KSCHED_DST)} files)`);

console.log(`gen-k8s-js-mirror: done (${countFiles(DST)} files at ${DST})`);
