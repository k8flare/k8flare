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

## Phase 1 — Service networking (object model done + verified; kube-proxy enabled; real traffic routing not yet proven end-to-end)

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
3. ~~**kube-proxy on agents**~~ — **enabled** (`DisableKubeProxy: false` in
   `supervisor.go`). A `networking.k8s.io/v1` `ServiceCIDR` stub had to be
   registered too: `MultiCIDRServiceAllocator` is GA and `LockToDefault: true`
   as of Kubernetes 1.35 (`pkg/features/kube_features.go` in the vendored
   `k8s.io/kubernetes`), so kube-proxy's `server.go` unconditionally creates
   and starts a `ServiceCIDR` informer regardless of whether anything in the
   cluster uses dynamic ServiceCIDR allocation — the same "unconditionally-
   started informer for an unregistered type hangs WaitForCacheSync forever"
   failure shape already seen twice before during the real-scheduler
   migration (DRA's ResourceClaim/ResourceSlice/DeviceClass, and
   ReplicaSet/StatefulSet/PodDisruptionBudget), also confirmed here via the
   same `matchesFieldSelector` gap (`spec.clusterIP!=None`, which kube-proxy's
   Service informer filters on, silently passed everything through until
   `spec.clusterIP` was added to the field allowlist in `store.go`).

   **An important correction, found by chasing this down properly rather than
   stopping at the first plausible-looking cause:** an earlier pass through
   this investigation concluded kube-proxy itself reproducibly hangs the
   Worker/DO, based on A/B testing that looked clean at the time. That
   conclusion was wrong, and the flaw was in the control: the "kube-proxy
   disabled" comparison run used a Durable Object instance with leftover
   state from many earlier manual test iterations (wrangler dev's local
   persistence lives at `packages/worker/.wrangler/state`, not the repo
   root's `.wrangler/state`, which is what got cleared) — never a genuinely
   fresh instance. Redone properly (`rm -rf packages/worker/.wrangler/state`,
   confirmed by a `resourceVersion` starting at single digits), the _same_
   "Workers runtime canceled this request because it detected that your
   Worker's code had hung" error reproduces **on a fresh `main` branch
   checkout with zero Phase 1 changes and kube-proxy never touched** — both
   locally is fine (never reproduces there at all, on this Mac) and, more
   importantly, **in this repo's own `e2e-conformance.yml` CI run on `main`
   plus only the unrelated `pnpm install` fix**. That is the definitive
   control: identical failure, identical error message, with none of this
   session's code in play. The hang is a **pre-existing, CI-environment-
   specific issue, unrelated to kube-proxy or anything in this phase** — it
   was never caught before simply because this workflow had never actually
   completed a run before this session (see the `pnpm install`/`npm install`
   bug fixed earlier).

   Sub-agent research into `workerd`'s actual source
   (`cloudflare/workerd`, `io-context.c++`/`io-gate.h`) narrows down what
   this pre-existing issue likely is, even though it's not yet fixed: hang-
   detection is idle-based, not a timer, and explicitly **does not apply to
   Durable Object (actor) requests** (`KJ_ASSERT(actor == kj::none)`) — so
   the abort fires in the _entry Worker_ awaiting a DO response, not inside
   the DO's own JS. All requests to one DO instance serialize through a
   single `InputGate`; if one entry-Worker-to-DO request's promise never
   settles (e.g. because a burst of simultaneous long-lived WebSocket-watch
   `fetch()` calls to the same DO instance queues deep enough under a
   constrained-CPU runner), that hung await gets force-aborted, but the
   DO-side InputGate it was waiting on may stay held — explaining the
   observed cascade (every other request to that same DO instance then
   stalls forever, until the instance is torn down). This is a real,
   distinct problem from anything in this phase's own code and needs its
   own investigation (see "Open follow-ups" below) — it is CI-specific,
   confirmed not to reproduce locally even after extensive repeated testing.

   **What is proven, locally, with kube-proxy enabled:** a real kubelet +
   kube-proxy joins, the node reaches `Ready`, `ClusterIP`/`EndpointSlice`/
   `Endpoints` are all created correctly for a real Service+Pod. **What is
   NOT yet proven:** an actual `curl` to a `ClusterIP` reaching the backing
   Pod. One local end-to-end attempt hit a separate, third issue before
   getting that far: kubelet's own Node _lister_ (its local informer cache)
   intermittently disagreed with direct API queries -- `kubectl`-equivalent
   `GET`s on the Node object returned correct, fresh data (confirmed
   directly, and zero errors appeared in the apiserver's own logs during the
   episode) while kubelet's internal error log kept insisting the node
   "was not found," blocking pod admission indefinitely. Ruled out as the
   cause: this session's new `reconcileNodeLifecycle` incorrectly tainting
   the node (checked directly -- no taint, condition still `Ready: True`,
   Lease still being renewed normally throughout). Not yet root-caused:
   likely a watch-delivery reliability gap under concurrent load, distinct
   from the CI hang above, tracked as a follow-up rather than chased further
   in this pass.

Separately, a Worker-side path resolving a Service to a backing Pod IP (via
the existing VPC/tunnel plumbing) gives **external** HTTP exposure — that is
an edge feature, not a cluster-networking prerequisite, and doubles as the
future managed-Ingress story.

Verify: EndpointSlice/Endpoints computation and kube-proxy startup verified
directly (see above); `[sig-network] EndpointSlice`/`EndpointsController`
basics added to the CI's EXPERIMENTAL group
(`.github/workflows/e2e-conformance.yml`) — not yet actually green in CI
because of the pre-existing environment issue above, unrelated to whether
these specific tests are correct. `curl` a ClusterIP from inside a pod and
`[sig-network] Services should serve a basic endpoint from pods` (real
traffic routing) remain unverified pending the watch-delivery follow-up.

### Open follow-ups from this phase

- **CI environment hang** (`e2e-conformance.yml` on GitHub-hosted
  `ubuntu-latest`): reproduces on a bare `main` checkout, so it blocks _any_
  future phase's CI verification, not just this one. Next step: reproduce
  with `wrangler dev --local-protocol http` (ruling out TLS-handshake-related
  idle time) and/or a self-hosted runner with more CPU, to test the
  constrained-CPU-runner hypothesis directly; consider filing a
  `cloudflare/workerd` issue with the InputGate-cascade theory once
  reproduced with a minimal case.
- **kubelet Node-lister staleness under concurrent watch load**: observed
  once, locally, not yet reliably reproduced or root-caused. Next step:
  reproduce deliberately (same Service/Pod/kube-proxy setup) and check
  whether the Node watch's underlying WebSocket ever received a
  message-delivery gap (compare kine revision numbers the DO broadcast vs.
  what the entry Worker's watch relay actually forwarded) rather than
  assuming it's connection-level.
  **Likely the same root cause as a second, more concrete data point found
  during Phase 9's Cloudflare Mesh evaluation** (`spikes/p9-mesh/RESEARCH.md`):
  a real 2-node flannel setup (either `host-gw` or `wireguard-native` — both
  reproduced identically) never completed cross-node route/tunnel
  convergence within a ~7-minute window, because flannel's own Node informer
  (a plain, unfiltered List+Watch, not the field-selector bug class found
  elsewhere) never finished its initial sync. This is very likely the actual
  mechanism behind this row and behind the "Service networking" gap in
  `README.md`'s table ("actual traffic routing... isn't proven end-to-end
  yet") — i.e. this is not just a Service/EndpointSlice-specific issue, it
  blocks multi-node pod networking generally, regardless of CNI backend.
  Worth prioritizing given it now blocks two independent things.
- **The same class of instability now also shows up locally in
  `go test ./pkg/apiserver/...`** (not just CI), simply because the suite
  has grown: 6 new test files/functions were added across Phase 3, so a full
  run now takes long enough (~70-90s, up from ~20s) to occasionally hit
  whatever the underlying limit is. Confirmed this isn't a regression in any
  specific new code: an isolated run of just the new tests
  (`-run 'TestWorkloadStatusSubresources|TestResourceAPIGroup|TestSupervisor|TestBasicAuth'`)
  passes cleanly and quickly every time, and the full suite's pass/fail
  outcome was inconsistent across repeated runs with zero code changes in
  between (4 clean passes, then a failure, then clean again) — this is
  capacity-related flakiness in the shared local `wrangler dev` instance
  under sustained load, not a deterministic bug. If it gets bad enough to
  block routine development, consider splitting `apiserver_test.go` into
  per-domain test binaries (each getting its own fresh `wrangler dev`
  instance) rather than one ever-growing shared one.

## Phase 2 — Node lifecycle (self-healing, part 1) — done, end-to-end verified

A dead agent today stays `Ready` forever and its pods are never rescheduled.
An alarm-driven controller over `kube-node-lease`
(`packages/etcd/src/nodelifecycle.ts`, `reconcileNodeLifecycle`, wired into
the alarm loop like the other controllers):

- ~~Lease staleness past threshold (40s, matching upstream's default
  `--node-monitor-grace-period`) → set every health condition
  (`Ready`/`MemoryPressure`/`DiskPressure`/`PIDPressure`) to `Unknown`, apply
  `node.kubernetes.io/unreachable` `NoExecute` taint~~ — implemented and
  verified.
- ~~Node dead past a longer threshold (5 minutes, matching upstream's
  default `--pod-eviction-timeout`) → delete its Pods (the upstream pod GC
  role), so schedulable replacements can exist~~ — implemented and verified.

No `needsXAttention` trigger-check exists for this one deliberately:
staleness is detected by the _absence_ of an expected write, so only the
periodic safety-net alarm tick can catch it (already sufficient granularity
at 60s against a 40s threshold). Deliberately not implemented: recovery (a
taint/Unknown status is never proactively cleared when a node comes back —
accepted gap for this pass, noted in the module's own doc comment).

Note the interplay: eviction alone just kills pods — _recreation_ needs
Phase 3. Shipping this first is still correct (the scheduler already refuses
not-Ready nodes; stale state is the bug).

**Verified end-to-end** against a real local `wrangler dev` + real agent:
confirmed the write logic does _not_ misfire on a healthy node (an
actively-renewed node kept `Ready: True`, no taint, throughout Phase 1
testing), then killed a real agent process outright and watched the full
sequence happen for real: the node's `Ready`/`MemoryPressure`/`DiskPressure`/
`PIDPressure` conditions all flipped to `Unknown` and the
`node.kubernetes.io/unreachable` `NoExecute` taint was applied within 48
seconds of the last Lease renewal (within the expected ~40–100s window given
the 40s grace period checked on a 60s tick); a Pod bound to that node was
then correctly deleted (404 on a subsequent `GET`) once staleness passed the
5-minute eviction threshold.

One local-dev-specific wrinkle surfaced doing this: `wrangler dev`'s local
Durable Object Alarm emulation did not visibly fire again on its own during
~7 minutes of pure read-only polling (no writes) — the eviction only
actually happened once an unrelated write (`wakeSchedulerSoon()`, via
creating an unrelated ConfigMap) forced the alarm to re-check. This looks
like a local-dev-only alarm-scheduling quirk (a true wall-clock proactive
timer vs. one that only gets re-checked when _some_ request nudges the DO
locally) rather than a logic bug — the eviction fired correctly and
immediately once re-checked, with the exact right condition
(`staleMs > POD_EVICTION_MS`) already true. Worth keeping in mind for future
alarm-dependent verification in local dev: don't conclude "not working" from
pure-wait polling alone without also trying a forced wake.

Verify: kill an agent; node goes Unknown, gets tainted, and its pods are
removed within the thresholds — **done**, see above. Conformance: node
lifecycle tests where applicable — not yet added to CI (blocked on the
pre-existing CI-environment issue from Phase 1).

## Phase 3 — Workload controllers + garbage collection (self-healing, part 2)

The single biggest "feels like normal Kubernetes" gap. Order matters:

1. **ReplicaSet / Deployment / Job / CronJob / DaemonSet** — done, end-to-end
   verified, via the **real, unmodified `kube-controller-manager` binary**
   (`cmd/controller-manager`), not hand-written TypeScript.

   The first pass through this phase hand-wrote a TypeScript reconciler for
   each of these five in `packages/etcd/src/*.ts`, run from the cluster DO's
   alarm loop. Each was implemented and individually verified end-to-end
   against a real local deployment, and each surfaced real bugs along the
   way — a `store.go` `Update()` that didn't preserve `metadata.uid`/
   `creationTimestamp` (breaking `ownerReferences`-based ownership checks
   generally, not just for one controller), the same UID/timestamp gap
   reproduced in the hand-written controllers' own direct-to-storage object
   creation, and a kine store revision-chaining collision on deterministic
   ReplicaSet/Job naming. All of this worked, but was a from-scratch
   reimplementation of non-trivial upstream semantics (rollout math, cron
   parsing, taint/toleration eligibility, run-to-completion bookkeeping) —
   exactly the kind of thing `cmd/scheduler` had already shown could instead
   be solved by embedding the real component. Once that was confirmed
   feasible for the controller-manager too (see below), all five `.ts` files
   were deleted outright in favor of it.

   **How the embed works**: identical pattern to `cmd/scheduler` — the real
   k3s codebase already runs `kube-controller-manager` this exact way
   (`pkg/executor/embed/embed.go`'s `ControllerManager` method):
   `cmapp.NewControllerManagerCommand()` → `command.SetArgs([...])` →
   `command.ExecuteContext(ctx)`, no subprocess exec, no new go.mod
   dependencies (`k8s.io/kube-controller-manager`/`k8s.io/controller-manager`
   were already transitive requires). `--controllers=` cleanly enables only
   the five named controllers, `--leader-elect=false` and a bearer-token
   kubeconfig match the scheduler's already-proven simplicity, and
   `--secure-port=0` disables the controller-manager's own healthz/metrics
   server (which would otherwise need delegated authentication/authorization
   against a real apiserver this project doesn't have).

   **What the apiserver needed before the embed would actually work** —
   found by running the real binary against it, not by reading its source
   first:
   - **`/status` subresources** for `replicasets`, `deployments`,
     `daemonsets`, `jobs`, `cronjobs` (`pkg/apiserver/subresource.go`,
     mirroring the pre-existing `pods/status`/`nodes/status` cases exactly).
     Real controllers call `UpdateStatus()`, which hits this subresource
     unconditionally — without it, every status update 404s silently.
   - **A `ControllerRevision` (apps/v1) stub type**, registered the same way
     as the DRA/ReplicaSet stub types were for the scheduler migration —
     without it, the DaemonSet informer's `WaitForCacheSync` blocks forever
     at startup and no controller in the process ever starts working.
   - **A `RESOURCE_KINDS` entry** (`packages/k8s/src/url-mapping.ts`) for
     `deployments`, `daemonsets`, `jobs`, `cronjobs`, and
     `controllerrevisions` — missing exactly like the EndpointSlice gap
     found during the kube-proxy migration: without the resolved Kind, watch
     bookmark synthesis can't fire `initial-events-end`, so these five
     types' informers never receive it and their reflectors log "event
     bookmark expired" and effectively never finish their initial sync —
     `deployment`/`daemonset`/`job`/`cronjob` controllers all silently never
     started processing anything until this was fixed.
   - **Real upstream apps/v1 and batch/v1 admission defaulting**, via
     `appsv1defaults.RegisterDefaults(Scheme)` /
     `batchv1defaults.RegisterDefaults(Scheme)` (the actual
     `k8s.io/kubernetes/pkg/apis/{apps,batch}/v1` packages, already a
     transitive dependency) plus a new `Scheme.Default(obj)` call in
     `ApplyDefaults` — not hand-reimplemented. Without this, a Deployment
     created with `spec.strategy.type` unset (the normal case) made the real
     deployment controller hard-error forever with `"unexpected deployment
strategy type: "`, since real clients rely on apiserver-side admission
     to fill in `RollingUpdate` before the object is ever stored.
   - **`metadata.generateName` support** in `store.go`'s `Create()` (using
     the real `k8s.io/apiserver/pkg/storage/names.SimpleNameGenerator`, not
     a hand-rolled equivalent) — real controllers create Pods this way
     rather than picking an explicit name themselves; without it every Pod
     create from a real controller failed with `"name is required"`.
   - **Job's `spec.selector`/`controller-uid` label auto-generation**
     (`store.go`, a small targeted port of upstream's unexported
     `pkg/registry/batch/job/strategy.go` `generateSelectorIfNeeded` —
     unexported, and needs the object's UID, so it couldn't be reused
     directly and has to run after `Create` assigns one). Without it, a
     Job's `spec.selector` stayed nil forever, so the real Job controller's
     own ownership check against each Pod it created always failed, and it
     repeatedly disowned and replaced the Pods it had just made.

   **Verified against a real local deployment**, with the real binary
   itself, after each fix: ReplicaSet create/scale up/scale down; Deployment
   create (auto-generates its ReplicaSet, upstream's real hash-based naming
   convention) and scale; Job `completions`/`parallelism`-bounded Pod
   creation with correct `ownerReferences` retained; CronJob scheduling a
   Job every real minute tick with correct naming and status tracking;
   DaemonSet placing one Pod per Node via `nodeAffinity`
   (`matchFields: metadata.name`) — which also required running the real
   `cmd/scheduler` alongside the controller-manager, since (unlike the old
   hand-written version) DaemonSet pods aren't pre-bound with
   `spec.nodeName` directly, they rely on the real scheduler to bind them
   via that affinity, exactly like any other Pod.

2. **StatefulSet** — deliberately last; honest StatefulSet support needs the
   storage story (below). Likely the same real-binary approach once
   reached — `statefulset` is already one of the controllers the real
   `kube-controller-manager` registers, just not yet added to
   `--controllers=`.
3. **OwnerReference GC** — cascading deletion (delete a Deployment, its
   ReplicaSets and Pods go too). The real `kube-controller-manager` has a
   `garbagecollector` controller that does exactly this generically for any
   type; not yet added to `--controllers=` here since it needs its own
   verification pass (discovery + a metadata-only client instead of a typed
   one — Get/List have a documented, likely-compatible fallback path in
   client-go for older-style apiservers like this one, but Watch is
   unverified) rather than assuming it works.

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
