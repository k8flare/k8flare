# k8flare

A Kubernetes control plane that runs on Cloudflare Workers and Durable
Objects, joined by unmodified k3s agents.

- `pkg/apiserver` — the apiserver: k8s.io/apiserver's own API installer
  over `genericregistry.Store`, with `KineStorage` adapting the store to the
  Cluster Durable Object, the k3s supervisor protocol the agent joins
  through, and node identity. Runs as a Go WASM Loader dynamic worker.
- `pkg/wasmhttp` — the bridge between a Go `http.Handler` and the Worker
  Loader bootstrap, with streaming responses and WebSocket clients.
- `worker/` — the one Worker: routing, the Loader bootstrap, and the
  Cluster Durable Object (a revisioned key-value log with watch fan-out).
- `cmd/agent` — the k3s agent, embedded unchanged but for one hook that
  lets it write bearer-token kubeconfigs (TLS terminates at the edge, so
  client certificates never reach the control plane).
- `hack/` — build tools: `mirror` copies pinned upstream modules into
  `.build/` with sha256-pinned overlays that make them build for GOOS=js,
  `wasmpack` prepares the binary for Static Assets, `devtls` terminates TLS
  in front of `wrangler dev`.

## Local development

```
pnpm install --dir worker
make wasm            # mirrors + Go WASM + wasm-opt + chunking (size is printed; cap 64MiB)
make test            # client-go tests against a wrangler dev the tests start themselves
make dev             # wrangler dev on :18787 (see the Makefile for why CLAUDECODE is unset)
make devtls          # https://localhost:6443 -> :18787, CA in .build/devtls/ca.crt
```

Tokens for dev live in `worker/.dev.vars` (copy `.dev.vars.example`).

### Joining a node (OrbStack VM)

```
make agent
orb -m <vm> sudo update-ca-certificates   # after copying .build/devtls/ca.crt to /usr/local/share/ca-certificates/
orb -m <vm> sudo systemd-run --unit k8flare-agent --collect --property=KillMode=mixed \
  /usr/local/bin/k8flare-agent --server https://host.orb.internal:6443 --token <JOIN_TOKEN> \
  --node-name <vm> --kubelet-plain-port 10255
```

The VM needs the stock `k3s` binary run once (it unpacks containerd, runc
and the CNI plugins the agent uses), and must not be connected to WARP.
`--kubelet-plain-port` exists because `kubectl logs` reaches the kubelet
through a Worker fetch that cannot verify the kubelet's certificate.

### kubectl

```
kubectl --server https://localhost:6443 --certificate-authority .build/devtls/ca.crt --token <ADMIN_TOKEN> get nodes
```

Pods need `spec.nodeName`: there is no scheduler yet. Services, cluster
DNS, and kube-proxy are not served yet either; use `dnsPolicy: Default`.
