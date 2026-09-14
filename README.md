# k8flare

A Kubernetes control plane that runs on Cloudflare Workers and Durable
Objects, joined by unmodified k3s agents.

The layout follows cloudflare/cloudflare-os: Worker configuration at the
root, one flat `packages/` directory, tooling under `scripts/`, plans under
`plans/`.

Every Go component is a Worker Loader dynamic worker loaded on first use,
so a request pays only for the binary it needs:

- `packages/apiserver` — the front: bearer-token authentication, the k3s
  supervisor endpoints, root discovery, and routing. `/api/*` and
  `/apis/<group>/*` go to that group's worker, unknown groups to
  `customresources`, `/openapi/*` to `openapi`.
- `packages/apiserver-{core,coordination,discovery,node,storage,authentication,authorization,apps,policy,resource,rbac,batch}`
  — one worker per served API group: k8s.io/apiserver's API installer over
  generic stores for that group only, so each links only its own types.
  The group list and each group's scheme registration are generated from
  upstream's discovery documents (`scripts/genresources`).
  `packages/apiserver-group` is what they share; `packages/apiserver-core`
  also holds the pod, node, and namespace specifics and wakes the
  scheduler when a Pod without a node is written.
- `packages/apiserver-installer`, `packages/apiserver-registry`,
  `packages/apiserver-auth`, `packages/apiserver-authz` — the installer,
  the generic store with its per-resource hooks, the authenticators and
  request filters, and the RBAC authorizer built from kine-backed roles
  and bindings unioned with upstream's bootstrap policy.
- `packages/apiserver-kine` — `storage.Interface` over the Cluster
  Durable Object's revisioned key-value log.
- `packages/apiserver-supervisor` — the k3s supervisor protocol the agent
  joins through, the CA vault, and node passwords.
- `packages/customresources` — apiextensions.k8s.io and every CRD-defined
  group, served by upstream's CRD handler and controllers (naming,
  establishing, discovery, finalizer) behind the `CustomResources`
  entrypoint.
- `packages/openapi` — the /openapi/v2 and /openapi/v3 documents, computed
  by kube-openapi from the same routes behind the `OpenAPI` entrypoint.
- `packages/scheduler` — the real kube-scheduler behind the `Scheduler`
  entrypoint, started on the first wake-up and pumped for a bounded window
  per wake-up.
- `packages/controllers` — sixteen of kube-controller-manager's controllers
  (replication, replicaset, deployment, daemonset, statefulset, job,
  cronjob, endpoints, endpointslice, nodeipam, nodelifecycle,
  tainteviction, serviceaccount, root-ca-cert-publisher, namespace,
  garbagecollector) behind the `Controllers` entrypoint, woken by writes to the
  resources they reconcile and held only while their workqueues are busy.
- `packages/printers` and `packages/printers-{core,coordination,discovery,node,storage,apps,policy,resource}`
  — upstream's `kubectl get` printers, one worker per API group.
- `packages/worker-bridge` — the bridge between a Go `http.Handler` and the
  Worker Loader bootstrap, with streaming requests, responses and
  WebSocket clients. `packages/loader-kit` — the Loader bootstrap and chunk
  assembly. `packages/control-plane-worker` — the Worker: the default fetch
  and the entrypoints above. `packages/cluster-store` — the Cluster
  Durable Object.
- `packages/agent` — the k3s agent, embedded unchanged but for one hook
  that lets it write bearer-token kubeconfigs (TLS terminates at the edge,
  so client certificates never reach the control plane).
- `scripts/` — `mirror` copies pinned upstream modules into `.build/` with
  sha256-pinned overlays that make them build for GOOS=js (no etcd, no
  gRPC egress, no APF controller, a clientset scheme that registers nothing
  so each worker links only the groups it imports, and a clientset and
  informer factory narrowed to the groups the scheduler uses),
  `genresources` writes the served-resource table and the per-group
  packages, `genprinters` extracts the kubectl
  table printers per API group, `genopenapi` prunes upstream's OpenAPI
  model definitions to the served kinds, `wasmpack` prepares the binary for
  Static Assets, `devtls` terminates TLS in front of `wrangler dev`.

## Local development

```
pnpm install
make mirrors         # go.mod points k8s.io/{apiserver,client-go,kubernetes,apiextensions-apiserver} at .build/*-mirror; every make target runs this first
make wasm            # mirrors + Go WASM + wasm-opt + chunking (size is printed; cap 64MiB); wasm-opt is skipped for binaries whose Go output is unchanged
make gen             # regenerate the served-resource table, printers and OpenAPI models after a Kubernetes bump
make test            # client-go tests against a wrangler dev the tests start themselves
make dev             # wrangler dev on :18787 (see the Makefile for why CLAUDECODE is unset)
make devtls          # https://localhost:6443 -> :18787, CA in .build/devtls/ca.crt
make e2e SET=required   # upstream e2e.test via ginkgo (PROCS=4 parallel; [Serial] specs run alone); needs make dev, make devtls, a joined node
```

Tokens for dev live in `.dev.vars` next to `wrangler.jsonc` (copy
`.dev.vars.example`).

### Joining a node (OrbStack VM)

```
make agent
orb -m <vm> sudo update-ca-certificates   # after copying .build/devtls/ca.crt to /usr/local/share/ca-certificates/
orb -m <vm> sudo systemd-run --unit k8flare-agent --collect --property=KillMode=mixed \
  /usr/local/bin/k8flare-agent --server https://host.orb.internal:6443 --token <JOIN_TOKEN> \
  --node-name <vm>
```

The VM needs the stock `k3s` binary run once (it unpacks containerd, runc
and the CNI plugins the agent uses), and must not be connected to WARP.
`kubectl logs` reaches the kubelet through the NodeTunnel Durable Object's
remotedialer session, not the VM's address, so the VM does not need to be
reachable from the Worker.

### kubectl

With `make dev` and `make devtls` running:

```
make kubeconfig                                  # writes .build/kubeconfig.yaml from .dev.vars and the devtls CA
export KUBECONFIG=$PWD/.build/kubeconfig.yaml
kubectl get nodes
kubectl apply -f pod.yaml
kubectl logs <pod>
```

Pods need `spec.nodeName`: there is no scheduler yet. Services, cluster
DNS, and kube-proxy are not served yet either; use `dnsPolicy: Default`.

### A stock k3s next to it (comparison)

A second OrbStack VM runs an unmodified k3s server for side-by-side checks:

```
orb create ubuntu:noble k3s-vanilla
orb -m k3s-vanilla bash -c 'curl -sfL https://get.k3s.io | sudo sh -s - server --write-kubeconfig-mode 644'
orb -m k3s-vanilla cat /etc/rancher/k3s/k3s.yaml | sed "s#127.0.0.1#$(orb -m k3s-vanilla hostname -I | cut -d' ' -f1)#" > .build/kubeconfig-k3s.yaml
KUBECONFIG=$PWD/.build/kubeconfig-k3s.yaml kubectl get nodes
```

Both kubeconfigs live under `.build/`: `kubeconfig.yaml` for k8flare,
`kubeconfig-k3s.yaml` for the stock cluster.
