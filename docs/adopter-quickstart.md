# Adopter quickstart

Read this before deciding to run k8flare. It answers the questions the
rest of the docs assume you already know, and states the limits plainly.

**Status: pre-production.** One maintainer, no tagged releases, APIs and
storage layout may change without migration. Nothing here is a support
commitment.

## What you need from Cloudflare

k8flare is not portable: it is built on Cloudflare primitives that have
no equivalent elsewhere. Before you start, check your account can use
all of these.

| Primitive | Used for | Status |
|---|---|---|
| **Worker Loader** (dynamic workers) | Runs every control-plane binary (apiserver, kcm, gc, scheduler, cluster operator). **Nothing works without it.** | Check availability for your account — this is the hard gate |
| Durable Objects (SQLite) | All cluster state, watch fan-out | GA. 10 GB per DO, ~1,000 req/s soft ceiling, 30-day point-in-time recovery |
| DO Facets | Per-namespace storage isolation | Open beta |
| Static Assets | WASM chunks, OpenAPI/discovery documents | GA |
| Containers | Pod-on-Containers NodeVMs — **optional**, omit the `containers` section if you use BYO-VM nodes only | Billed by wall clock |
| VPC / Cloudflare Mesh | Optional node networking | Beta |

You also need a paid Workers plan; the free plan does not cover Durable
Objects.

## What you must change before deploying

`packages/k8flare-worker/wrangler.jsonc` ships with example values.

| Value | Why it matters |
|---|---|
| `vars.GATEWAY_URL` | The public URL nodes dial, and the origin baked into every kubeconfig the operator mints. Leave it pointing at someone else's deployment and **your nodes join their control plane**. |
| `name` | The Worker name claimed in your account. |
| `containers[].authorized_keys` | Empty by default. Add your own SSH key only to debug NodeVMs. |

Then deploy, and set the one secret:

```sh
wrangler deploy -c packages/k8flare-worker/wrangler.jsonc
openssl rand -hex 24 | wrangler secret put K3S_TOKEN --name <your-worker>
```

Deploy first. `wrangler secret put` against a Worker that does not exist
yet prompts to create one (*"There doesn't seem to be a Worker called
… Do you want to create a new Worker with that name?"*), which a piped
`openssl` cannot answer.

That ordering has a consequence you have to plan for: **between the
deploy and the secret, your Worker is live and accepts the publicly
documented token `k8flare-dev-token`** — on a public `*.workers.dev` URL
that is a world-writable Kubernetes API. Set the secret immediately, and
do not hand out the URL until you have.

## Upgrading: check the migrations block first

`packages/k8flare-worker/wrangler.jsonc` carries a Durable Object
`migrations` array. `wrangler deploy` silently applies every tag you have
not applied yet, and a tag containing `deleted_classes` **destroys that
Durable Object's storage** — which is where all your cluster state lives.

This is not hypothetical: two of the four tags in that block are
destructive (`v2` drops the pre-consolidation `Etcd` class, `v3` deletes
every current class). Both were deliberate pre-production wipes, and a
fresh deployment replays them harmlessly because there is nothing to
lose. An existing deployment pulling a *future* destructive tag would
lose everything, and there is no backup mechanism.

So before every upgrade:

```sh
git diff HEAD..origin/main -- packages/k8flare-worker/wrangler.jsonc
```

If a new tag appeared, read it before deploying. If you want the decision
to be yours rather than the upstream branch's, pin a commit or fork.

## Monitoring

`/healthz`, `/livez` and `/readyz` answer `200 ok` **without a token**, so
an external uptime monitor can reach them.

Be clear on what that asserts: the Worker is routable and its script
loaded. It is answered in the Worker shell and deliberately does *not*
touch storage or load any control-plane component — an unauthenticated
path that spun up a 65MB WASM module per request would be a cost
amplifier on a public URL. For "is the API actually serving", probe a
real endpoint with a token:

```sh
curl -sf -H "Authorization: Bearer $TOKEN" https://<your-worker>/api/v1/namespaces
```

## Security posture (read this before trusting it with anything)

- **Cluster tokens are `system:masters` and bypass RBAC entirely**
  (`pkg/apiserver/auth.go`, `pkg/apiserver/rbac.go`). The bundled
  `k8flare:cluster-admin` roles only constrain ServiceAccount identities.
- **The node-join credential is the same token.** Every BYO VM you join
  is handed cluster-admin. Do not join nodes you do not control.
- **Revocation lags up to ~60 s** (per-isolate token cache). Emergency
  revocation is deleting the cluster.
- Not supported yet: admission webhooks, CRDs, dynamic PV provisioning
  (PVCs stay `Pending`).
- CronJobs do fire on a parked cluster: the control plane arms an alarm
  for the next schedule and wakes cold to run it
  ([S27](platform-verification.md)). Each fire costs one wake-up, so a
  `* * * * *` schedule is not an idle cluster — see
  [cost-model.md](cost-model.md).

## What it costs

Honest state: **no deployed cluster has been billed yet.** The figures in
[cost-model.md](cost-model.md) are modeled from published unit prices,
not observed invoices. What *is* mechanically verified — by
`.github/workflows/cost-gate.yml` against a real local stack — is the
*behaviour* the cost model depends on: an idle cluster arms no alarms,
runs no processes, and produces no Worker invocations; and a cluster with
work it can never finish (a Deployment with no nodes to schedule on)
settles to a near-zero write rate instead of billing rows forever.

Rules of thumb:

- **Idle cluster:** storage plus a Worker Loader charge of
  $0.002/unique-binary/day while a cluster is active. No compute.
- **Active:** Workers CPU time (I/O wait is free), DO requests and
  rows read/written.
- **Pod-on-Containers:** billed by wall clock. A permanently-busy
  container node measured ~$58/month — more than an equivalent VPS. Use
  BYO-VM nodes unless you specifically want per-Pod microVMs.

## Backup and exit

**There is no backup mechanism.** State lives in Durable Objects; your
safety nets are:

- Cloudflare's 30-day point-in-time recovery on DO SQLite.
- `kubectl get -o yaml` export of your objects. Script and rehearse this
  — it is also your migration path: your workloads are plain Kubernetes
  objects, so they apply onto EKS/GKE/k3s unchanged, and BYO-VM nodes run
  an unmodified k3s agent. You would lose the control plane, not the
  cluster contents.

## Where to go next

- [admin-guide.md](admin-guide.md) — operating it: issuing clusters,
  handing out credentials, rotation, teardown *(Japanese)*
- [user-guide.md](user-guide.md) — for people you hand a kubeconfig to
  *(Japanese)*
- [development.md](development.md) — local dev loop, test lanes, pitfalls
- [cost-model.md](cost-model.md) — per-component cost estimates and the
  measured idle behaviour
- [platform-verification.md](platform-verification.md) — every platform
  spike and measured finding, including the problems still open
