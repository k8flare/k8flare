SHELL := /usr/bin/env bash
export GOTOOLCHAIN := auto

ASSETS := packages/control-plane-worker/assets/wasm
BUILD := .build/wasm
CAP := 67108864
GROUPS := core coordination discovery node storage
WASM_OPT := wasm-opt -Oz --strip-debug --strip-producers --enable-bulk-memory --enable-nontrapping-float-to-int --enable-sign-ext --enable-mutable-globals

.PHONY: mirrors wasm gen agent dev devtls kubeconfig check vet test clean

mirrors:
	cd scripts && go run ./mirror

GO_SRC := $(shell find packages -name '*.go' -not -name '*_test.go') go.mod scripts/mirror/main.go $(shell find scripts/mirror/_overlays -type f)

$(ASSETS)/wasm_exec.js: scripts/wasmpack/main.go
	cd scripts && go run ./wasmpack exec ../$@

## One dynamic worker per Go binary; each must stay under the Loader cap.
$(BUILD)/apiserver.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/apiserver/cmd/apiserver-wasm

define OPTIMIZE
	$(WASM_OPT) $< -o $@
	@size=$$(wc -c < $@ | tr -d ' '); echo "$(notdir $@): $$size bytes (cap $(CAP))"; \
		[ "$$size" -lt $(CAP) ] || { echo "exceeds the Worker Loader cap" >&2; exit 1; }
endef

$(BUILD)/apiserver.opt.wasm: $(BUILD)/apiserver.raw.wasm
	$(OPTIMIZE)

$(BUILD)/printers-%.opt.wasm: $(BUILD)/printers-%.raw.wasm
	$(OPTIMIZE)

$(BUILD)/printers-%.raw.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/printers-$*/cmd/printers-wasm

$(ASSETS)/apiserver.manifest.json: $(BUILD)/apiserver.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) apiserver

$(ASSETS)/printers-%.manifest.json: $(BUILD)/printers-%.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) printers-$*

wasm: $(ASSETS)/wasm_exec.js $(ASSETS)/apiserver.manifest.json $(foreach g,$(GROUPS),$(ASSETS)/printers-$(g).manifest.json)

gen:
	cd scripts && go run ./genresources && go run ./genprinters

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
	cd scripts && go vet ./...

test: wasm
	go test -count=1 ./packages/...

clean:
	rm -rf $(BUILD) $(ASSETS)
