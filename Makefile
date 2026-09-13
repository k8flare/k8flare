SHELL := /usr/bin/env bash
export GOTOOLCHAIN := auto

ASSETS := packages/control-plane-worker/assets/wasm
BUILD := .build/wasm
CAP := 67108864

.PHONY: mirrors wasm gen agent dev devtls check vet test clean

mirrors:
	cd scripts && go run ./mirror

GO_SRC := $(shell find packages -name '*.go' -not -name '*_test.go') go.mod scripts/mirror/main.go $(shell find scripts/mirror/_overlays -type f)

$(ASSETS)/wasm_exec.js: scripts/wasmpack/main.go
	cd scripts && go run ./wasmpack exec ../$@

$(BUILD)/apiserver.wasm: $(GO_SRC) | mirrors
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $@ ./packages/apiserver/cmd/apiserver-wasm

$(BUILD)/apiserver.opt.wasm: $(BUILD)/apiserver.wasm
	wasm-opt -Oz --strip-debug --strip-producers --enable-bulk-memory --enable-nontrapping-float-to-int --enable-sign-ext --enable-mutable-globals $< -o $@
	@size=$$(wc -c < $@ | tr -d ' '); echo "apiserver.opt.wasm: $$size bytes (cap $(CAP))"; \
		[ "$$size" -lt $(CAP) ] || { echo "exceeds the Worker Loader cap" >&2; exit 1; }

$(ASSETS)/apiserver.manifest.json: $(BUILD)/apiserver.opt.wasm
	cd scripts && go run ./wasmpack chunk ../$< ../$(ASSETS) apiserver

wasm: $(ASSETS)/wasm_exec.js $(ASSETS)/apiserver.manifest.json

gen:
	cd scripts && go run ./genresources

agent: mirrors
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o .build/bin/k8flare-agent-linux-arm64 ./packages/agent

# CLAUDECODE is unset on purpose: with it set, wrangler dev enters its
# AI-agent mode, whose observability capture buffers application/json
# streaming responses until they close, which stalls every JSON watch.
dev:
	env -u CLAUDECODE -u AI_AGENT pnpm exec wrangler dev --local --persist-to .wrangler/state --port 18787

devtls:
	cd scripts && go run ./devtls -listen :6443 -upstream http://127.0.0.1:18787 -dir ../.build/devtls

check:
	pnpm exec wrangler types >/dev/null && pnpm exec tsc --noEmit

vet: mirrors
	go vet ./packages/...
	GOOS=js GOARCH=wasm go vet ./packages/...
	cd scripts && go vet ./...

test: wasm
	go test -count=1 ./packages/...

clean:
	rm -rf $(BUILD) $(ASSETS)
