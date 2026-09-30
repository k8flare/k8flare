# k8flare

A k3s-compatible Kubernetes control plane that runs on Cloudflare
Workers and Durable Objects. Stock `k3s agent` nodes join it the same
way they join a k3s server, and the cluster passes the full Kubernetes
Conformance suite.

The control plane costs the nodes nothing: the API server, controllers,
scheduler, datastore, and the add-ons that k3s runs as pods all run on
Cloudflare. A node runs only what has to be local, so a 1 vCPU / 1 GB
machine (a Linode Nanode) gives almost all of its resources to
workloads.

See [SECURITY.md](SECURITY.md) for the security model and how to report
a vulnerability.

## What you get

- **The Kubernetes API, complete.** Every group, version, resource, and
  subresource that kube-apiserver serves by default, with upstream
  validation, defaulting, and registry strategies. Watch with bookmarks
  and `sendInitialEvents`, paginated lists, field and label selectors,
  JSON / merge / strategic-merge patch, server-side apply with
  schema-aware managed fields, dry-run, Table output, JSON and protobuf.
- **Extensibility.** CustomResourceDefinitions with structural schemas,
  defaulting, CEL validation, status and scale subresources, and
  conversion webhooks. API aggregation through `APIService`. Mutating
  and validating admission webhooks, ValidatingAdmissionPolicy, and
  MutatingAdmissionPolicy.
- **Every default controller.** The kube-controller-manager set,
  including node lifecycle with `not-ready` and `unreachable` taints,
  taint-based eviction, attach/detach, the persistent-volume binder,
  and delayed requeues (progress deadlines, Job backoff, TTLs, CronJob
  schedules) that fire on time without further writes.
- **The scheduler.** The default kube-scheduler profile, including
  volume binding, preemption, and Dynamic Resource Allocation.
- **k3s server behaviour.** The supervisor protocol, `K10` join tokens
  with CA pinning, node passwords, packaged add-ons, `server/manifests`
  auto-deploy, and HelmChart resources.
- **Operations.** Token management, certificate and CA rotation,
  secrets encryption at rest, snapshots and restore, audit logging,
  and zero-downtime upgrades.
- **Cloudflare-native extras.** Cloudflare Access for `kubectl` sign-in,
  controllers and webhooks written as Workers, LoadBalancer Services on
  Cloudflare hostnames, snapshots in R2, and Workers observability.

## Architecture

Every Go component is compiled to WebAssembly and loaded on first use as
a Worker Loader dynamic worker, so a request pays only for the code it
needs. Cluster state lives in one Cluster Durable Object, a revisioned
key-value log with the semantics kube-apiserver expects from etcd.
Writes enqueue controller work on Cloudflare Queues; Durable Object
alarms carry every deadline, so nothing polls.

| Component | Package |
|---|---|
| Front Worker: authentication, supervisor endpoints, discovery, routing | `packages/apiserver`, `packages/control-plane-worker` |
| One worker per API group, sharing one installer and generic store | `packages/apiserver-<group>`, `packages/apiserver-group`, `packages/apiserver-installer`, `packages/apiserver-registry` |
| Authenticators, RBAC and Node authorizers | `packages/apiserver-auth`, `packages/apiserver-authz` |
| Admission chain, webhooks, admission policies | `packages/admission` |
| CRDs and custom resources | `packages/customresources` |
| OpenAPI v2 and v3 | `packages/openapi` |
| API aggregation | `packages/apiserver-apiregistration` |
| Datastore: `storage.Interface` over the Cluster Durable Object | `packages/apiserver-kine`, `packages/cluster-store` |
| k3s supervisor protocol, CA vault, node passwords | `packages/apiserver-supervisor` |
| Controllers | `packages/workloads`, `packages/gc`, `packages/hpa`, `packages/attachdetach` |
| Scheduler | `packages/scheduler` |
| Metrics API | `packages/metricsapi` |
| kubelet access (logs, exec, attach, port-forward, proxy) | `packages/node-tunnel` |
| kubectl table printers | `packages/printers`, `packages/printers-<group>` |
| Go ↔ Worker Loader bridge and bootstrap | `packages/worker-bridge`, `packages/loader-kit` |

The layout follows cloudflare/cloudflare-os: Worker configuration at the
root, one flat `packages/` directory, tooling under `scripts/`.

## Joining a node

A node joins with the stock k3s binary:

```
curl -sfL https://get.k3s.io | K3S_URL=https://<cluster-host> K3S_TOKEN=<K10 token> sh -s - agent
```

Cloudflare presents a serving certificate issued by the cluster's server
CA, so the agent pins the CA hash from the `K10` token exactly as it
does against a k3s server. Kubelet and kube-proxy client certificates
are verified at the edge with mutual TLS against the cluster's client
CA and passed to the Worker, so nodes authenticate as `system:node:<name>`
with certificates, not shared secrets.

Set up the hostname once. `k8flare edge-certificate --hosts <cluster-host>`
issues a serving certificate from the server CA; upload it with
`bundle_method: user_defined` (a private CA is accepted only in that mode).
Upload the client CA it also writes as a Cloudflare mTLS CA (bring your own
CA, Enterprise) and associate it with the hostname. Do not add the WAF rule
that blocks requests without a verified certificate: bearer-token clients
and nodes that have not yet been issued a certificate present none. The
Worker trusts only `request.cf.tlsClientAuth`, and the apiserver verifies
the forwarded certificate against the client CA again. The agent needs
`--disable-apiserver-lb` (or `disable-apiserver-lb: true` in its config),
because the local load balancer makes the agent dial `127.0.0.1`, which
sends no SNI for the edge to route on.

The agent reaches the kubelet API through a remotedialer tunnel held by
a per-node Durable Object, so nodes need no public address or inbound
port. `kubectl logs`, `exec`, `attach`, and `port-forward` stream over
WebSocket, the protocol kubectl uses by default.

## Packaged components

Like k3s, the cluster ships its add-ons and deploys them on first start.
Anything in the cluster's manifests bucket is applied the way k3s
applies `/var/lib/rancher/k3s/server/manifests`, and `HelmChart`
resources are installed by the Helm controller. Each add-on can be
turned off in the cluster configuration, as `--disable` does for k3s.

Add-ons run on Cloudflare unless they must touch the node:

| Component | Runs on | Notes |
|---|---|---|
| Helm controller, manifest deployer | Cloudflare | Worker controllers |
| metrics-server | Cloudflare | `metrics.k8s.io` served from kubelet summaries |
| Service load balancer | Cloudflare | `type: LoadBalancer` gets a Cloudflare hostname; see below |
| Ingress | Cloudflare | `Ingress` and Gateway API routed at the edge instead of an in-cluster Traefik |
| Network policy | Node | Enforced by the agent's embedded controller |
| CoreDNS | Node | One small replica serving `10.43.0.10`, answering from the control plane's view of Services and Endpoints |
| local-path provisioner | Node | Default `local-path` StorageClass with `WaitForFirstConsumer`; helper pods run only while a volume is created or deleted |

A node therefore runs containerd, the kubelet, flannel, kube-proxy, and
CoreDNS, and nothing else from the platform.

## Operations

- **Tokens.** Create, list, rotate, and revoke join tokens, including
  short-lived bootstrap tokens, from `k8flare token`. A created token is printed as
  `K10<server CA hash>::<id>.<secret>`, the string `K3S_TOKEN` takes; it
  optionally expires (`--ttl`, default 24h, `0` never), is
  stored hashed in the vault, and is accepted next to `JOIN_TOKEN`.
- **Certificates.** Leaf certificates rotate automatically before
  expiry. The server and client CAs rotate on demand without
  re-joining nodes, as `k3s certificate rotate-ca` does.
- **Secrets encryption.** Secrets are encrypted at rest with a key
  held outside the datastore. Keys rotate with `k8flare secrets-encrypt`:
  prepend a new key to the `SECRETS_ENCRYPTION_KEYS` Worker secret, redeploy,
  run `k8flare secrets-encrypt reencrypt`, then check
  `k8flare secrets-encrypt status` for `0 stale` before removing the old key.
- **Snapshots and restore.** Scheduled and on-demand snapshots go to R2,
  exclude key material, and restore into the same or a new cluster.
  Point-in-time recovery covers the last 30 days. `k8flare snapshot
  save|list|restore` manages snapshots; restoring one into a cluster that
  already holds data needs `--force` and replaces its contents (the vault
  and its keys are never touched, and encrypted Secrets need the same
  `SECRETS_ENCRYPTION_KEYS`). `k8flare restore --to <time>` does
  point-in-time recovery.
- **Audit log.** Every request is recorded under an audit policy and
  shipped to Workers Logs or any Logpush destination.
- **Health.** `/livez`, `/readyz`, and `/healthz` report the datastore,
  queues, and controllers, with per-check detail like kube-apiserver.
- **Upgrades.** A new version is deployed with gradual rollout; storage
  migrations run in place, and nodes keep running across the upgrade.

## Security

- Authentication: client certificates, ServiceAccount tokens bound to
  their Pod or Secret with audience checks, bootstrap tokens, OIDC, and
  Cloudflare Access. Anonymous requests get `system:anonymous` with the
  same public-info access as upstream.
- Authorization: Node and RBAC authorizers, with RBAC escalation and
  bind checks on every Role and binding write.
- Admission: the upstream default plugin chain, including
  NodeRestriction and Pod Security Admission.
- Internal traffic between Workers runs over service bindings with
  per-component identities, not a shared admin credential.
- Rate limiting and API Priority and Fairness protect the datastore.

## Cloudflare features

- **Access sign-in.** `kubectl` authenticates through Cloudflare Access;
  Access groups map to Kubernetes groups for RBAC. An exec credential
  plugin can hand `kubectl` the Access application token as its bearer token.
- **Workers as controllers.** Annotate a CRD with
  `k8flare.io/controller: <worker>` and changes to its resources are
  delivered to that Worker, which reconciles through the Kubernetes
  API. No pod, no node.
- **Workers as webhooks and API servers.** An admission webhook,
  conversion webhook, or `APIService` can point at a Worker instead of a
  Service: webhooks with the URL `https://k8flare.com/worker/<name>`, and
  any of them with the `k8flare.com/worker: <name>` annotation.
- **LoadBalancer Services on the edge.** `type: LoadBalancer` is
  published as `{name}--{namespace}.<cluster domain>` and routed to
  ready endpoints through the node tunnel, with TLS from Cloudflare.
- **Observability.** Workers Logs, traces for every request and
  controller pass, and cluster metrics without a monitoring stack on
  the nodes.

## Conformance

CI builds the workers, starts a control plane, joins stock k3s agents,
and runs the Kubernetes Conformance suite on every change. The full
suite also runs against a Cloudflare deployment, and every release
passes it.

## Local development

```
pnpm install
make mirrors
make wasm
make gen             # after a Kubernetes bump
make dev             # wrangler dev on :18787
make devtls          # https://localhost:6443 -> :18787
make kubeconfig      # .build/kubeconfig.yaml
make test            # Kubernetes Conformance against the local cluster
make test-packages   # package tests
make e2e SET=required
```

Copy `.dev.vars.example` to `.dev.vars` and put strong tokens there.
Production secrets go in `wrangler secret put`, never in git.

## License

Original k8flare code is [MIT](LICENSE). Kubernetes and k3s remain
Apache-2.0; see [NOTICE](NOTICE).
