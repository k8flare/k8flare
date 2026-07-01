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

## Phase 1 — Service networking (in progress: the "next" roadmap item)

`ClusterIP` Services must actually route. Three pieces, in dependency order:

1. **ClusterIP allocation** — an allocator over `ServiceIPRange`
   (`10.43.0.0/16`, declared in `defaultClusterConfig()`,
   `supervisor.go:36-52`), same counter-in-DO pattern as PodCIDR allocation
   (no ClusterIP allocator exists yet — confirmed by grep, this is new work).
2. **EndpointSlice controller** — Service selector + ready Pods →
   `discovery.k8s.io/v1` EndpointSlices (not registered anywhere yet —
   confirmed). Note: kube-proxy in v1.36 consumes **EndpointSlices, not
   Endpoints** — serving the legacy `Endpoints` type (which we already
   register) is for app compatibility, so the controller mirrors to both.
3. **kube-proxy on agents** — the embedded k3s agent (the real,
   unmodified `github.com/k3s-io/k3s/pkg/agent`, run from `cmd/agent`) has
   its own real kube-proxy code, gated by the `DisableKubeProxy` field it
   reads from the supervisor's `/v1-k3s/config` response
   (`k3s`'s own `pkg/agent/config/config.go`'s `getKubeProxyDisabled`); our
   supervisor sets that field `true` (`supervisor.go:47`). Flip it to
   `false` so in-cluster pod→ClusterIP traffic is programmed node-side with
   zero Worker involvement.

Separately, a Worker-side path resolving a Service to a backing Pod IP (via
the existing VPC/tunnel plumbing) gives **external** HTTP exposure — that is
an edge feature, not a cluster-networking prerequisite, and doubles as the
future managed-Ingress story.

Verify: `curl` a ClusterIP from inside a pod; add conformance
`[sig-network] Services` basics to the required set.

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
