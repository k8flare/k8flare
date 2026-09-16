SHELL := /usr/bin/env bash
MAKEFLAGS += --jobs=8
.SECONDARY:
export BINARYEN_CORES := 2
export GOTOOLCHAIN := auto

ASSETS := packages/control-plane-worker/assets/wasm
BUILD := .build/wasm
CAP := 67108864
GROUPS := core coordination discovery node storage apps policy resource rbac batch
API_GROUPS := core coordination discovery node storage authentication authorization apps policy resource rbac batch
WASM_OPT := wasm-opt -Oz --strip-debug --strip-producers --enable-bulk-memory --enable-nontrapping-float-to-int --enable-sign-ext --enable-mutable-globals

.PHONY: mirrors wasm gen agent dev devtls kubeconfig check vet test clean e2e deploycheck

mirrors:
	cd scripts && go run ./mirror

GO_SRC := $(shell find packages -name '*.go' -not -name '*_test.go') go.mod scripts/mirror/main.go $(shell find scripts/mirror/_overlays -type f)

$(ASSETS)/wasm_exec.js: scripts/wasmpack/main.go
	cd scripts && go run ./wasmpack exec ../$@

## One dynamic worker per Go binary; each must stay under the Loader cap.
$(BUILD)/apiserver.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/apiserver/cmd/apiserver-wasm

## wasm-opt -Oz is the slow step (40s for the largest binary); it is skipped
## when the raw binary's hash matches the one the existing output came from.
define OPTIMIZE
	@raw=$$(shasum -a 256 $< | cut -c1-64); \
	if [ -f $@ ] && [ "$$(cat $@.sha256 2>/dev/null)" = "$$raw" ]; then touch $@; echo "$(notdir $@): unchanged"; exit 0; fi; \
	$(WASM_OPT) $< -o $@ && echo "$$raw" > $@.sha256 && \
	size=$$(wc -c < $@ | tr -d ' '); echo "$(notdir $@): $$size bytes (cap $(CAP))"; \
		[ "$$size" -lt $(CAP) ] || { echo "exceeds the Worker Loader cap" >&2; exit 1; }
endef

$(BUILD)/apiserver.opt.wasm: $(BUILD)/apiserver.raw.wasm
	$(OPTIMIZE)

$(BUILD)/openapi.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/openapi/cmd/openapi-wasm

$(BUILD)/openapi.opt.wasm: $(BUILD)/openapi.raw.wasm
	$(OPTIMIZE)

$(ASSETS)/openapi.manifest.json: $(BUILD)/openapi.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) openapi

$(BUILD)/customresources.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/customresources/cmd/customresources-wasm

$(BUILD)/customresources.opt.wasm: $(BUILD)/customresources.raw.wasm
	$(OPTIMIZE)

$(ASSETS)/customresources.manifest.json: $(BUILD)/customresources.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) customresources

$(BUILD)/apiserver-%.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/apiserver-$*/cmd/apiserver-wasm

$(BUILD)/apiserver-%.opt.wasm: $(BUILD)/apiserver-%.raw.wasm
	$(OPTIMIZE)

$(ASSETS)/apiserver-%.manifest.json: $(BUILD)/apiserver-%.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) apiserver-$*

$(BUILD)/scheduler.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/scheduler/cmd/scheduler-wasm

$(BUILD)/scheduler.opt.wasm: $(BUILD)/scheduler.raw.wasm
	$(OPTIMIZE)

$(ASSETS)/scheduler.manifest.json: $(BUILD)/scheduler.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) scheduler

$(BUILD)/controllers.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/controllers/cmd/controllers-wasm

$(BUILD)/controllers.opt.wasm: $(BUILD)/controllers.raw.wasm
	$(OPTIMIZE)

$(ASSETS)/controllers.manifest.json: $(BUILD)/controllers.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) controllers

$(BUILD)/printers-%.opt.wasm: $(BUILD)/printers-%.raw.wasm
	$(OPTIMIZE)

$(BUILD)/printers-%.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/printers-$*/cmd/printers-wasm

$(ASSETS)/apiserver.manifest.json: $(BUILD)/apiserver.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) apiserver

$(ASSETS)/printers-%.manifest.json: $(BUILD)/printers-%.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) printers-$*

## node-tunnel is bundled straight into the shell Worker (not the Loader):
## the NodeTunnel Durable Object needs it synchronously, so it ships as one
## compiled wasm asset instead of the chunked manifests above.
NODE_TUNNEL_WASM := packages/node-tunnel/assets/node-tunnel.wasm

$(BUILD)/node-tunnel.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/node-tunnel/cmd/node-tunnel-wasm

$(BUILD)/node-tunnel.opt.wasm: $(BUILD)/node-tunnel.raw.wasm
	$(OPTIMIZE)

$(NODE_TUNNEL_WASM): $(BUILD)/node-tunnel.opt.wasm
	mkdir -p $(dir $@)
	cp $< $@

wasm: $(ASSETS)/wasm_exec.js $(ASSETS)/apiserver.manifest.json $(foreach g,$(API_GROUPS),$(ASSETS)/apiserver-$(g).manifest.json) $(ASSETS)/openapi.manifest.json $(ASSETS)/customresources.manifest.json $(ASSETS)/scheduler.manifest.json $(ASSETS)/controllers.manifest.json $(foreach g,$(GROUPS),$(ASSETS)/printers-$(g).manifest.json) $(NODE_TUNNEL_WASM)

gen:
	cd scripts && go run ./genresources && go run ./genprinters && go run ./genopenapi

agent: mirrors
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o .build/bin/k8flare-agent-linux-arm64 ./packages/agent

# CLAUDECODE is unset on purpose: with it set, wrangler dev enters its
# AI-agent mode, whose observability capture buffers application/json
# streaming responses until they close, which stalls every JSON watch.
dev:
	env -u CLAUDECODE -u AI_AGENT pnpm exec wrangler dev -c wrangler.jsonc --local --persist-to .wrangler/state --port 18787

devtls:
	cd scripts && go run ./devtls -listen :6443 -upstream http://127.0.0.1:18787 -dir ../.build/devtls

## kubeconfig: write .build/kubeconfig.yaml for the dev stack (make dev + make devtls).
## The admin token comes from .dev.vars; the CA is the one devtls generated.
kubeconfig:
	@test -f .build/devtls/ca.crt || { echo "run make devtls first (it generates .build/devtls/ca.crt)" >&2; exit 1; }
	@token=$$(sed -n 's/^ADMIN_TOKEN=//p' .dev.vars); \
	printf 'apiVersion: v1\nkind: Config\nclusters:\n- name: k8flare-dev\n  cluster:\n    server: https://localhost:6443\n    certificate-authority: %s/.build/devtls/ca.crt\nusers:\n- name: admin\n  user:\n    token: %s\ncontexts:\n- name: k8flare-dev\n  context:\n    cluster: k8flare-dev\n    user: admin\ncurrent-context: k8flare-dev\n' "$(CURDIR)" "$$token" > .build/kubeconfig.yaml
	@echo "export KUBECONFIG=$(CURDIR)/.build/kubeconfig.yaml"

check:
	pnpm exec wrangler types >/dev/null
	pnpm exec tsc --noEmit

vet: mirrors
	go vet ./packages/...
	GOOS=js GOARCH=wasm go vet ./packages/...

deploycheck:
	cd scripts && go run ./deploycheck -server $(SERVER) -token $(TOKEN) -node $(NODE)

e2e:
	cd scripts && go run ./e2e -set $(or $(SET),required) -procs $(or $(PROCS),4)
	cd scripts && go vet ./...

test: wasm
	go test -count=1 ./packages/...

clean:
	rm -rf $(BUILD) $(ASSETS)
