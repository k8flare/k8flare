# k8flare

A Kubernetes control plane that runs on Cloudflare Workers and Durable
Objects, joined by unmodified k3s agents.

The layout follows cloudflare/cloudflare-os: Worker configuration at the
root, one flat `packages/` directory, tooling under `scripts/`, plans under
`plans/`.

- `packages/apiserver` — the apiserver entry: k8s.io/apiserver's own API
  installer over the stores, authentication, and the WASM entrypoint. Runs
  as a Go WASM Loader dynamic worker.
- `packages/apiserver-registry` — one generic `genericregistry.Store` per
  served resource; the served resources are generated from upstream's
  discovery documents (`scripts/genresources`).
- `packages/apiserver-kine` — `storage.Interface` over the Cluster
  Durable Object's revisioned key-value log.
- `packages/apiserver-supervisor` — the k3s supervisor protocol the agent
  joins through, the CA vault, and node passwords.
- `packages/worker-bridge` — the bridge between a Go `http.Handler` and the
  Worker Loader bootstrap, with streaming responses and WebSocket clients.
- `packages/control-plane-worker` — the Worker: routing and the Loader
  bootstrap, which hands the Cluster Durable Object's stub to the dynamic
  worker. `packages/cluster-store` — the Cluster Durable Object.
- `packages/agent` — the k3s agent, embedded unchanged but for one hook
  that lets it write bearer-token kubeconfigs (TLS terminates at the edge,
  so client certificates never reach the control plane).
- `scripts/` — `mirror` copies pinned upstream modules into `.build/` with
  sha256-pinned overlays that make them build for GOOS=js, `genresources`
  writes the served-resource table, `wasmpack` prepares the binary for
  Static Assets, `devtls` terminates TLS in front of `wrangler dev`.

## Local development

```
pnpm install
make wasm            # mirrors + Go WASM + wasm-opt + chunking (size is printed; cap 64MiB)
make gen             # regenerate the served-resource table after a Kubernetes bump
make test            # client-go tests against a wrangler dev the tests start themselves
make dev             # wrangler dev on :18787 (see the Makefile for why CLAUDECODE is unset)
make devtls          # https://localhost:6443 -> :18787, CA in .build/devtls/ca.crt
```

Tokens for dev live in `.dev.vars` next to `wrangler.jsonc` (copy
`.dev.vars.example`).

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

With `make dev` and `make devtls` running:

```
make kubeconfig                                  # writes .build/kubeconfig.yaml from .dev.vars and the devtls CA
export KUBECONFIG=$PWD/.build/kubeconfig.yaml
kubectl get nodes
kubectl apply -f pod.yaml --validate=false       # no OpenAPI is served yet, hence --validate=false
kubectl logs <pod>
```

Pods need `spec.nodeName`: there is no scheduler yet. Services, cluster
DNS, and kube-proxy are not served yet either; use `dnsPolicy: Default`.
