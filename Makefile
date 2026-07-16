# Local build/dev entry points. scripts/ was removed 2026-07-08: pure
# shell logic lives directly in recipes below; JSON/text-manipulation
# logic (mirror generation, wasm chunking/patching) lives in
# packages/wasm-build/src/*.ts, run via plain `node file.ts` (this repo's
# Node version strips TS types natively, no build step or extra runner
# needed). scripts/setup-tunnel.sh and scripts/ec2-user-data.sh moved to
# infra/ -- they're one-time/guided infra bootstrap, not build steps.
#
# Prefers real dependency-based rebuilds (GNU Make's file-mtime rules)
# over always re-running everything: `make wasm` only pays the ~2min kcm
# wasm-opt pass when apiserver/kcm's own Go source (or the k3s pin)
# actually changed since the last build.
#
# CI does NOT rely on Make's mtime staleness check -- a fresh `git
# checkout` doesn't preserve commit timestamps, so "is the committed
# output stale" can't be trusted there. CI calls `npm run build:wasm`,
# which forces a rebuild (`make -B wasm`) unconditionally on every run.

SHELL := /usr/bin/env bash

WASM_TOOLS := packages/wasm-build/src
ASSETS := workers/k8flare/assets/wasm
BUILD := .build/wasm
CAP := 67108864 # the Worker Loader's 64MiB total-module-bytes cap (S14 Part 2)

APISERVER_SRC := $(shell find pkg/apiserver -name '*.go') go.wasm.mod
# KCM_SRC excludes pkg/controllers/gc and its cmd/gc-wasm entrypoint: they
# share the pkg/controllers/cmd parent directory but not a package with
# controllermanager.go (see pkg/controllers/gc's doc comment), so kcm.
# manifest.json has no reason to rebuild when only GC-specific source
# changes.
KCM_SRC := $(shell find pkg/controllers pkg/leanclient -name '*.go' -not -path 'pkg/controllers/gc/*' -not -path 'pkg/controllers/cmd/gc-wasm/*') \
	go.wasm.mod \
	$(shell find pkg/clientgo-lean-overlays pkg/k8s-js-overlays -type f)
GC_SRC := $(shell find pkg/controllers/gc pkg/controllers/restconfig pkg/controllers/cmd/gc-wasm pkg/leanclient pkg/apiserver/apidef -name '*.go') \
	go.wasm.mod \
	$(shell find pkg/clientgo-lean-overlays pkg/k8s-js-overlays -type f)
SCHED_SRC := $(shell find pkg/controllers/sched pkg/controllers/restconfig pkg/controllers/cmd/kcm-wasm/scheduler pkg/leanclient -name '*.go') \
	go.wasm.mod \
	$(shell find pkg/clientgo-lean-overlays pkg/k8s-js-overlays -type f)
NODES_AGENT_SRC := $(shell find pkg/agent cmd/agent -name '*.go') go.mod go.sum

.PHONY: all wasm wasm-apiserver wasm-kcm wasm-gc wasm-sched gen-mirrors gen check vet test dev deploy clean-wasm nodes-agent setup-tunnel help

all: wasm

help:
	@echo "targets: wasm wasm-apiserver wasm-kcm wasm-gc wasm-sched gen check vet test dev deploy clean-wasm nodes-agent setup-tunnel"

## gen-mirrors: regenerate .build/{k8s-js,clientgo-lean}-mirror, the local
## copies go.mod's k8s.io/kubernetes and k8s.io/client-go replace directives
## point at. Required before ANY Go build in this repo (not just wasm) --
## always runs (gen-*.ts own their own drift-check/idempotency discipline,
## same convention as everything they replaced); see CLAUDE.md's mirror-
## regeneration caveat before changing that.
gen-mirrors:
	node $(WASM_TOOLS)/gen-k8s-js-mirror.ts
	node $(WASM_TOOLS)/gen-clientgo-lean-mirror.ts

$(ASSETS)/wasm_exec.js: $(WASM_TOOLS)/patch-wasm-exec.ts $(WASM_TOOLS)/gomod.ts
	@mkdir -p $(ASSETS)
	node $(WASM_TOOLS)/patch-wasm-exec.ts "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $@

## wasm: build all WASM chunks (apiserver + kcm + gc + sched); skipped per-binary if its inputs are unchanged
wasm: $(ASSETS)/apiserver.manifest.json $(ASSETS)/kcm.manifest.json $(ASSETS)/gc.manifest.json $(ASSETS)/sched.manifest.json $(ASSETS)/selector.wasm

## wasm-apiserver / wasm-kcm / wasm-gc: build just one chunk -- e.g. CI
## jobs that never touch KCM/GC skip their ~2min wasm-opt pass this way.
wasm-apiserver: $(ASSETS)/apiserver.manifest.json
wasm-kcm: $(ASSETS)/kcm.manifest.json
wasm-gc: $(ASSETS)/gc.manifest.json
wasm-sched: $(ASSETS)/sched.manifest.json

wasm-selector: $(ASSETS)/selector.wasm

$(ASSETS)/apiserver.manifest.json: $(APISERVER_SRC) $(ASSETS)/wasm_exec.js | gen-mirrors
	@command -v wasm-opt >/dev/null 2>&1 || { echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2; exit 1; }
	@mkdir -p $(ASSETS) $(BUILD)
	echo "== apiserver (./pkg/apiserver/cmd/apiserver-wasm)"; \
	GOFLAGS=-modfile=go.wasm.mod GOOS=js GOARCH=wasm go build -tags leanwidth -ldflags="-s -w" -trimpath -o $(BUILD)/apiserver.wasm ./pkg/apiserver/cmd/apiserver-wasm; \
	wasm-opt -Oz \
		--strip-debug --strip-producers \
		--enable-bulk-memory --enable-nontrapping-float-to-int \
		--enable-sign-ext --enable-mutable-globals \
		$(BUILD)/apiserver.wasm -o $(BUILD)/apiserver.opt.wasm; \
	raw=$$(wc -c < $(BUILD)/apiserver.opt.wasm | tr -d ' '); \
	if [ "$$raw" -ge $(CAP) ]; then \
		echo "::error::apiserver ($$raw bytes) exceeds the Worker Loader's 64MiB cap ($(CAP) bytes) -- the dynamic worker cannot load. Trim dependencies (see docs/platform-verification.md S14)." >&2; \
		exit 1; \
	fi; \
	echo "apiserver: $$raw bytes ($$(( ($(CAP) - $$raw) / 1024 ))KiB headroom under the 64MiB Loader cap)"; \
	node $(WASM_TOOLS)/chunk-wasm.ts $(BUILD)/apiserver.opt.wasm $(ASSETS) apiserver

# KCM: built against go.wasm.mod (k8s.io/client-go -> width-pruned
# .build/clientgo-lean-mirror) with -tags leanwidth (narrow
# kubernetes.Interface + narrow leanclient stubs) and wasm-opt -Oz
# (REQUIRED: unoptimized it exceeds the 64MiB cap, S14 Part 2; ~2min).
# Interface WIDTH is what keeps this binary under the cap -- full width
# measured 98.6MB opt vs 66.1MB narrow (docs/platform-verification.md).
#
# sched got its width answer on 2026-07-10 (see sched.manifest.json's
# recipe below): -tags schedwidth + the DRA/CEL and cri-client severing
# in gen-k8s-js-mirror.ts took it from 101.1MB opt to ~45MB opt.
$(ASSETS)/kcm.manifest.json: $(KCM_SRC) $(ASSETS)/wasm_exec.js | gen-mirrors
	@command -v wasm-opt >/dev/null 2>&1 || { echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2; exit 1; }
	@mkdir -p $(ASSETS) $(BUILD)
	echo "== kcm (./pkg/controllers/cmd/kcm-wasm)"; \
	GOFLAGS=-modfile=go.wasm.mod GOOS=js GOARCH=wasm go build -tags leanwidth -ldflags="-s -w" -trimpath -o $(BUILD)/kcm.wasm ./pkg/controllers/cmd/kcm-wasm; \
	wasm-opt -Oz \
		--strip-debug --strip-producers \
		--enable-bulk-memory --enable-nontrapping-float-to-int \
		--enable-sign-ext --enable-mutable-globals \
		$(BUILD)/kcm.wasm -o $(BUILD)/kcm.opt.wasm; \
	raw=$$(wc -c < $(BUILD)/kcm.opt.wasm | tr -d ' '); \
	if [ "$$raw" -ge $(CAP) ]; then \
		echo "::error::kcm ($$raw bytes) exceeds the Worker Loader's 64MiB cap ($(CAP) bytes) -- the dynamic worker cannot load. Trim dependencies (see docs/platform-verification.md S14)." >&2; \
		exit 1; \
	fi; \
	echo "kcm: $$raw bytes ($$(( ($(CAP) - $$raw) / 1024 ))KiB headroom under the 64MiB Loader cap)"; \
	node $(WASM_TOOLS)/chunk-wasm.ts $(BUILD)/kcm.opt.wasm $(ASSETS) kcm

# GC: the real, unmodified upstream garbagecollector controller
# (pkg/controllers/gc), a THIRD dynamic worker alongside kcm/sched --
# deliberately its own binary, not folded into kcm.manifest.json's
# RunControllerManager, because its dependency-graph builder needs an
# informer/watch per resource type across apidef.Table, which would
# compete for the same 128MiB isolate memory budget that already forced
# five other controllers out of that binary once (see
# pkg/controllers/controllermanager.go's doc comment). Same
# go.wasm.mod + -tags leanwidth + wasm-opt -Oz shape as kcm above --
# also needed a lean overlay for k8s.io/client-go/informers' aggregate
# GenericInformer type (pkg/clientgo-lean-overlays/informers/
# factory_leanwidth.go) that kcm never needed.
$(ASSETS)/gc.manifest.json: $(GC_SRC) $(ASSETS)/wasm_exec.js | gen-mirrors
	@command -v wasm-opt >/dev/null 2>&1 || { echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2; exit 1; }
	@mkdir -p $(ASSETS) $(BUILD)
	echo "== gc (./pkg/controllers/cmd/gc-wasm)"; \
	GOFLAGS=-modfile=go.wasm.mod GOOS=js GOARCH=wasm go build -tags leanwidth -ldflags="-s -w" -trimpath -o $(BUILD)/gc.wasm ./pkg/controllers/cmd/gc-wasm; \
	wasm-opt -Oz \
		--strip-debug --strip-producers \
		--enable-bulk-memory --enable-nontrapping-float-to-int \
		--enable-sign-ext --enable-mutable-globals \
		$(BUILD)/gc.wasm -o $(BUILD)/gc.opt.wasm; \
	raw=$$(wc -c < $(BUILD)/gc.opt.wasm | tr -d ' '); \
	if [ "$$raw" -ge $(CAP) ]; then \
		echo "::error::gc ($$raw bytes) exceeds the Worker Loader's 64MiB cap ($(CAP) bytes) -- the dynamic worker cannot load. Trim dependencies (see docs/platform-verification.md S14)." >&2; \
		exit 1; \
	fi; \
	echo "gc: $$raw bytes ($$(( ($(CAP) - $$raw) / 1024 ))KiB headroom under the 64MiB Loader cap)"; \
	node $(WASM_TOOLS)/chunk-wasm.ts $(BUILD)/gc.opt.wasm $(ASSETS) gc

# SCHED: the real, unmodified upstream kube-scheduler as the FOURTH
# dynamic worker. Built with -tags schedwidth (its own kubernetes.
# Interface width, narrower than full but wider than KCM's leanwidth --
# see pkg/clientgo-lean-overlays/kubernetes/clientset_schedwidth.go)
# against go.wasm.mod, whose kube-scheduler replace points at
# .build/kube-scheduler-mirror (one import rewritten to sever the
# DRA/CEL chain -- see gen-k8s-js-mirror.ts's kube-scheduler-mirror
# comment). Additional js-pair severings (DRA plugin, cri-client) in
# the k8s mirror took this binary from 101.1MB opt (2026-07-07, "far
# over the cap, host-only forever") to ~45MB opt -- full accounting in
# docs/platform-verification.md's S21 entry.
$(ASSETS)/sched.manifest.json: $(SCHED_SRC) $(ASSETS)/wasm_exec.js | gen-mirrors
	@command -v wasm-opt >/dev/null 2>&1 || { echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2; exit 1; }
	@mkdir -p $(ASSETS) $(BUILD)
	echo "== sched (./pkg/controllers/cmd/kcm-wasm/scheduler)"; \
	GOFLAGS=-modfile=go.wasm.mod GOOS=js GOARCH=wasm go build -tags schedwidth -ldflags="-s -w" -trimpath -o $(BUILD)/sched.wasm ./pkg/controllers/cmd/kcm-wasm/scheduler; \
	wasm-opt -Oz \
		--strip-debug --strip-producers \
		--enable-bulk-memory --enable-nontrapping-float-to-int \
		--enable-sign-ext --enable-mutable-globals \
		$(BUILD)/sched.wasm -o $(BUILD)/sched.opt.wasm; \
	raw=$$(wc -c < $(BUILD)/sched.opt.wasm | tr -d ' '); \
	if [ "$$raw" -ge $(CAP) ]; then \
		echo "::error::sched ($$raw bytes) exceeds the Worker Loader's 64MiB cap ($(CAP) bytes) -- the dynamic worker cannot load. Trim dependencies (see docs/platform-verification.md S14)." >&2; \
		exit 1; \
	fi; \
	echo "sched: $$raw bytes ($$(( ($(CAP) - $$raw) / 1024 ))KiB headroom under the 64MiB Loader cap)"; \
	node $(WASM_TOOLS)/chunk-wasm.ts $(BUILD)/sched.opt.wasm $(ASSETS) sched

SELECTOR_SRC := $(shell find pkg/selectormatch -name '*.go')

# SELECTOR: the watch fan-out's label/field selector matcher (real
# apimachinery parsers), bundled INTO the gateway Worker script as a
# plain wasm module import -- NOT a Loader dynamic worker (production
# Workers forbid runtime WebAssembly compilation, and a Loader hop
# would put a cold start on the watch hot path). No chunking: 4.6MB
# opt / ~1.3MB gzip rides well inside both the 25MiB ASSETS per-file
# cap and the Worker script's 10MiB-gzip deploy budget. Execution
# model and the S8-vs-synchronous-FuncOf verification behind it:
# docs/platform-verification.md S22.
$(ASSETS)/selector.wasm: $(SELECTOR_SRC)
	@command -v wasm-opt >/dev/null 2>&1 || { echo "wasm-opt not found -- install binaryen (mise: aqua:web-assembly/binaryen, apt/brew: binaryen)" >&2; exit 1; }
	@mkdir -p $(ASSETS) $(BUILD)
	echo "== selector (./pkg/selectormatch/cmd/selector-wasm)"; \
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $(BUILD)/selector.wasm ./pkg/selectormatch/cmd/selector-wasm; \
	wasm-opt -Oz \
		--strip-debug --strip-producers \
		--enable-bulk-memory --enable-nontrapping-float-to-int \
		--enable-sign-ext --enable-mutable-globals \
		$(BUILD)/selector.wasm -o $(BUILD)/selector.opt.wasm; \
	cp $(BUILD)/selector.opt.wasm $(ASSETS)/selector.wasm; \
	echo "selector: $$(wc -c < $(ASSETS)/selector.wasm | tr -d ' ') bytes (bundled import, no Loader cap)"

## gen: regenerate derived artifacts from pkg/apiserver/apidef.Table and go.mod's k8s.io/kubernetes pin
gen: | gen-mirrors
	go run ./cmd/k8flare-gen

## check: TypeScript check (vp check)
check:
	vp check

## vet: go vet, split by GOOS -- a plain `go vet ./pkg/...` wildcard fails
## for reasons unrelated to real bugs: pkg/cfruntime is js&&wasm-only (host
## vet correctly excludes it, that's not a failure to route around), and
## pkg/k8s-js-overlays/pkg/clientgo-lean-overlays are flat bags of overlay
## source for OTHER packages (packages/wasm-build/src/gen-*.ts copy them file-by-file into
## .build/*-mirror/), not compilable Go packages of their own -- a wildcard
## trips over their multiple `package` clauses in one directory. Vet only
## the paths that are actually this module's own compilable packages.
vet: | gen-mirrors
	go vet ./pkg/apiserver/... ./pkg/agent/... ./cmd/k8flare-gen/...
	GOOS=js GOARCH=wasm go vet ./pkg/apiserver/cmd/... ./pkg/cfruntime/... ./pkg/controllers/...

## test: apiserver integration tests (spins up its own wrangler dev; needs wasm built first)
test: wasm
	go test -count=1 ./pkg/apiserver/...

## dev: local wrangler dev server
dev:
	wrangler dev -c workers/k8flare/wrangler.jsonc --persist-to .wrangler/state

## nodes-agent: cross-compile the unmodified k3s agent embed (cmd/agent)
## for linux/amd64 into workers/k8flare/images/node/, where the node-image
## Dockerfile COPYs it -- Cloudflare Containers run amd64, and building
## the full k8s tree under qemu on ARM Macs would take 20+ minutes, so
## the Go build happens on the host (S15 round-2 lesson). Run before
## `make deploy` (workers/k8flare).
nodes-agent: workers/k8flare/images/node/k8flare-agent

workers/k8flare/images/node/k8flare-agent: $(NODES_AGENT_SRC) | gen-mirrors
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $@ ./cmd/agent
	echo "$@: $$(wc -c < $@ | tr -d ' ') bytes"

## deploy: build node agent images, then wrangler deploy (uses whatever wasm is already committed -- run `make wasm` first if it needs rebuilding)
deploy: nodes-agent
	wrangler deploy --config workers/k8flare/wrangler.jsonc

## clean-wasm: drop the built chunks so the next `make wasm` rebuilds from scratch
clean-wasm:
	rm -f $(ASSETS)/apiserver.wasm.part* $(ASSETS)/apiserver.manifest.json
	rm -f $(ASSETS)/kcm.wasm.part* $(ASSETS)/kcm.manifest.json
	rm -f $(ASSETS)/gc.wasm.part* $(ASSETS)/gc.manifest.json
	rm -f $(ASSETS)/sched.wasm.part* $(ASSETS)/sched.manifest.json
	rm -f $(ASSETS)/selector.wasm
	rm -f $(ASSETS)/wasm_exec.js

## setup-tunnel: guided Cloudflare Tunnel + VPC Service setup for BYO-VM
## kubelet access (infra/setup-tunnel.sh) -- legacy path, superseded by
## Cloudflare Mesh for new setups (see docs/cloudflare-mesh-networking.md).
## Reads config from env vars (CLOUDFLARE_ACCOUNT_ID, NODE_INTERNAL_IP,
## DRY_RUN, ...), not Make args -- see the script's own usage comment.
setup-tunnel:
	bash infra/setup-tunnel.sh
