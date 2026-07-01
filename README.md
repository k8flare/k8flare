# k8flare

Kubernetes control plane on Cloudflare Workers.

Runs a minimal K8s API server as a Cloudflare Worker with Durable Objects (SQLite-backed etcd-like storage), enabling `kubectl` to manage workloads without traditional infrastructure.

## Architecture

```
kubectl → Cloudflare Worker (Go WASM + TypeScript)
              ↓
         Durable Object (Etcd) — SQLite-backed K8s state store
              ↓
         k3s Agent (EC2) — kubelet + containerd + flannel
```

**Components:**

- **Worker** — TypeScript routing layer + Go WASM K8s API server
- **Etcd** — Durable Object with SQLite, implements kine-compatible storage
- **Agent** — k3s agent binary with Cloudflare-specific adaptations (CA replacement, token auth, flannel bypass)

## Quick Start

### From Release

```bash
# Download the latest release
gh release download -R k8flare/k8flare -p 'k8flare-worker-*.tar.gz'
tar xzf k8flare-worker-*.tar.gz
cd k8flare-worker

# Set your cluster token
npx wrangler secret put K3S_TOKEN

# Deploy
npx wrangler deploy
```

### From Source

```bash
git clone https://github.com/k8flare/k8flare.git
cd k8flare
npm install

# Build Go WASM
npm run build:wasm

# Deploy
npm run deploy
```

### Agent Setup

Deploy the k3s agent on an EC2 instance (or any Linux server):

```bash
# Download agent binary
gh release download -R k8flare/k8flare -p 'k8flare-agent-*'
chmod +x k8flare-agent-linux-*

# Run agent
./k8flare-agent-linux-arm64 \
  --server https://your-k8flare.workers.dev \
  --token YOUR_K3S_TOKEN
```

See [scripts/ec2-user-data.sh](scripts/ec2-user-data.sh) for automated EC2 bootstrap.

## Configuration

### Worker Environment Variables

| Variable    | Required | Description                                                        |
| ----------- | -------- | ------------------------------------------------------------------ |
| `K3S_TOKEN` | Yes      | Cluster authentication token. Generate with `openssl rand -hex 32` |

### Optional: VPC Service (for kubelet proxy)

To enable `kubectl logs` and `kubectl exec`, set up a Cloudflare Tunnel + VPC Service:

```bash
./scripts/setup-tunnel.sh
```

Then add the VPC binding to `packages/worker/wrangler.jsonc`:

```jsonc
"vpc_services": [{ "binding": "KUBELET_VPC", "service_id": "YOUR_SERVICE_ID" }]
```

## Development

```bash
npm install
npm run build:wasm    # Build Go WASM binary
npm run dev           # Start local dev server
npm run check         # Lint + format + typecheck (vp)
npm run test          # Run tests
go test ./pkg/...     # Run Go tests
```

## License

MIT
