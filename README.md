# k8flare

A Kubernetes control plane that runs on Cloudflare Workers and Durable
Objects, joined by unmodified k3s agents.

**Status: pre-production.** One maintainer, no tagged releases, APIs and
storage layout may change. A deployment is a single cluster for trusted
operators. See [SECURITY.md](SECURITY.md).

The layout follows cloudflare/cloudflare-os: Worker configuration at the
root, one flat `packages/` directory, tooling under `scripts/`.

Every Go component is a Worker Loader dynamic worker loaded on first use,
so a request pays only for the binary it needs. Cluster state lives in
one Cluster Durable Object. Writes enqueue work on Cloudflare Queues.
There are no cron triggers. Durable Object alarms are booked only at a
known deadline (watch lease expiry, node-lease check).

- `packages/apiserver` — the front: bearer-token authentication, the k3s
  supervisor endpoints, root discovery, and routing. `/api/*` and
  `/apis/<group>/*` go to that group's worker, unknown groups to
  `customresources`, `/openapi/*` to `openapi`.
- `packages/apiserver-{core,coordination,discovery,node,storage,authentication,authorization,apps,policy,resource,rbac,batch}`
  — one worker per served API group. `packages/apiserver-group` is what
  they share. `packages/apiserver-core` holds the pod, node, and
  namespace specifics.
- `packages/apiserver-installer`, `packages/apiserver-registry`,
  `packages/apiserver-auth`, `packages/apiserver-authz` — the installer,
  the generic store, authenticators, and the RBAC authorizer.
- `packages/apiserver-kine` — `storage.Interface` over the Cluster
  Durable Object's revisioned key-value log.
- `packages/apiserver-supervisor` — the k3s supervisor protocol, the CA
  vault, and node passwords.
- `packages/customresources` — apiextensions.k8s.io and CRD-defined
  groups, behind the `CustomResources` entrypoint.
- `packages/openapi` — `/openapi/v2` and `/openapi/v3`, behind `OpenAPI`.
- `packages/scheduler` — kube-scheduler, behind `Scheduler`, woken by
  the `k8flare-scheduler` queue when a Pod has no node or a node changes.
- `packages/workloads` — ReplicaSet, Deployment, ReplicationController,
  Job, CronJob, DaemonSet, StatefulSet, Endpoints, EndpointSlice,
  ServiceAccount, root-CA publisher, and namespace deletion, behind
  `Workloads`. ServiceAccount provisioning and terminating namespaces
  use the `k8flare-accounts` queue so a long workloads batch cannot
  hold them.
- `packages/gc` — garbage collection, behind `GarbageCollector`.
- `packages/printers` and `packages/printers-{core,coordination,discovery,node,storage,apps,policy,resource,rbac,batch}`
  — kubectl table printers.
- `packages/worker-bridge` — Go `http.Handler` ↔ Worker Loader.
  `packages/loader-kit` — Loader bootstrap. `packages/control-plane-worker`
  — the Worker. `packages/cluster-store` — the Cluster Durable Object.
  `packages/node-tunnel` — kubelet access through a remotedialer session.
- `packages/agent` — the k3s agent, with one hook so it can write
  bearer-token kubeconfigs (TLS terminates at the edge).
- `scripts/` — `mirror` pins upstream modules into `.build/` with
  GOOS=js overlays, `genresources` / `genprinters` / `genopenapi`
  regenerate tables, `wasmpack` prepares Static Assets, `devtls`
  terminates TLS in front of `wrangler dev`.

## Local development

```
pnpm install
make mirrors
make wasm
make gen             # after a Kubernetes bump
make test
make dev             # wrangler dev on :18787
make devtls          # https://localhost:6443 -> :18787
make e2e SET=required
```

Copy `.dev.vars.example` to `.dev.vars` and put strong tokens there.
Production secrets go in `wrangler secret put`, never in git.

`make e2e` needs a joined Ready node. Cluster DNS and kube-proxy are not
served; use `dnsPolicy: Default` for pods that must resolve off-cluster.

### Joining a node

Build the agent with `make agent`. The machine needs a stock `k3s`
binary run once (it unpacks containerd, runc, and CNI plugins) and must
not be on WARP. `kubectl logs` reaches the kubelet through the NodeTunnel
Durable Object, so the node does not need a public address.

OrbStack example:

```
orb -m <vm> sudo update-ca-certificates   # after copying .build/devtls/ca.crt
orb -m <vm> sudo systemd-run --unit k8flare-agent --collect --property=KillMode=mixed \
  /usr/local/bin/k8flare-agent --server https://host.orb.internal:6443 --token <JOIN_TOKEN> \
  --node-name <vm>
```

Any Linux host can run the same `k8flare-agent` binary against
`https://<worker>` with the join token.

### kubectl

```
make kubeconfig
export KUBECONFIG=$PWD/.build/kubeconfig.yaml
kubectl get nodes
```

## License

Original k8flare code is [MIT](LICENSE). Kubernetes and k3s remain
Apache-2.0; see [NOTICE](NOTICE).
