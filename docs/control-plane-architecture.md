# Scheduler and controller-manager on Cloudflare

Notes from investigating how much of a real Kubernetes control plane
(scheduler, controller-manager) k8flare currently has, and how the rest
could be implemented within Cloudflare's execution model. This reflects the
state as of the scheduling-conformance pass (node-selector/capacity/hostPort
predicates, scheduling Events, LimitRange enforcement, ServiceAccount
auto-provisioning, namespace cascading deletion).

> **Phase 6 note (2026-07-03):** the "What exists today" section below is a
> point-in-time snapshot from before the v2 rearchitecture (single `Etcd`
> Durable Object, hand-written TypeScript scheduler) and is kept as a
> historical record, not a current description — everything in it has since
> been superseded: the DO is now `Cluster` (`workers/storage`, with
> per-namespace Durable Object Facets, see
> [`docs/multi-tenancy-and-hosting.md`](multi-tenancy-and-hosting.md)), and
> real `kube-scheduler`/`kube-controller-manager` binaries replaced the
> hand-written scheduler/reconcilers (both now BYO-VM/host-process-only —
> see [`docs/platform-verification.md`](platform-verification.md)'s S8
> section for why, and the top of `README.md` for the current architecture
> diagram). The "Migrating to the real `kube-scheduler`" investigation below
> retains its historical/reference value (real bugs found by actually
> running it) even though the surrounding architecture description doesn't
> match today's layout.

## What exists today

- **One Durable Object (`Etcd`)** backs the entire cluster: kine-style KV
  storage, watch/broadcast, and the scheduler all run inside this single
  SQLite-backed DO instance (`env.ETCD.get(env.ETCD.idFromName("default"))`
  — there is only ever one).
- **A round-robin scheduler** (`packages/etcd/src/scheduler.ts`) assigns
  `spec.nodeName` to unbound pods and allocates `spec.podCIDR` to new nodes.
  It runs from the DO's `alarm()` handler, not from any HTTP path.
- **A real `pods/binding` subresource** exists in the Go apiserver
  (`pkg/apiserver/subresource.go`) — the standard protocol a real
  `kube-scheduler` would use — but nothing calls it; the scheduler above
  writes `nodeName` directly to storage instead.
- **A full watch/reflector pipeline**: the Etcd DO broadcasts change events
  over WebSocket; the Worker (`packages/k8flare-worker/src/k8s/watch.ts`) translates them
  into Kubernetes `WatchEvent`s with correct ADDED/MODIFIED/DELETED
  synthesis on label/field-selector transitions, and watch-bookmark support
  for client-go reflectors. This pipeline only serves _external_ watchers
  (kubectl, kubelet, informers) today — nothing inside the Worker/DO
  consumes its own event stream to drive reconciliation.
- **A generic CRD framework** (`packages/crd`) used by both `DynamicWorker`
  and `WorkerTrigger` — reusable for any future custom resource.
- **`WorkerTrigger`'s `cron` type is schema-validated but not wired up.**
  No Cloudflare Cron Trigger is registered anywhere and the Worker exports
  no `scheduled()` handler. This is the one "designed for this, not yet
  built" precedent in the codebase.

## What's missing

- Scheduling beyond CPU-aware predicates: memory / ephemeral-storage
  awareness, taints/tolerations, node/pod affinity and anti-affinity,
  priority and preemption. (`nodeSelector`, CPU request-vs-capacity, and
  `hostPort` conflict detection are implemented — see the Scheduling table
  in the README.)
- Controller-manager-equivalent reconciliation beyond what exists today
  (namespace cascading deletion, default ServiceAccount auto-provisioning —
  object only, no token/Secret issuance yet): Endpoints generation from
  Services+Pods, Node lifecycle (a node whose agent dies is never marked
  NotReady — it heartbeats itself via a direct write, so nothing currently
  double-checks staleness), owner reference garbage collection.
- Workload API types — `Deployment`, `ReplicaSet`, `StatefulSet`,
  `DaemonSet`, `Job`, `CronJob` — aren't registered in the scheme or
  resource stores at all (`pkg/apiserver/scheme.go`, `resources.go`).
  There is nothing yet for a controller-manager to reconcile beyond
  Pod/Node/Endpoints/Service.

## Mapping Kubernetes controllers onto Cloudflare primitives

Real Kubernetes controllers are level-triggered: watch + periodic full
resync, so that a missed or dropped watch event is eventually corrected
regardless. Of Cloudflare's primitives:

| Primitive             | Granularity                                               | Fit for controller reconciliation                                                                                                          |
| --------------------- | --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Cron Triggers         | 1 minute minimum                                          | Too coarse for anything reactive; fine for genuinely periodic, user-facing jobs (this is what `WorkerTrigger`'s unused `cron` type is for) |
| Queues                | Async, at-least-once                                      | Good for decoupling "event happened" from "action taken," not explored yet                                                                 |
| Durable Object Alarms | Sub-second, single alarm per DO, persists across restarts | Matches the watch+resync pattern well — this is what the existing scheduler already uses                                                   |

**Durable Object Alarms are the right primitive for controller
reconciliation**, and should stay co-located with the Etcd DO's storage
rather than split into separate DOs per controller — a separate DO can't
share the same SQLite instance, so cross-DO reconciliation would need
HTTP/RPC calls for no real benefit at the current scale. Revisit this only
if a single DO's alarm tick becomes a measured bottleneck.

This position survives the multi-tenancy target architecture
([`multi-tenancy-and-hosting.md`](multi-tenancy-and-hosting.md)) with one
refinement: namespaced _storage_ moves out into per-namespace follower DOs,
but controllers are writers, so they stay in the cluster DO — the one place
that assigns revisions — and that is exactly where the alarm loop already
lives.

## Cost problem with the original scheduler loop, and the fix

The original loop set `setAlarm(Date.now() + 5000)` unconditionally,
forever, from both `initialize()` and `alarm()` itself — a full
`/registry/pods/` + `/registry/nodes/` table scan every 5 seconds, whether
or not anything needed scheduling. That's ~17,000 DO wake-ups/day per
cluster regardless of activity.

Fixed by making the wake-up event-driven:

- `needsSchedulerAttention(key, value)` (`packages/etcd/src/scheduler.ts`)
  checks, on every write, whether it's a pod without `spec.nodeName` or a
  node without `spec.podCIDR`.
- When a write needs attention, `wakeSchedulerSoon()`
  (`packages/etcd/src/index.ts`) pulls the alarm in to ~1 second out if it
  isn't already due sooner — never pushes it further away, so a burst of
  writes coalesces into one scheduler pass instead of one per write.
- The periodic alarm itself now fires every 60 seconds instead of 5,
  purely as a safety net for a missed trigger (e.g. a pod that only
  becomes schedulable once a node's Ready condition flips, without any
  further pod/node write of its own) — not the primary mechanism.

Net effect: roughly a 12x reduction in idle-cluster DO wake-ups, while
actual scheduling latency in the common case dropped from "up to 5s" to
"about 1s" (debounce window).

## Scheduler correctness fix bundled with the above

The round-robin scheduler picked from _every_ registered Node regardless
of its Ready condition or `spec.unschedulable`. `isNodeSchedulable()` now
filters candidates to Ready, schedulable nodes before assigning — a node
that's registered but hasn't reported Ready yet (or was cordoned) is no
longer a valid target. Covered by `TestSchedulerSkipsNotReadyNodes` in
`pkg/apiserver/apiserver_test.go`.

Deliberately **not** done in this pass: routing the actual `nodeName`
assignment through the real `pods/binding` subresource. That subresource
lives in the Go/WASM apiserver, while the scheduler runs inside the Etcd
DO — using it would mean the DO calling back out through the Worker into
the Go apiserver, which then calls back into the same DO to persist the
result. That's a lot of cross-boundary indirection for marginal benefit
over the current direct write, which already persists and broadcasts the
change correctly.

This indirection problem goes away entirely once the scheduler is an
external process instead of code running inside the DO — see below.

## Migrating to the real `kube-scheduler`

**Done.** `cmd/scheduler` runs the actual, unmodified
`k8s.io/kubernetes/cmd/kube-scheduler` binary against this apiserver — the
TypeScript scheduler's binding logic (predicates, hostPort conflict
detection, Event emission) has been deleted; `packages/etcd/src/scheduler.ts`
now only allocates PodCIDRs. Verified against the live cluster: a real Pod
gets bound with a `Scheduled` Event and runs, and the official
sig-scheduling conformance suite (6 tests) passes end-to-end.

The predicates that used to live in `scheduler.ts` (`nodeSelector`,
capacity, `hostPort`) were a hand-written subset of what `kube-scheduler`
already does — the plan below was to stop extending that reimplementation
and run the real thing instead, which is what happened.

**The embedding pattern already exists in this dependency tree.** k3s's
own `pkg/executor/embed/embed.go` embeds kube-scheduler with the same
three calls it uses for kubelet — `sapp.NewSchedulerCommand(ctx.Done())`
→ `command.SetArgs(args)` → `command.ExecuteContext(ctx)`
(`k3s-io/k3s@.../pkg/executor/embed/embed.go` lines 263–283). A
`cmd/scheduler` binary in this repo would follow the same shape as the
existing `cmd/agent`.

**Auth is simpler than kubelet's case.** `pkg/apiserver/auth.go`'s
`AuthMiddleware` treats any bearer token equal to the shared cluster token
as authenticated with `system:masters`. A scheduler client needs only
`Host` + `BearerToken` in its kubeconfig — no client-cert bootstrap, no
`pkg/cacert` CA-swap dance. `DelegatingAuthenticationOptions` /
`DelegatingAuthorizationOptions` (used only for the scheduler's own
metrics/healthz endpoint) default to tolerating a missing
TokenReview/SubjectAccessReview API, so k8flare doesn't need to implement
either.

**Leader election turns off cleanly.** `--leader-elect=false` is the same
flag k3s itself passes when `cfg.NoLeaderElect` is set
(`pkg/daemons/control/server.go`). With it set, `Options.Config()` never
builds a `LeaderElectionConfig` and `Run()` calls `sched.Run(ctx)` directly
— zero Lease API calls needed for this mode (though Lease CRUD already
works here today if leader election is ever turned on instead).

**No new dependencies.** `k8s.io/kube-scheduler`, `k8s.io/kube-controller-manager`,
`k8s.io/controller-manager`, and `k8s.io/kubernetes` are already present in
`go.mod` as `// indirect` requires, pulled in transitively because
`k3s-io/k3s` itself imports `kube-scheduler/app`. Wiring up `cmd/scheduler`
only needs `go mod tidy` to promote them to direct requires.

**The one real gap: Dynamic Resource Allocation.** DRA's feature gate is
GA and `LockToDefault: true` as of v1.36 (`pkg/features/kube_features.go`)
— it cannot be turned off with `--feature-gates`. `scheduler.New()`
unconditionally starts informers for `ResourceClaim` and `ResourceSlice`
(`resource.k8s.io/v1`) whenever that gate is on, regardless of which
plugins are enabled in the scheduler profile. Since neither type is
registered in `pkg/apiserver/scheme.go`, those informers would never sync
and the scheduler would hang forever in `WaitForCacheSync`. The fix is
mechanical: register empty `ResourceStore`s for `ResourceClaim` and
`ResourceSlice`, the same pattern already used for `CSIDriver`/`CSINode`
in `pkg/apiserver/resources.go` — they never need real data, just to exist
so the informer can sync against an empty list.

**PV/PVC/StorageClass-touching plugins are avoidable via config, unlike
DRA.** `VolumeBinding`, `VolumeRestrictions`, `NodeVolumeLimits`, and
`VolumeZone` all touch storage types this apiserver doesn't register, but
unlike DRA these are ordinary enabled/disabled plugins — disabling them in
`KubeSchedulerConfiguration` (`profiles[0].plugins.multiPoint.disabled`)
prevents their informers from ever being registered in the first place
(`pkg/scheduler/eventhandlers.go`'s `addAllEventHandlers` only wires up
GVKs required by _enabled_ plugins).

**Binding becomes a normal external call.** Once the scheduler runs
outside the DO, assigning `nodeName` naturally goes through the real
`pods/binding` subresource (already implemented in
`pkg/apiserver/subresource.go`, currently unused) as an ordinary external
HTTP POST — the DO-calls-Worker-calls-DO indirection concern noted above
no longer applies, since the caller is now a separate process, exactly
like kubelet's own status PATCH calls today.

**Cross-compilation and CI** follow the existing `cmd/agent` pattern
exactly — two more `GOOS=linux GOARCH={arm64,amd64}` build steps in
`.github/workflows/release.yml`, output as
`k8flare-scheduler-linux-{arm64,amd64}`.

### What the plan above missed, found by actually running it

Everything above was confirmed by reading source. Two more real bugs only
showed up when a real `cmd/scheduler` was pointed at a real cluster and
asked to actually bind a pod — static analysis said "informers synced",
but zero pods were ever being scheduled:

- **DRA's informer problem is broader than ResourceClaim/ResourceSlice.**
  The default `InterPodAffinity`/`PodTopologySpread` plugins (owning-
  controller lookups), `DefaultPreemption` (PodDisruptionBudget checks),
  and DRA's own `DeviceClass` informer are _also_ started unconditionally
  by the default profile, for the same "always-on regardless of your
  plugin config" reason as DRA. `ReplicaSet`, `StatefulSet`,
  `ReplicationController`, `PodDisruptionBudget`, and `DeviceClass` all
  needed the same empty-`ResourceStore` treatment as `ResourceClaim`/
  `ResourceSlice` before every informer would actually reach
  `WaitForCacheSync`.

- **Field selectors silently dropped every `!=` term.** Real
  kube-scheduler's Pod informer filters with
  `status.phase!=Succeeded,status.phase!=Failed`
  (`pkg/scheduler/scheduler.go`'s `newPodInformer`). Both
  `pkg/apiserver/store.go`'s and `packages/k8flare-worker/src/k8s/watch.ts`'s field
  selector parsing split on the first `=` they found — for `!=` terms that
  `=` is the one inside `!=`, so `status.phase!=Succeeded` parsed as field
  `status.phase!` (with the `!` stuck to the field name) and value
  `Succeeded`. That field never matches anything, so every Pod watch event
  was silently filtered out — the informer still reported "synced" (it did
  receive the sync bookmark correctly), it just never received an actual
  pod. Fixed by checking for `!=` before falling back to `=`.

- **The official e2e framework needs `kube-root-ca.crt` too.** Not
  scheduler-specific, but only surfaced once a namespaced conformance test
  could get past its own `BeforeEach`: `test/e2e/framework/framework.go`
  waits for a `kube-root-ca.crt` ConfigMap in every test namespace before
  proceeding, the same way it waits for the default ServiceAccount. Added
  to the same `ApplyPostCreateEffects` Namespace hook
  (`pkg/apiserver/serviceaccount.go`) that already provisions the
  ServiceAccount.

One thing that did _not_ get resolved: driving these same
scheduler-dependent tests from `go test` itself (building and running a
real `cmd/scheduler` as part of `pkg/apiserver/apiserver_test.go`, the way
`setupWranglerDev` already runs a real `wrangler dev`) hit an unexplained
hang specific to being a child of the `go test` process — the identical
binary, same flags, same target server, runs correctly within seconds from
a plain shell. Not root-caused; reverted rather than left as flaky
infrastructure. The 6 tests that depended on it were removed, since they
tested the now-deleted TS scheduler's DO-alarm-based binding specifically —
scheduler behavior is validated by the official conformance suite instead,
both by hand against the live cluster and via `.github/workflows/e2e-conformance.yml`.

## Suggested sequencing for the rest

1. ~~Scheduler cost + correctness~~ — done: event-driven alarm,
   Ready/unschedulable filtering, `nodeSelector`, capacity, `hostPort`,
   scheduling Events, LimitRange enforcement.
2. ~~ServiceAccount auto-provisioning, namespace cascading deletion~~ —
   done (object-level only for ServiceAccount; no token/Secret issuance
   yet).
3. ~~Real `kube-scheduler` migration~~ — done: see "Migrating to the real
   `kube-scheduler`" above.
4. **Endpoints controller** (next): Service + Pod label selectors → Endpoints,
   paired with a Worker-side HTTP path so `ClusterIP` Services actually
   route traffic — the most-hit gap for anyone trying to run more than a
   single bare Pod.
5. **Node lifecycle**: lease staleness → `NotReady`, so a dead agent's
   pods actually get rescheduled instead of being considered "on" a node
   forever.
6. **Workload API types**: register `ReplicaSet` (and its Pod-template
   diffing/create/delete reconciler) first, since `Deployment` is a
   rollout state machine layered on top of `ReplicaSet` management, not a
   separate reconciliation loop.

This list is now superseded by two fuller plans it grew into:
[`general-purpose-k8s-plan.md`](general-purpose-k8s-plan.md) (items 4–6
above, extended through DNS, API machinery, and auth, with conformance CI as
the definition of done) and
[`multi-tenancy-and-hosting.md`](multi-tenancy-and-hosting.md) (the
multi-cluster / namespace-DO / facets / hosted-product track).

## Note (2026-07-11): Pod-on-Containers binding also moved to the real scheduler

This document predates the Loader/Dynamic-Worker rearchitecture (it still
refers to a `packages/etcd`-era "Etcd DO" that no longer exists) and only
ever covered `cmd/scheduler`'s BYO-VM/host-process path above. The
*other* scheduler this repo has -- `nodes/scheduler.ts`'s per-Pod
Containers-backend binder -- went through the same migration described
above for `cmd/scheduler`, but later and by a different route: rather
than a host process, it's the real kube-scheduler running as a fourth
Loader dynamic worker (`pkg/controllers/sched`, `docs/cost-model.md`'s
"Phase 10" entry), and rather than the TS binder calling the `pods/
binding` subresource itself, `pkg/apiserver/computeclass.go`'s
admission-time `AssignContainersNode` now pins each Pod to a dedicated,
not-yet-existing Node name so the real scheduler binds it once that
Node's own real kubelet self-registers it Ready -- see
`docs/cost-model.md`'s "Task #2" entry for the cost accounting and the
open verification item (Docker-dependent, unexercised in this sandbox).
