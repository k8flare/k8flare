SHELL := /usr/bin/env bash
export GOTOOLCHAIN := auto

ASSETS := worker/assets/wasm
BUILD := .build/wasm
CAP := 67108864

.PHONY: mirrors wasm agent dev devtls check vet test clean

mirrors:
	cd hack && go run ./mirror

$(ASSETS)/wasm_exec.js: hack/wasmpack/main.go
	cd hack && go run ./wasmpack exec ../$@

wasm: mirrors $(ASSETS)/wasm_exec.js
	mkdir -p $(BUILD)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o $(BUILD)/apiserver.wasm ./pkg/apiserver/cmd/apiserver-wasm
	wasm-opt -Oz --strip-debug --strip-producers --enable-bulk-memory --enable-nontrapping-float-to-int --enable-sign-ext --enable-mutable-globals \
		$(BUILD)/apiserver.wasm -o $(BUILD)/apiserver.opt.wasm
	@size=$$(wc -c < $(BUILD)/apiserver.opt.wasm | tr -d ' '); echo "apiserver.opt.wasm: $$size bytes (cap $(CAP))"; \
		[ "$$size" -lt $(CAP) ] || { echo "exceeds the Worker Loader cap" >&2; exit 1; }
	cd hack && go run ./wasmpack chunk ../$(BUILD)/apiserver.opt.wasm ../$(ASSETS) apiserver

# CLAUDECODE is unset on purpose: with it set, wrangler dev enters its
# AI-agent mode, whose observability capture buffers application/json
# streaming responses until they close, which stalls every JSON watch.
agent: mirrors
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o .build/bin/k8flare-agent-linux-arm64 ./cmd/agent

dev:
	cd worker && env -u CLAUDECODE -u AI_AGENT pnpm exec wrangler dev --local --persist-to ../.wrangler/state --port 18787

devtls:
	cd hack && go run ./devtls -listen :6443 -upstream http://127.0.0.1:18787 -dir ../.build/devtls

check:
	cd worker && pnpm exec wrangler types >/dev/null && pnpm exec tsc --noEmit

vet: mirrors
	go vet ./pkg/...
	GOOS=js GOARCH=wasm go vet ./pkg/...
	cd hack && go vet ./...

test: wasm
	go test -count=1 ./pkg/...

clean:
	rm -rf $(BUILD) $(ASSETS)
