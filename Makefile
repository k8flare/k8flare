# Local dev entry points. Prefers real dependency-based rebuilds (GNU
# Make's file-mtime rules) over always re-running scripts/*.sh: `make wasm`
# only pays the ~2min kcm wasm-opt pass when apiserver/kcm's own Go source
# (or the k3s pin) actually changed since the committed chunks.
#
# CI does NOT use this file -- a fresh `git checkout` doesn't preserve
# commit timestamps, so Make's mtime comparison can't be trusted to decide
# "is the committed wasm stale" there. CI keeps calling `npm run build:wasm`
# (-> scripts/build-wasm-chunks.sh) directly, unconditionally, on every run.
# This Makefile is a local convenience layer only.

SHELL := /usr/bin/env bash

WASM_DIR := workers/k8flare/assets/wasm

APISERVER_SRC := $(shell find pkg/apiserver cmd/apiserver-wasm -name '*.go') go.mod go.sum
KCM_SRC := $(shell find pkg/controllers pkg/leanclient cmd/kcm-wasm -name '*.go') \
	go.wasm.mod \
	$(shell find third_party/clientgo-lean-overlays third_party/k8s-js-overlays -type f)

.PHONY: all wasm gen check vet test dev deploy clean-wasm help

all: wasm

help:
	@echo "targets: wasm gen check vet test dev deploy clean-wasm"

## wasm: build both WASM chunks (apiserver + kcm); skipped per-binary if its inputs are unchanged
wasm: $(WASM_DIR)/apiserver.manifest.json $(WASM_DIR)/kcm.manifest.json

$(WASM_DIR)/apiserver.manifest.json: $(APISERVER_SRC)
	bash scripts/build-wasm-chunks.sh apiserver

$(WASM_DIR)/kcm.manifest.json: $(KCM_SRC)
	bash scripts/build-wasm-chunks.sh kcm

## gen: regenerate derived artifacts from pkg/apiserver/apidef.Table and go.mod's k8s.io/kubernetes pin
gen:
	go run ./cmd/k8flare-gen

## check: TypeScript check (vp check)
check:
	vp check

## vet: go vet across the non-generated-only packages
vet:
	go vet ./pkg/... ./cmd/k8flare-gen/...

## test: apiserver integration tests (spins up its own wrangler dev; needs wasm built first)
test: wasm
	go test -count=1 ./pkg/apiserver/...

## dev: local wrangler dev server
dev:
	wrangler dev -c workers/k8flare/wrangler.jsonc --persist-to .wrangler/state

## deploy: build node agent images, then wrangler deploy (uses whatever wasm is already committed -- run `make wasm` first if it needs rebuilding)
deploy:
	bash scripts/build-nodes-agent.sh
	wrangler deploy --config workers/k8flare/wrangler.jsonc

## clean-wasm: drop the built chunks so the next `make wasm` rebuilds from scratch
clean-wasm:
	rm -f $(WASM_DIR)/apiserver.wasm.part* $(WASM_DIR)/apiserver.manifest.json
	rm -f $(WASM_DIR)/kcm.wasm.part* $(WASM_DIR)/kcm.manifest.json
