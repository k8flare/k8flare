// Regenerates .build/component-base-mirror/: a local copy of
// k8s.io/component-base with tracing/utils.go replaced.
//
// Upstream's tracing/utils.go imports the OTLP-over-gRPC trace exporter at
// package scope, so k8s.io/apiserver/pkg/storage/cacher -- which wants only
// tracing.Start and tracing.SpanFromContext, both in tracing.go -- links
// google.golang.org/grpc and the protobuf runtime through it. Measured on
// the apiserver chunk before this mirror existed: grpc 1.37MB, protobuf
// runtime 2.44MB, opentelemetry 0.66MB of a 42.6MB code section
// (docs/wasm-size.md).
//
// Every caller of the functions utils.go provides is outside the link graph
// (pkg/server/options, pkg/server, pkg/util/webhook, pkg/endpoints/filters),
// verified with `go list -deps`; the replacement keeps the two that linked
// code names -- NewNoopTracerProvider and Propagators -- and drops the rest.
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
const OVERLAY_DIR = path.join(ROOT, "pkg/k8s-js-overlays/component-base");
const DST = path.join(ROOT, ".build/component-base-mirror");

const { module: upstreamModule, version: upstreamVersion } = readUpstreamModule(
  path.join(OVERLAY_DIR, "upstream-module.txt"),
);

console.log(`gen-component-base-mirror: resolving ${upstreamModule}@${upstreamVersion}...`);
const src = resolveModuleDir(upstreamModule, upstreamVersion);

checkPin(
  "gen-component-base-mirror",
  path.join(src, "tracing/utils.go"),
  path.join(OVERLAY_DIR, "upstream-tracing-utils.go.sha256"),
  "Review pkg/k8s-js-overlays/component-base/tracing_utils.go against the new upstream tracing/utils.go, then refresh the pin.",
);

console.log(`gen-component-base-mirror: copying ${src} -> ${DST}...`);
replaceMirrorDir(src, DST);

fs.copyFileSync(path.join(OVERLAY_DIR, "tracing_utils.go"), path.join(DST, "tracing/utils.go"));

console.log(`gen-component-base-mirror: done (${countFiles(DST)} files at ${DST})`);
