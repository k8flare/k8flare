#!/usr/bin/env node
// Regenerates .build/clientgo-lean-mirror/: a full local copy of the
// upstream k3s-io/kubernetes-forked k8s.io/client-go module, with
// kubernetes/typed/<group>/<version>, applyconfigurations/<group>/<version>
// and kubernetes/clientset.go replaced by pkg/clientgo-lean-overlays/'s
// hand-curated, pruned versions.
//
// Why this exists, and what it prunes: see
// pkg/clientgo-lean-overlays/README.md. Short version: importing
// client-go's own generated PodInterface (etc.) -- required to satisfy
// upstream controller code's exact client parameter type -- costs ~44MiB
// of linked GOOS=js/wasm code per type, almost entirely from *unrelated*
// sibling files (other core/v1 types' generated code) in the same package
// being linked despite never being referenced. go build -overlay can't fix
// this (client-go resolves into GOMODCACHE; overlay refuses to touch it,
// same restriction as pkg/k8s-js-overlays/), so -- same lever as
// that sibling mirror -- this generates a pruned local copy and go.mod
// replaces k8s.io/client-go with it.
//
// go.mod's k8s.io/client-go replace line depends on this directory
// existing on disk -- same operational note as gen-k8s-js-mirror.ts: run
// this (or `make wasm`, which calls it) after every fresh clone before
// any Go build works.
import * as fs from "node:fs";
import * as path from "node:path";
import {
  checkPin,
  readUpstreamModule,
  replaceMirrorDir,
  resolveModuleDir,
  countFiles,
} from "./gomod.ts";

const ROOT = path.resolve(import.meta.dirname, "../../..");
const OVERLAY_DIR = path.join(ROOT, "pkg/clientgo-lean-overlays");
const DST = path.join(ROOT, ".build/clientgo-lean-mirror");

const { module: upstreamModule, version: upstreamVersion } = readUpstreamModule(
  path.join(OVERLAY_DIR, "upstream-module.txt"),
);

console.log(`gen-clientgo-lean-mirror: resolving ${upstreamModule}@${upstreamVersion}...`);
const src = resolveModuleDir(upstreamModule, upstreamVersion);

console.log(`gen-clientgo-lean-mirror: copying ${src} -> ${DST}...`);
replaceMirrorDir(src, DST);

// kubernetes/typed/<group>/<version> and applyconfigurations/<group>/<version>
// are NOT pruned, deliberately -- see this repo's honest-correction note
// (docs/platform-verification.md, Phase 10) and
// pkg/clientgo-lean-overlays/README.md. Short version: pruning them broke
// the moment pkg/scheduler.New's informerFactory parameter (fixed to the
// real k8s.io/client-go/informers.SharedInformerFactory, which imports
// all ~54 groups' typed/applyconfigurations packages directly in its own
// factory.go) entered the same binary -- those packages cross-reference
// each other extensively (e.g. autoscaling's
// HorizontalPodAutoscalerApplyConfiguration references core's
// ObjectReferenceApplyConfiguration), so partial pruning cascades into
// compile errors across unrelated groups. The per-type overlay files
// (kubernetes/typed/<group>/<version>/*.go, applyconfigurations/<group>/
// <version>/*.go) are kept in pkg/clientgo-lean-overlays/ for the record
// and because pkg/leanclient/gen's generated clients still implement
// their (now merely redundant-with-upstream, not size-saving)
// interfaces, but this script no longer swaps them in.

fs.copyFileSync(
  path.join(OVERLAY_DIR, "kubernetes/clientset.go"),
  path.join(DST, "kubernetes/clientset.go"),
);
// leanwidth variant: narrow kubernetes.Interface for the `-tags leanwidth`
// KCM wasm build (see its doc comment for why width decides binary size).
fs.copyFileSync(
  path.join(OVERLAY_DIR, "kubernetes/clientset_leanwidth.go"),
  path.join(DST, "kubernetes/clientset_leanwidth.go"),
);

// kubernetes/scheme/register.go: THE size lever for the GOOS=js binaries.
// Upstream init()-registers all ~55 group-versions, and init side effects
// defeat dead-code elimination -- importing client-go/discovery (reached
// via applyconfigurations/meta/v1 from every typed client) dragged ~21MiB
// of generated API-type code into the wasm builds. The overlay registers
// only the group-versions the wasm control-plane binaries actually
// serialize; see its doc comment. Drift-checked like the k8s-js-mirror
// overlays: a client-go bump that changes upstream's register.go must be
// human-reviewed here.
checkPin(
  "gen-clientgo-lean-mirror",
  path.join(src, "kubernetes/scheme/register.go"),
  path.join(OVERLAY_DIR, "upstream-scheme-register.go.sha256"),
  `Review the new upstream file, update kubernetes/scheme/register.go in ${OVERLAY_DIR} if group-versions were added/renamed, then refresh upstream-scheme-register.go.sha256.`,
);
fs.copyFileSync(
  path.join(OVERLAY_DIR, "kubernetes/scheme/register.go"),
  path.join(DST, "kubernetes/scheme/register.go"),
);
// register_sched.go: resource/v1 (DRA) scheme registration, `!leanwidth`-
// tagged so it only applies to the scheduler build, never KCM's -- see
// register.go's doc comment for why this must not be unconditional.
fs.copyFileSync(
  path.join(OVERLAY_DIR, "kubernetes/scheme/register_sched.go"),
  path.join(DST, "kubernetes/scheme/register_sched.go"),
);

// informers/<group>/<version> and listers/<group>/<version> are NOT
// pruned, deliberately -- see pkg/clientgo-lean-overlays/README.md's
// "correction" note. kubernetes.Interface (above) stays full width
// because it's an *external* fixed contract (client-go/tools/
// leaderelection/resourcelock, and every informers/<group>/<version>'s
// own NewFilteredXInformer, hardcode a parameter of that exact type --
// narrowing it broke compilation the same way narrowing
// kubernetes.Interface itself did, per that same correction note).
//
// informers/factory.go (the SharedInformerFactory *aggregate*) is
// different: nothing external requires its 19-group width, only
// pkg/scheduler.New's own call site (this repo's fork of it) -- so it's
// pruned to the 5 groups (Core/Apps/Storage/Resource/Scheduling) the
// real, unmodified upstream kube-scheduler actually calls. See
// pkg/clientgo-lean-overlays/informers/factory.go's doc comment for the
// full accounting. Drift-checked like scheme/register.go above.
checkPin(
  "gen-clientgo-lean-mirror",
  path.join(src, "informers/factory.go"),
  path.join(OVERLAY_DIR, "upstream-informers-factory.go.sha256"),
  `Review the new upstream file, update ${OVERLAY_DIR}/informers/factory.go if the group accessor set changed, then refresh upstream-informers-factory.go.sha256.`,
);
fs.copyFileSync(
  path.join(OVERLAY_DIR, "informers/factory.go"),
  path.join(DST, "informers/factory.go"),
);
// generic.go implements ForResource via a switch spanning every API type
// in all ~54 groups (the mechanism that would silently re-widen the
// pruned factory back out) -- deleted; the pruned factory.go above
// redeclares GenericInformer and stubs ForResource itself (confirmed
// unused by pkg/scheduler's own call sites).
fs.rmSync(path.join(DST, "informers/generic.go"), { force: true });

console.log(`gen-clientgo-lean-mirror: done (${countFiles(DST)} files at ${DST})`);
