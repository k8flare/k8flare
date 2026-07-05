#!/usr/bin/env bash
# Cross-compiles the unmodified k3s agent embed (cmd/agent) for
# linux/amd64 into workers/nodes/images/node/, where the node-image
# Dockerfile COPYs it -- Cloudflare Containers run amd64, and building
# the full k8s tree under qemu on ARM Macs would take 20+ minutes,
# so the Go build happens on the host (S15 round-2 lesson).
# Run before `wrangler deploy --config workers/nodes/wrangler.jsonc`.
set -euo pipefail
cd "$(dirname "$0")/.."
bash scripts/gen-k8s-js-mirror.sh >/dev/null
bash scripts/gen-clientgo-lean-mirror.sh >/dev/null
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o workers/nodes/images/node/k8flare-agent ./cmd/agent
echo "workers/nodes/images/node/k8flare-agent: $(wc -c < workers/nodes/images/node/k8flare-agent | tr -d ' ') bytes"
