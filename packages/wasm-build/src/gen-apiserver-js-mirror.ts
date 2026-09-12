#!/usr/bin/env node
// Regenerates .build/apiserver-js-mirror/: a local copy of the
// k8s.io/apiserver staging module (from the k3s-io/kubernetes fork go.mod
// pins) with the handful of files swapped that otherwise drag the etcd
// client, CEL, or the APF controller into GOOS=js builds. go.wasm.mod's
// `k8s.io/apiserver` replace points here; go.mod (host builds) keeps
// pointing at the untouched upstream staging module.
//
// Why each overlay exists (measured 2026-07-26, spike for the generic
// registry adoption -- see docs/platform-verification.md):
//   - storage/storagebackend/factory: etcd3-backed storage construction
//     (go-systemd + etcd client don't compile under js). Stubbed; k8flare
//     supplies its own storage.Interface.
//   - storage/storagebackend/config.go: imported etcd3 just for
//     LeaseManagerConfig; patched to a local copy of the struct.
//   - storage/feature: polls etcd endpoints for feature support. Stubbed.
//   - sharding/parser.go: embeds a CEL parser for ShardSelector
//     expressions (-5.7MB wasm). k8flare never sets ShardSelector.
//   - storage/cacher/cache_watcher.go: utilflowcontrol.WatchInitialized
//     pulls the APF controller, which needs flowcontrol informers the
//     lean client-go doesn't ship. Call shimmed to a no-op.
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
const OVERLAY_DIR = path.join(ROOT, "pkg/k8s-js-overlays/apiserver");
const DST = path.join(ROOT, ".build/apiserver-js-mirror");

const { module: upstreamModule, version: upstreamVersion } = readUpstreamModule(
  path.join(ROOT, "pkg/k8s-js-overlays/upstream-module.txt"),
);
const stagingModule = `${upstreamModule}/staging/src/k8s.io/apiserver`;

console.log(`gen-apiserver-js-mirror: resolving ${stagingModule}@${upstreamVersion}...`);
const src = resolveModuleDir(stagingModule, upstreamVersion);

// Drift check: every upstream file this mirror replaces or patches is
// pinned; a k8s version bump that changes one of them must be reviewed
// (does the overlay still cover everything?) before the pin is refreshed.
const pins: Array<[string, string]> = [
  ["pkg/storage/storagebackend/factory/factory.go", "upstream-factory.go.sha256"],
  ["pkg/storage/storagebackend/factory/etcd3.go", "upstream-etcd3.go.sha256"],
  ["pkg/storage/storagebackend/config.go", "upstream-config.go.sha256"],
  ["pkg/storage/feature/feature_support_checker.go", "upstream-feature_support_checker.go.sha256"],
  ["pkg/sharding/parser.go", "upstream-parser.go.sha256"],
  ["pkg/storage/cacher/cache_watcher.go", "upstream-cache_watcher.go.sha256"],
  ["pkg/endpoints/installer.go", "upstream-installer.go.sha256"],
];
for (const [rel, pin] of pins) {
  checkPin(
    "gen-apiserver-js-mirror",
    path.join(src, rel),
    path.join(OVERLAY_DIR, pin),
    `Review pkg/k8s-js-overlays/apiserver against the new upstream ${rel}, update the overlay if the change matters for GOOS=js, then refresh ${pin}.`,
  );
}

console.log(`gen-apiserver-js-mirror: copying ${src} -> ${DST}...`);
replaceMirrorDir(src, DST);

const cp = (overlay: string, rel: string) =>
  fs.copyFileSync(path.join(OVERLAY_DIR, overlay), path.join(DST, rel));

/** Replace one exact line in a mirrored file; fails loudly if it is not there. */
const patch = (rel: string, from: string, to: string) => {
  const file = path.join(DST, rel);
  const before = fs.readFileSync(file, "utf8");
  if (!before.includes(from)) {
    throw new Error(
      `gen-apiserver-js-mirror: ${rel} no longer contains the line this patch replaces:\n${from}`,
    );
  }
  fs.writeFileSync(file, before.replace(from, to));
};

const appendToFile = (rel: string, text: string) => {
  fs.appendFileSync(path.join(DST, rel), text);
};

// factory: stub replaces the whole package (etcd3.go deleted, tests dropped).
for (const f of ["etcd3.go", "factory_test.go", "etcd3_test.go", "tls_test.go"]) {
  fs.rmSync(path.join(DST, "pkg/storage/storagebackend/factory", f), { force: true });
}
cp("factory.go", "pkg/storage/storagebackend/factory/factory.go");
cp("storagebackend_config.go", "pkg/storage/storagebackend/config.go");
cp("feature_support_checker.go", "pkg/storage/feature/feature_support_checker.go");
fs.rmSync(path.join(DST, "pkg/storage/feature/feature_support_checker_test.go"), { force: true });
cp("sharding_parser.go", "pkg/sharding/parser.go");
// storageversion/manager.go: see pkg/k8s-js-overlays/apiserver/storageversion_manager.go.
cp("storageversion_manager.go", "pkg/storageversion/manager.go");
fs.rmSync(path.join(DST, "pkg/sharding/parser_test.go"), { force: true });
cp("cache_watcher.go", "pkg/storage/cacher/cache_watcher.go");
cp("cacher_flowcontrol_shim.go", "pkg/storage/cacher/flowcontrol_js_shim.go");

// endpoints/installer.go: the request scope's hub version is hardcoded to the
// internal one, and this apiserver registers external versions only -- so
// PATCH, which decodes to the hub before applying the merge, fails with "no
// kind is registered for the internal version". Fall back to the served
// version when the scheme has no internal kind, which is what an apiserver
// without internal types (apiextensions does the same) needs. A one-condition
// patch rather than a whole-file overlay: the file is 1300 lines and carrying
// it would hide every upstream change to the installer behind our copy.
patch(
  "pkg/endpoints/installer.go",
  "\t\tHubGroupVersion: schema.GroupVersion{Group: fqKindToRegister.Group, Version: runtime.APIVersionInternal},",
  "\t\tHubGroupVersion: hubGroupVersionFor(a.group.Typer, a.group.GroupVersion, fqKindToRegister),",
);
appendToFile(
  "pkg/endpoints/installer.go",
  `
// hubGroupVersionFor returns the version a request body is decoded to before
// a patch is applied: the internal one when the scheme has it, and the served
// one when it does not. Added by gen-apiserver-js-mirror.ts.
func hubGroupVersionFor(typer runtime.ObjectTyper, served schema.GroupVersion, kind schema.GroupVersionKind) schema.GroupVersion {
	internal := schema.GroupVersion{Group: kind.Group, Version: runtime.APIVersionInternal}
	if typer != nil && typer.Recognizes(internal.WithKind(kind.Kind)) {
		return internal
	}
	return served
}
`,
);

console.log(`gen-apiserver-js-mirror: done (${countFiles(DST)} files at ${DST})`);
