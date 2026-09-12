# prodprobe — the synthetic convergence probe for a real deployment

Every other gate in this repo runs against `wrangler dev`. S30 is the
reason this one exists: the deployed control plane was broken from
2026-07-26 to 2026-09-09 while every local gate stayed green, because
`wrangler dev` does not reproduce the production runtime semantics that
broke (`docs/platform-verification.md` S30/S31).

One run, against the deployment you point it at:

1. `GET /readyz?verbose=true` must report every component ready.
2. Create a Deployment; the controllers must act on it. By default that
   means the Pod objects exist and the Deployment's own status catches up --
   compute is NOT required, so a cluster with no nodes still passes. Pass
   `-require-running` to demand `Running` instead, which needs a node.
   The default is deliberate: S30's measured symptom was a control plane that
   served the first list and then ignored `kubectl scale`, which is a
   controller failure, and coupling the daily probe to whether compute exists
   made it fail for an unrelated reason (S39's correction: Pod-on-Containers
   is currently broken in production).
3. Scale it to 2; the second pod must reach `Running` too. This step is
   not redundant: S30's measured symptom was a control plane that served
   the first list and then ignored `kubectl scale` for 3+ minutes, which
   a probe that only ever creates one Deployment would call healthy.
4. Delete it; the pods must disappear.
5. Any node the probe's own pods demand-started must detach first. A
   NodeVM outlives the workload that caused it -- measured 2026-09-11
   against production, two nodes were still attached the moment the
   deployment was gone and had detached about two minutes later. Waiting
   is bounded by `-node-drain-timeout` (default 10m); a node that never
   leaves is reported as a failure, because that is a real finding.
6. Without `-parking`, a weaker check still runs: the cluster's own
   resourceVersion must not move across the quiet window. It needs only the
   cluster token, so it works with no Cloudflare credential. It is a **proxy**
   — it proves nothing *wrote*, not that nothing *ran*, so an alarm chain that
   wakes and does no work is invisible to it. It does catch the two shapes that
   have actually bitten: a control plane that never stops writing (S26's no-op
   write storm) and one that keeps reconciling after the workload is gone.
   Measured in production 2026-09-11: revision 188 unchanged over 2 minutes.
7. With `-parking`, the account's analytics must show **zero** Worker and Durable Object
   requests over a quiet window afterwards — cost invariants #1/#3.

## Run it

```
go run ./cmd/prodprobe \
  -url https://k8flare.example.workers.dev \
  -token "$CLUSTER_TOKEN" \
  -compute "" \
  -account "$CLOUDFLARE_ACCOUNT_ID" \
  -api-token "$CLOUDFLARE_ANALYTICS_TOKEN"
```

Takes ~25 minutes, most of it sleeping: the cluster is given time to
quiesce, the quiet window has to elapse, and Cloudflare's analytics
datasets lag ingestion by a few minutes. `-h` lists every timeout.

Convergence only, no analytics credentials needed (also what the local
smoke uses):

```
go run ./cmd/prodprobe -url http://127.0.0.1:8787 -token k8flare-dev-token \
  -parking=false -compute ""
```

(`-compute ""` because `wrangler dev` without Docker has no Containers
backend; the pod then needs whatever nodes are attached.)

## Secrets a maintainer must set

Repository secrets, read by `.github/workflows/prod-probe.yml`. With any
of them absent the workflow logs a notice and skips — a fork or an
outside contributor never sees a red run they cannot fix.

| Secret | What it is |
| --- | --- |
| `K8FLARE_PROBE_URL` | Base URL of the deployment to probe, e.g. `https://k8flare.example.workers.dev`. Point it at a canary deployment, not at one carrying user workloads: step 4 asserts the whole Worker script is idle. |
| `K8FLARE_PROBE_TOKEN` | A cluster token for that deployment (`/clusters` bootstrap kubeconfig, or the `K3S_TOKEN` secret's value for the default cluster). |
| `CLOUDFLARE_ACCOUNT_ID` | The account the Worker is deployed in. |
| `CLOUDFLARE_ANALYTICS_TOKEN` | An API token with **Account Analytics: Read**. Nothing else — this token never needs write scope, and must not be the deploy token. |

Optional repository *variable* `K8FLARE_PROBE_SCRIPT`: the Worker script
name as analytics sees it. Defaults to `k8flare`
(`packages/k8flare-worker/wrangler.jsonc`'s `name`).

## The node precondition

Step 4 asserts zero requests. A BYO node's kubelet heartbeats every ~10
seconds, so a cluster with a node attached can never be quiet — S30's own
production measurement deleted the node before it saw 12 quiet minutes.
So the probe defaults to `-compute containers`: the pod annotation that
routes it to the demand-started Pod-on-Containers backend
(`pkg/apiserver/computeclass.go`), where the node is booted for the pod
and torn down with it.

If any `Node` object still exists when the quiet window is about to
start, the probe fails and says so rather than measuring something it
cannot interpret. Against a deployment with permanently attached nodes,
run with `-parking=false` and understand that idle cost is then
unverified.

## What has NOT been verified

**The GraphQL query in `analytics.go` has never been run against a real
Cloudflare account.** It was written against the dataset names S30 used
(`workersInvocationsAdaptive`,
`durableObjectsInvocationsAdaptiveGroups`) but the exact field and filter
shape is unverified — no credentials were available to the author.

The probe is built so that a wrong query cannot produce a false green:
before it trusts a quiet window, it queries the window in which it drove
the Deployment itself and requires a non-zero request count there. A
query that returns nothing fails as an *instrument error*, naming itself
as the suspect. The first maintainer run should still be a
`workflow_dispatch` with eyes on the log.

Also unverified end to end: the `-compute containers` path (no
deployment was available to run it against), and the alerting step, which
needs a real failing run to exercise.

## Local smoke

```
bash cmd/prodprobe/smoke.sh
```

Starts its own `wrangler dev` (needs `make wasm` first), then checks that
`/readyz?verbose=true` reports every component, that an anonymous caller
gets a bare `ok` and no breakdown, that readiness goes 503 while
`/healthz` stays cheap when the Loader path is broken, and that the probe
both passes against a converging cluster and fails against one where
nothing can schedule. It is a test of this tooling, **not** of
production.

## Scheduling it, now that Actions is off

`.github/workflows/prod-probe.yml` ran this daily. GitHub Actions was switched
off on cost grounds (2026-09-12), so **that workflow no longer runs** and the
file is kept only as the specification of what a run must assert.

**Nothing schedules it.** A launchd wrapper existed briefly and was removed on
2026-09-13: it was 14KB of macOS-specific shell for a probe that had never once
run successfully, and its presence made the repository claim an observability
it did not have. Scheduling belongs to whoever operates a deployment, not to
this repository, and it is one line:

```sh
K8FLARE_PROBE_URL=https://your-deployment.workers.dev \
K8FLARE_PROBE_TOKEN=... \
  go run ./cmd/prodprobe -parking=false -compute ""
```

Add `CLOUDFLARE_ACCOUNT_ID` and `CLOUDFLARE_API_TOKEN` (Account Analytics:Read)
and drop `-parking=false` to also assert the idle-cost invariant. Put it behind
whatever your platform schedules with — cron, launchd, a CI runner you pay for,
a different machine. The only thing this repository asserts is what a run
checks, which is the rest of this document.

```sh
mkdir -p ~/.config/k8flare
cat > ~/.config/k8flare/probe.env <<'ENV'
K8FLARE_PROBE_URL=https://your-deployment.workers.dev
K8FLARE_PROBE_TOKEN=...
CLOUDFLARE_ACCOUNT_ID=...     # optional -- without it, convergence only
CLOUDFLARE_API_TOKEN=...      # optional -- Account Analytics:Read
ENV
chmod 600 ~/.config/k8flare/probe.env
```

Two things that bit the launchd attempt are worth carrying to whatever you
schedule it with. A scheduler runs the job with a bare `PATH` and no login
shell, so a toolchain under `$HOME` (mise, homebrew) is unreachable and the run
fails before it starts. And `go.mod`'s replace directives point into
`.build/*-mirror`, so a wiped `.build/` turns every scheduled run into an error
nobody reads — build the binary once, `go build -o /somewhere ./cmd/prodprobe`,
and schedule that.

