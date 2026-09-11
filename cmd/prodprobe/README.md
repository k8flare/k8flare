# prodprobe — the synthetic convergence probe for a real deployment

Every other gate in this repo runs against `wrangler dev`. S30 is the
reason this one exists: the deployed control plane was broken from
2026-07-26 to 2026-09-09 while every local gate stayed green, because
`wrangler dev` does not reproduce the production runtime semantics that
broke (`docs/platform-verification.md` S30/S31).

One run, against the deployment you point it at:

1. `GET /readyz` must report every component ready.
2. Create a Deployment; a pod must reach `Running`.
3. Delete it; the pods must disappear.
4. The account's analytics must show **zero** Worker and Durable Object
   requests over a quiet window afterwards — cost invariants #1/#3.

## Run it

```
go run ./cmd/prodprobe \
  -url https://k8flare.example.workers.dev \
  -token "$CLUSTER_TOKEN" \
  -account "$CLOUDFLARE_ACCOUNT_ID" \
  -api-token "$CLOUDFLARE_ANALYTICS_TOKEN"
```

Takes ~25 minutes, most of it sleeping: the cluster is given time to
quiesce, the quiet window has to elapse, and Cloudflare's analytics
datasets lag ingestion by a few minutes. `-h` lists every timeout.

Convergence only, no analytics credentials needed (also what the local
smoke uses):

```
go run ./cmd/prodprobe -url http://127.0.0.1:8787 -token k8flare-dev-token -parking=false
```

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
`/readyz` reports every component, refuses an unauthenticated caller,
goes 503 while `/healthz` stays cheap when the Loader path is broken, and
that the probe both passes against a converging cluster and fails against
one where nothing can schedule. It is a test of this tooling, **not** of
production.
