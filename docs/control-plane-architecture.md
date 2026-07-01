# Scheduler and controller-manager on Cloudflare

Notes from investigating how much of a real Kubernetes control plane
(scheduler, controller-manager) k8flare currently has, and how the rest
could be implemented within Cloudflare's execution model. This reflects the
state as of the watch-conformance and scheduler-cost fixes.

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
  over WebSocket; the Worker (`packages/k8s/src/watch.ts`) translates them
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

- Any real scheduling algorithm beyond round-robin (taints/tolerations,
  affinity, resource-aware bin-packing, `nodeSelector`).
- Any controller-manager-equivalent reconciliation: Endpoints generation
  from Services+Pods, Node lifecycle (a node whose agent dies is never
  marked NotReady — it heartbeats itself via a direct write, so nothing
  currently double-checks staleness), ServiceAccount + token
  auto-provisioning per namespace, namespace cascading deletion, owner
  reference garbage collection.
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
change correctly. Worth reconsidering if/when the binding subresource
needs to do something the direct write can't (admission hooks, audit
trail).

## Suggested sequencing for the rest

1. **Scheduler cost + correctness** (this pass): event-driven alarm,
   Ready/unschedulable filtering.
2. **Small reconcilers on the same pattern**: Node lifecycle (lease
   staleness → NotReady), ServiceAccount + token auto-provisioning per
   namespace, namespace cascading deletion. These are all "watch one or
   two resource types, fix up related state" — the same shape as the
   existing scheduler functions, and establish the reconciler pattern
   other controllers can follow.
3. **Workload API types**: register `ReplicaSet` (and its Pod-template
   diffing/create/delete reconciler) first, since `Deployment` is a
   rollout state machine layered on top of `ReplicaSet` management, not a
   separate reconciliation loop.
