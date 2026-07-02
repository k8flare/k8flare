# Plan: becoming a general-purpose Kubernetes

Goal: a typical application — Deployment + Service + DNS, written for any
normal cluster — deploys here with **unmodified manifests**. The
[README's gap table](../README.md#whats-missing-for-general-purpose-use) is
the inventory; this is the order and the mechanism for closing it.

**The success metric is the conformance CI.** Every phase must grow the
required focus set in `.github/workflows/e2e-conformance.yml` — the same way
the real-scheduler migration was proven (its baseline: sig-scheduling
predicates + API watch tests). A phase isn't done when the code merges; it's
done when official e2e tests that previously could not pass are in the
required set and green.

Where controllers run: in the cluster DO's alarm loop, level-triggered
(watch-equivalent trigger + periodic resync), colocated with the revision
authority because controllers are writers. See
[`control-plane-architecture.md`](control-plane-architecture.md) for why
alarms are the right primitive and
[`multi-tenancy-and-hosting.md`](multi-tenancy-and-hosting.md) for where
storage is heading around them.

## Phase 1 — Service networking (partially done — object model shipped, kube-proxy blocked on a real bug)

`ClusterIP` Services must actually route. Three pieces, in dependency order:

1. ~~**ClusterIP allocation**~~ — done. An allocator over `ServiceIPRange`
   (`10.43.0.0/16`, `defaultClusterConfig()`, `supervisor.go`), same
   counter-in-DO pattern as PodCIDR allocation
   (`packages/etcd/src/serviceip.ts`). Reserves indices 0–10 for the future
   `kubernetes.default` (1) and `kube-dns` (10) Services.
2. ~~**EndpointSlice controller**~~ — done. Service selector + ready Pods →
   `discovery.k8s.io/v1` EndpointSlices (`packages/etcd/src/endpoints.ts`),
   mirrored to the legacy `Endpoints` type for app compatibility (kube-proxy
   in v1.36 itself consumes EndpointSlices, not Endpoints). Registering the
   type required a matching `discovery.k8s.io/v1` group in the Go apiserver
   _and_ a `RESOURCE_KINDS` entry in `packages/k8s/src/url-mapping.ts` — the
   watch layer's bookmark synthesis needs the resolved Kind to fire the
   `initial-events-end` bookmark client-go's reflector waits for; missing
   either one leaves the type served but its watches permanently unsynced.
   Verified end-to-end against a live local `wrangler dev` instance: ClusterIP
   allocation, named-port resolution, ready/not-ready address separation,
   headless-with-selector and no-selector edge cases, deletion GC, and
   reconcile idempotency (no revision churn when nothing changed) all
   confirmed correct by direct API calls.
3. **kube-proxy on agents** — **blocked, not yet enabled.** The embedded k3s
   agent (the real, unmodified `github.com/k3s-io/k3s/pkg/agent`) has its own
   real kube-proxy code, gated by the `DisableKubeProxy` field the supervisor
   sends via `/v1-k3s/config`. Flipping it to `false` **reproducibly hangs
   the Worker/DO** — Cloudflare's own "Workers runtime canceled this request
   because it detected that your Worker's code had hung" error fires within
   seconds, and the Worker never recovers (every subsequent request times
   out) until the instance is torn down. Confirmed by direct A/B testing
   (`DisableKubeProxy: true` vs `false`, all else identical) both in a local
   `wrangler dev` + real agent setup and in this repo's own
   `e2e-conformance.yml` CI run — same failure, same message, both places.
   **Not yet root-caused.** What's ruled out: a code-level infinite loop (no
   `while` loops anywhere in the new `serviceip.ts`/`endpoints.ts`, and every
   `for` loop is bounded by a SQL query result); the exact watch request
   patterns kube-proxy issues (Service with `spec.clusterIP!=None` +
   `labelSelector`, EndpointSlice with a label selector, Node by exact name)
   all independently work fine when reproduced fresh via `curl`. What's
   suspicious: the hang always appears alongside a
   `[remotedialer] Agent disconnected: ... (code=1006)` log line
   (`packages/proxy/src/remotedialer.ts`), though that file's own code looks
   too simple to hang by itself, and a lone disconnect _without_ kube-proxy
   enabled does not cause the same permanent stall — kube-proxy adds several
   new simultaneous long-lived watch connections to the same single-threaded
   DO (Service, EndpointSlice, ServiceCIDR, plus its own Node informer),
   which is the next thing to investigate: does the DO's single execution
   thread getting tied up by one stuck request cascade into every other
   concurrent watch on that DO stalling too, and does kube-proxy's specific
   connection count make that cascade unrecoverable where a lighter load
   (kube-scheduler alone) isn't? Next steps: reproduce with kube-proxy's
   informers enabled one at a time (Service only, then +EndpointSlice, then
   +Node) to see which addition first triggers the unrecoverable state;
   consider whether `workerd`'s hang-detection has a way to surface which
   specific request/promise never resolved.

Separately, a Worker-side path resolving a Service to a backing Pod IP (via
the existing VPC/tunnel plumbing) gives **external** HTTP exposure — that is
an edge feature, not a cluster-networking prerequisite, and doubles as the
future managed-Ingress story.

Verify: EndpointSlice/Endpoints computation verified directly (see above);
`[sig-network] EndpointSlice`/`EndpointsController` basics added to the
CI's EXPERIMENTAL group (`.github/workflows/e2e-conformance.yml`) — these
don't need kube-proxy to pass, since they only check the _objects_, not
traffic routing. `curl` a ClusterIP from inside a pod and
`[sig-network] Services should serve a basic endpoint from pods` (real
traffic routing) stay blocked until the kube-proxy hang above is fixed.

## Phase 2 — Node lifecycle (self-healing, part 1)

A dead agent today stays `Ready` forever and its pods are never rescheduled.
An alarm-driven controller over `kube-node-lease`:

- Lease staleness past threshold → set `NodeReady=Unknown`, apply
  `node.kubernetes.io/unreachable` `NoExecute` taint.
- Node dead past a longer threshold → delete its pods (the upstream pod GC
  role), so schedulable replacements can exist.

Note the interplay: eviction alone just kills pods — _recreation_ needs Phase 3. Shipping this first is still correct (the scheduler already refuses
not-Ready nodes; stale state is the bug).

Verify: kill an agent; node goes NotReady and its pods are removed within the
thresholds. Conformance: node lifecycle tests where applicable.

## Phase 3 — Workload controllers + garbage collection (self-healing, part 2)

The single biggest "feels like normal Kubernetes" gap. Order matters:

1. **ReplicaSet** — the primitive reconciler: template hash, create/delete
   pods toward `spec.replicas`. The apps/v1 type is already registered (as a
   scheduler-informer stub) — this upgrades it to a real, controlled
   resource.
2. **Deployment** — a rollout state machine layered on ReplicaSets, not a
   separate pod-management loop.
3. **Job / CronJob** — run-to-completion semantics; CronJob's tick maps
   naturally onto DO alarms.
4. **DaemonSet** — after node lifecycle, since it reconciles against node
   membership.
5. **StatefulSet** — deliberately last; honest StatefulSet support needs the
   storage story (below).
6. **OwnerReference GC** — cascading deletion (delete a Deployment, its
   ReplicaSets and Pods go too). Required from step 1 onward; implemented as
   a background sweep in the same alarm loop.

Verify: conformance `[sig-apps]` ReplicaSet/Deployment basics move into the
required set — these are Conformance-tagged upstream, so this phase is the
largest single jump in official conformance coverage.

## Phase 4 — Cluster DNS

The `MissingClusterDNS` kubelet warning, observed on every real pod today.
Standard shape, k3s-style:

1. CoreDNS as a Deployment (Phase 3) with a `kube-dns` Service pinned to
   `10.43.0.10` (Phase 1).
2. CoreDNS authenticates to the apiserver with a mounted kubeconfig Secret
   carrying a scoped token initially; upgraded to a projected ServiceAccount
   token when Phase 5 lands TokenRequest.
3. Only then flip the supervisor to advertise `cluster-dns=10.43.0.10` to
   kubelets — advertising a dead DNS IP earlier would break pod DNS, so this
   is strictly last.

Verify: `nslookup kubernetes.default.svc.cluster.local` from a pod;
conformance `[sig-network] DNS` basics.

## Phase 5 — API machinery & auth parity

Quick wins first:

- **Version honesty**: discovery reports `Major:1 Minor:34`
  (`discovery.go:216`) while the stack builds and tests against v1.36 —
  report 36.
- **PriorityClass**: register `scheduling.k8s.io/v1` and resolve
  `priorityClassName` → `spec.priority` at pod admission (the apiserver's
  job, not the scheduler's); the scheduler side already works.
- **OpenAPI v2/v3 discovery documents** for the served types, so `kubectl
apply` stops needing `--validate=false`.

Then the auth ladder, shared with the hosted-product plan:

- **TokenRequest / TokenReview** — real ServiceAccount tokens (unblocks
  in-cluster clients done properly, starting with CoreDNS).
- **RBAC** — types, an authorizer in front of the stores, default
  roles/bindings. This is also what turns namespaces into a sellable tenancy
  boundary (see the multi-tenancy doc), so it is sequenced with that track.
- Server-side apply, dry-run, admission webhooks: evaluated after the above;
  client-side apply with OpenAPI covers most real usage until then.

## Phase 6 — the rest, on demand

- **Storage**: PV/PVC/StorageClass with a `local-path`-style provisioner
  first (k3s precedent), before any network storage ambitions.
- **Metrics & autoscaling**: `metrics.k8s.io` served from kubelet summary
  data, then HPA.
- **NetworkPolicy**: blocked on CNI choice (flannel host-gw enforces
  nothing); revisit with the node-backend work.
- **Ingress**: the Worker _is_ the ingress — this folds into the hosted
  product's Service-exposure path rather than running an ingress controller
  per cluster.

## Sequencing rationale

Phases 1→4 are strictly ordered by dependency (Endpoints before kube-proxy
before DNS; workload controllers before CoreDNS-as-Deployment). Phase 5's
quick wins can land any time; its auth ladder gates both Phase 4's clean
ending and the multi-tenancy track's namespace tenancy. Phase 6 items are
pulled forward only by real user demand.
