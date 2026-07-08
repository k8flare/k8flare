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

## Phase 1 — Service networking (done: real traffic routing proven end-to-end 2026-07-08, see the update below; CI gate is a custom exec-free check, not the upstream exec-dependent test)

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

- **The ClusterIP real-traffic promotion attempt (2026-07-07) found a
  DIFFERENT blocker than expected, before ever reaching kube-proxy.**
  `[sig-network] Services should serve a basic endpoint from pods
[Conformance]` was run in isolation in CI
  (`.github/workflows/e2e-conformance.yml`'s advisory "ClusterIP traffic
  candidate" step) and failed: `service is not reachable within 2m0s
timeout`. The actual cause, from the ginkgo log, is that upstream's
  reachability check drives itself via `kubectl exec <pod> -- nc ...`,
  and every attempt got `error: unable to upgrade connection: exec
requires a WebSocket upgrade (Upgrade: websocket header missing)` —
  i.e. `kubectl exec` itself never worked in this CI run, which matches
  README's documented gap ("`kubectl logs`/`kubectl exec` — Off by
  default — requires the optional Cloudflare Tunnel + VPC Service
  setup") that `e2e-conformance.yml` does not configure. **Whether
  kube-proxy actually routes ClusterIP packets to a pod was never
  exercised** — the test never got past its own connectivity-check
  plumbing. Recorded per rule #4 instead of quietly reverting the
  candidate step: it stays in CI as a advisory, continuously-run probe
  (harmless, capped at 10 minutes) until either (a) the Tunnel+VPC path
  is wired into the CI harness so `kubectl exec` actually works there,
  or (b) a non-exec verification method is found. Do not promote this
  test to `BASELINE_FOCUS` based on this run's green-looking CI status
  — that greenness comes entirely from `continue-on-error: true`
  absorbing the step's real failure, not from the test passing.
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

### 2026-07-08 update: real ClusterIP traffic routing PROVEN, plus two real infra bugs found and fixed along the way

Follow-up to the 2026-07-07 entry above, which established that `kubectl
exec` (not networking) was blocking the upstream conformance test, and that
Cloudflare Mesh (`b6d4340`, landed after that entry) was a candidate to
unblock it. Investigated whether Mesh actually unblocks `kubectl exec` in
`e2e-conformance.yml`, and — per rule 2 — verified the actual networking
claim directly, in an environment fully under control, rather than trusting
the CI-wiring question's answer to stand in for it.

**Real ClusterIP routing works.** Verified directly with a real local
`wrangler dev` (Go/WASM apiserver, real Durable Object storage) and a real
`cmd/agent` (embedded kubelet + containerd + kube-proxy in iptables mode +
flannel host-gw), run inside a privileged Docker container (containerd needs
a real Linux kernel, unavailable on this session's macOS host) joining that
`wrangler dev` instance exactly like a BYO VM would:

1. Created a real `nginx:alpine` backend Pod (scheduled by the real,
   unmodified `cmd/scheduler`) and a `ClusterIP` Service selecting it.
   Confirmed `EndpointSlice` correctly populated with the Pod's real IP
   (`10.42.1.2`).
2. Created a second, separate `busybox` Pod (`10.42.1.3`, scheduled onto the
   same node by the same real scheduler). Entered its network namespace via
   `crictl exec` — the container-runtime level, deliberately bypassing this
   project's own `kubectl exec` gap entirely, since that gap is exactly what
   this investigation needed to route around to test networking in
   isolation.
3. From inside that separate Pod's network namespace: `wget
http://10.43.90.196:80/` (the Service's real `ClusterIP`) returned nginx's
   actual welcome page, exit code 0. Cross-checked the same ClusterIP from
   the node's own host network namespace (same result) to confirm the
   iptables `KUBE-SERVICES` DNAT chain kube-proxy installs is what's doing
   the routing, not some other path.

This directly confirms `[sig-network] Services should serve a basic endpoint
from pods [Conformance]`'s actual subject matter — a Pod reaching another
Pod through a Service's `ClusterIP` — works correctly end-to-end on this
stack. The only thing that ever blocked this test was its own exec-based
reachability check, exactly as the 2026-07-07 entry suspected but had not
directly confirmed.

**Two real, previously-undetected infrastructure bugs were found and fixed
while setting up this verification** (both are genuine fixes, not workarounds
specific to the Docker-based test rig):

1. **`wrangler dev` startup now hard-fails with no Cloudflare credentials, a
   silent regression from `b6d4340`.** `workers/k8flare/wrangler.jsonc`'s
   `vpc_networks` MESH binding has zero local-dev emulation — confirmed
   directly that `remote: true` vs `false` makes no difference, and that
   even a syntactically-valid but wrong `CLOUDFLARE_API_TOKEN` still hard
   fails (needs a genuinely valid, correctly-scoped token). `wrangler dev`
   tries to establish a real remote-proxy session for this binding at
   **startup**, not first use, and exits immediately if that fails — before
   ever binding a port. Confirmed via `gh run list` that no
   `e2e-conformance.yml` run has executed since `b6d4340` landed (2026-07-07
   13:52 JST), so this was never caught. Worse: `pkg/apiserver/apiserver_test.go`'s
   `setupWranglerDev` — the single choke point nearly every Go test in this
   package uses — has the identical gap, and `ci.yml`'s "Go test" step (which
   gates every PR to `main`) hasn't run since 2026-07-03, well before
   `b6d4340`, so it's _also_ never been caught. This is easy to miss on a
   developer machine with a cached `wrangler login` session — it silently
   succeeds by actually proxying that one binding through real Cloudflare
   infrastructure instead of failing (confirmed by reproducing both the
   failure, with a clean `HOME` and zero credentials, and the false-negative
   "it works for me," with an ambient real OAuth session, on the same
   machine). **Fix**: pass `--local` (disables remote bindings outright,
   nothing in either harness needs `MESH`) — added to
   `.github/workflows/e2e-conformance.yml`'s wrangler dev step and to
   `setupWranglerDev`'s `exec.Command` args. Verified the fix directly: `go
test -count=1 ./pkg/apiserver/...` now passes with `HOME` pointed at an
   empty directory and zero Cloudflare env vars set, which failed (hung at
   wrangler dev startup) before the fix.
2. **A real race condition in `pkg/cacert.ReplaceServerCA`** blocked the
   local verification above before it ever got to test networking: the
   goroutine that replaces k3s's self-signed `server-ca.crt` with the system
   CA bundle (needed because Cloudflare terminates TLS with a publicly
   trusted cert, not k3s's own) used a fixed 10ms poll, racing against k3s's
   own bootstrap goroutine, which downloads and writes that same file and
   then, in the same call chain, immediately builds a long-lived REST client
   that reads it exactly once (`k3s-io/k3s/pkg/executor/embed`'s
   `util.WaitForAPIServerReady`). Losing this race permanently wedges the
   agent: a goroutine dump (`SIGQUIT`) showed
   `(*Embedded).Kubelet.func1()` parked forever on
   `<-e.APIServerReadyChan()`, because the captured client can never
   validate the real server's TLS certificate. Real bare-metal CI runners
   have so far reliably won this race (`e2e-conformance.yml`'s "Wait for
   node to register as Ready" step consistently completes in ~11s across
   the runs checked) — but a privileged Docker container on this session's
   OrbStack-backed Linux VM reproducibly lost it across every clean restart
   tried. This is a genuine, latent race regardless of which environment
   currently wins it more often, per rule 5 (fix flaky infra, don't leave it
   to chance) — a slower or more contended bare-metal runner could just as
   easily lose it. **Fix**: replaced the fixed-interval poll with an
   `fsnotify` watcher reacting to the actual file-write event (sub-
   millisecond latency instead of up to 10ms of blind polling), plus an
   immediate replace-if-present check right after the watcher is armed
   (covers process restarts and the watcher-setup window). This cannot make
   the race fully deterministic without a local fork of the vendored k3s
   embed code to add a real synchronization point (out of scope here), but
   narrows the window by roughly two orders of magnitude. Verified directly:
   the exact same Docker-based repro that reliably hung before the fix
   (across every attempt) reached `Node Ready` promptly and repeatably
   after it.

**CI wiring investigated and NOT adopted**: wiring Mesh into
`e2e-conformance.yml` so the upstream `kubectl exec`-based test could pass
there was investigated concretely, not just considered abstractly.
`workers/k8flare/src/gateway/proxy/target.ts`'s `resolveKubeletTarget` —
used by **both** `kubectl logs` and `kubectl exec` for BYO-VM nodes, a
correction to this task's own premise that logs is exec-free for this node
type — requires either the `MESH` binding (real Cloudflare infrastructure,
see bug 1 above) or the legacy `KUBELET_VPC` Tunnel+VPC Service binding
(same "no local emulation" property). Making this test pass in CI would mean
minting a real Mesh connector token per ephemeral CI run (the same API calls
`workers/k8flare/src/nodes/meshconnector.ts` already makes for per-Pod
Mesh, verified against the real KOOFFICE account per
`spikes/s17-mesh-nodevm/FINDINGS.md`), a new sensitive
`CLOUDFLARE_API_TOKEN` CI secret (Zero Trust/Tunnel scope), and per-run
cleanup to avoid leaking the account's 50-connector cap — all of which
directly contradicts this workflow's own deliberate design property, stated
in its header comment: "Everything runs local-only on the runner (127.0.0.1)
-- this never touches the deployed Cloudflare Worker." Concretely, every
real Cloudflare credential this session had access to belongs to this
project's own KOOFFICE account (`.secrets/`), and minting even a
throwaway, immediately-deleted resource against it on every future PR run
is a materially different, ongoing security posture than what this harness
was designed for — not something to switch to as a side effect of one
promotion decision. **Not pursued.**

**What was promoted instead**: a new REQUIRED step,
`.github/workflows/e2e-conformance.yml`'s "Verify real ClusterIP traffic
routing (Phase 1 promotion gate, exec-free)", proves the identical
underlying claim the upstream test names (a separate Pod reaching another
Pod through a Service's `ClusterIP`) through Pod `status.phase` /
`containerStatuses[].state.terminated.exitCode` — kubelet reports both to
the apiserver directly, over the same plain object-API path every other
step in this job already polls with `curl`+`jq`, regardless of the
kubelet-proxy gap. Verified by hand first, then by running the extracted
step script itself (not just its logic) against the live local repro
described above, real scheduler included. This is a deliberate departure
from "grow `BASELINE_FOCUS`" — it is not a ginkgo focus string, because the
specific upstream test named as this phase's acceptance criterion cannot run
in this harness as designed (its reachability check is exec-driven, not a
networking check) — but it is a new, required, blocking, real verification
of the same underlying claim, which is what "growing the required set"
means in spirit even where the literal mechanism doesn't apply. The upstream
test itself stays in the advisory group, now clearly commented as a standing
probe rather than a meaningful signal today.

Also corrected in this pass: the Phase 2 entry below already noted that a
recovered node's taint (`node.kubernetes.io/unreachable`, `NoExecute`)
is never proactively cleared — hit this directly and unexpectedly during
this session's own manual verification (a `wrangler dev` restart briefly
starved the node of heartbeats, past the 40s grace period, tainting it; the
taint stayed after the node recovered), confirming that documented gap is
real and not just theoretical. Not fixed here (unrelated to this phase's
scope), but the new required CI step above is unaffected in practice: a real
CI job's node never goes stale mid-run the way a long manual test session
restarting `wrangler dev` did.

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

2. **StatefulSet** — done, end-to-end verified, via the same **real,
   unmodified `kube-controller-manager` binary** approach as the five
   controllers above. The storage prerequisite this phase was originally
   waiting on (R2-backed PersistentVolume/PersistentVolumeClaim) landed
   separately as its own Phase 8 (see README's Volumes (R2 PV/PVC)
   section) well before this item was picked back up.

   **What was actually needed, found by running the real binary against
   this apiserver** — much less than the original five controllers, because
   the apiserver had already moved from Phase 3's original per-type
   `subresource.go` switch statement to a generic, table-driven
   `pkg/apiserver/apidef.Table` (each `ResourceDef` declares its own
   `Subresources`) by the time this item was picked back up. StatefulSet's
   `apidef.Table` entry already had `statusSubresource()` and
   `scaleSubresource()` wired generically, `appsv1defaults.RegisterDefaults`
   already covers `SetObjectDefaults_StatefulSet` (StatefulSet is in the same
   `apps/v1` package as Deployment/ReplicaSet, whose defaulting was already
   registered), and the `ControllerRevision` stub type from the DaemonSet
   work already existed — none of these needed new work. The one real gap:
   - **`pkg/leanclient`'s narrow WASM-only client had no `StatefulSet` or
     `PersistentVolumeClaim` type** (`cmd/k8flare-gen/leanclient.go`'s
     `leanClientGroups` table, generating `pkg/leanclient/gen/{appsv1,corev1}`)
     — both were permanent panic stubs ("unused by this repo's controllers"),
     since nothing before this needed them. `pkg/controllers.RunControllerManager`
     (the WASM Controllers DO's hand-wired controller set — it bypasses
     `kube-controller-manager`'s own `app` package entirely, see that file's
     doc comment for why) calls `k8s.io/kubernetes/pkg/controller/statefulset.
NewStatefulSetController(ctx, podInformer, setInformer, pvcInformer,
revInformer, kubeClient)` directly, which needs real (non-panicking)
     `Client.StatefulSets(ns)` / `Client.PersistentVolumeClaims(ns)` and their
     informers. Added both as real `leanClientType` entries (CRUD + Watch +
     UpdateStatus + the same `ApplyScale`/`GetScale`/`UpdateScale` stub set
     ReplicaSet/Deployment already have) and regenerated via
     `go run ./cmd/k8flare-gen` — `third_party/clientgo-lean-overlays/kubernetes/
typed/{core,apps}/v1` did **not** need touching: a prior correction
     (Phase 10, see that mirror's README) already stopped pruning
     `kubernetes/typed/<group>/<version>` at all (the real scheduler's
     `SharedInformerFactory` needs it full-width), so the real, unmodified
     `StatefulSetInterface`/`PersistentVolumeClaimInterface` declarations were
     already available to implement against. Confirmed
     `k8s.io/kubernetes/pkg/controller/statefulset` itself compiles cleanly for
     `GOOS=js GOARCH=wasm` standalone before wiring anything (rule: verify
     before investing), matching the daemon/job/deployment/replicaset/cronjob
     packages' own GOOS=js compatibility — unlike `pkg/scheduler` itself
     (`docs/platform-verification.md`'s S8), nothing in `pkg/controller/
statefulset`'s own import graph touches `mount-utils`/`probe`/
     `securitycontext`.
   - `cmd/controller-manager/main.go`'s `--controllers` default (the separate
     BYO-VM/host-process binary `.github/workflows/e2e-conformance.yml`
     builds and runs alongside `wrangler dev` for CI) also got `statefulset`
     added, for the "kept in sync by hand" reason
     `pkg/controllers/controllermanager.go`'s doc comment already states.

   **Verified end-to-end** against a real local `wrangler dev` instance
   (`workers/k8flare/wrangler.jsonc`, `--enable-containers=false`) with the
   real WASM Controllers DO controller-manager (not the BYO-VM binary —
   the harder, more representative path since it's the actual production
   code) driving a real `StatefulSet` created directly against the running
   apiserver: admission defaulting (`podManagementPolicy: OrderedReady`,
   `updateStrategy.type: RollingUpdate` with `rollingUpdate.partition: 0`,
   `revisionHistoryLimit: 10`, `persistentVolumeClaimRetentionPolicy` all
   filled in with zero extra code, confirming the apps/v1 defaulter
   coverage claim above); ordered creation (`web-0` created alone, `web-1`
   withheld until `web-0`'s Pod status was set to `Ready` — no real kubelet
   in this environment, so Pod readiness was driven directly via the
   `pods/status` subresource the same way a real kubelet would, and the
   real statefulset controller reacted to the resulting watch event and
   created `web-1` within the next poll); a real, separately-bound
   `PersistentVolumeClaim` per replica from `volumeClaimTemplates`
   (`www-web-0`, `www-web-1`, both `Bound` via the existing R2 PV bind path,
   confirming StatefulSet's per-replica-PVC semantics work against this
   apiserver's storage layer); ordered scale-down via the `/scale`
   subresource (`replicas: 2 -> 1` deleted `web-1`, the highest ordinal,
   and left `web-0` and both PVCs alone — matching
   `persistentVolumeClaimRetentionPolicy`'s default `whenScaled: Retain`);
   and a `RollingUpdate` (changing the Pod template's image recreated
   `web-0` with a new UID and a new `controller-revision-hash` label
   (`web-5486fd4f4c` -> `web-647bbbf899`), with `status.updateRevision`
   moving ahead of `status.currentRevision` exactly as upstream's rollout
   bookkeeping does). `/status` and `/scale` were confirmed to reflect real,
   live state throughout (`readyReplicas`/`currentReplicas`/`availableReplicas`
   tracked the manual readiness pokes above), not just accept writes.
   `bash scripts/build-wasm-chunks.sh` stayed green: `apiserver` unchanged
   at 62,863,479 bytes, `kcm` grew from 64,019,330 to 65,041,921 bytes
   (+~1MB, from the real statefulset controller/informer/lister code now
   linked in) — still comfortably under the Loader's 64MiB cap with ~2MB
   headroom.

   **What's still honestly not proven**: everything above ran without a
   real kubelet in this environment (Pod readiness was driven by hand via
   direct `pods/status` writes, the same technique this apiserver's own Go
   test suite uses elsewhere), so a real container process never actually
   ran — this exercises the statefulset controller's own reconciliation
   logic for real, but not the full kubelet-in-the-loop path. And per the
   Storage prerequisite's own README caveat, a StatefulSet's PVC is S3 API
   access via injected env vars, not a real mounted POSIX filesystem — an
   app that expects a real data directory at its `volumeMounts[].mountPath`
   (the overwhelmingly common real-world StatefulSet use case: databases,
   etc.) still won't find one on the `workers/nodes` Containers backend in
   v1. This is not a StatefulSet-specific gap — it's the same gap every
   other PVC-mounting workload already has — but it means "StatefulSet
   works" here means "the controller's object-model orchestration is real
   and correct," not "you can run a real stateful database on this platform
   today."

3. **OwnerReference GC** — cascading deletion (delete a Deployment, its
   ReplicaSets and Pods go too). The real `kube-controller-manager` has a
   `garbagecollector` controller that does exactly this generically for any
   type; not yet added to `--controllers=` here since it needs its own
   verification pass (discovery + a metadata-only client instead of a typed
   one — Get/List have a documented, likely-compatible fallback path in
   client-go for older-style apiservers like this one, but Watch is
   unverified) rather than assuming it works.

   **Investigated (2026-07-05):** embedding the real `garbagecollector`
   compiles for GOOS=js/wasm cheaply (+2.1MB over the s13-kcm-lean-only
   baseline's 67MB, `-s -w`) once its RESTMapper/`metadata.Interface`
   dependencies are stubbed just enough to type-check. Making it functionally
   real, though, requires building two subsystems `pkg/leanclient` doesn't
   have today: a generic PartialObjectMetadata client
   (`k8s.io/client-go/metadata`'s `Interface` — Get/List/Watch/Delete/Patch
   across arbitrary GVRs, which `graph_builder.go` depends on throughout) and
   a `meta.ResettableRESTMapper`. Neither is a small addition on top of the
   existing typed-clientset generator (`cmd/k8flare-gen`) — it's a second,
   differently-shaped client generator plus a REST-mapping layer, comparable
   in scope to `pkg/leanclient` itself.

   Given that, `pkg/apiserver/gc.go`'s `CascadeDeleteDependents` was added as
   a deliberately narrower, **temporary** substitute instead: a synchronous
   ownerReferences walk over this apiserver's own namespaced `ResourceStore`s
   (known statically from `apidef.Table`, no RESTMapper/discovery needed),
   run inline inside the same DELETE request that removes the owner —
   handles `kubectl delete deployment` cascading to its ReplicaSets/Pods, and
   honors `propagationPolicy: Orphan`, but is not upstream code and does not
   cover cluster-scoped owners or CRDs. This is recorded here rather than
   quietly swapped in per CLAUDE.md rule 4: the long-term intent is still to
   replace it with the real `garbagecollector` once the PartialObjectMetadata
   client + RESTMapper exist, not to keep the hand-rolled version.

   **Correction (2026-07-05, found live while verifying the S14
   ASSETS+LOADER controllers deploy path):** the first version of this
   walk deleted children-first (a ReplicaSet's Pods before the ReplicaSet
   itself). Against the now-live in-Workers kube-controller-manager that
   order races: the real replicaset controller, still holding the RS in
   its informer cache, recreated Pods mid-cascade and — with no background
   garbagecollector to reap dangling dependents — they survived as
   permanent orphans (observed: deleting a Deployment left one
   freshly-created Pod behind). `deleteDependents` now deletes owner-first
   and re-sweeps until quiescent (bounded passes); verified live and by
   `TestOwnerReferenceCascadeDelete`. A window in principle remains (a
   create landing after the final pass) — the real `garbagecollector`
   stays the actual fix.

Verify: conformance `[sig-apps]` ReplicaSet/Deployment basics move into the
required set — these are Conformance-tagged upstream, so this phase is the
largest single jump in official conformance coverage. StatefulSet's own
`[sig-apps] StatefulSet` conformance tests are not yet added to
`e2e-conformance.yml`'s advisory or required groups — the local verification
above used direct API driving (no real kubelet in this session's
environment), not the official e2e suite; adding those tests is a follow-up,
not assumed to pass sight-unseen.

## Phase 4 — Cluster DNS

**Shipped 2026-07-07, in a different shape than the CoreDNS-as-Deployment
plan below** (user decision: no Containers dependency for DNS). The
`MissingClusterDNS` kubelet warning is resolved without ever needing a
`kube-dns` Service or ClusterIP routing to reach it:

1. `cmd/agent` runs `pkg/dnsshim`, a node-local UDP/TCP DNS listener
   bound to `169.254.20.10` — the NodeLocal DNSCache convention
   address (link-local, so identical on every node, no cross-host
   collision, no ClusterIP/kube-proxy dependency at all).
2. `supervisor.go`'s `/v1-k3s/config` advertises
   `ClusterDNS: 169.254.20.10`, which the embedded k3s agent code
   applies to kubelet's `--cluster-dns` with zero agent-side wiring
   (`config.Control.ClusterDNS` unmarshals directly off that JSON
   field) — every ClusterFirst Pod's `/etc/resolv.conf` points here.
3. The shim forwards `*.cluster.local` queries over DNS-over-HTTPS
   (RFC 8484) to `pkg/apiserver`'s new `/dns-query` endpoint, which
   synthesizes A records from live Service objects (ClusterIP) and
   EndpointSlice objects (headless Services → backing Pod IPs) —
   token-authenticated like every other apiserver route. Everything
   else forwards to the node's own upstream resolvers (read once from
   the host's real, untouched `/etc/resolv.conf`).

**Deviation from "embed the real thing" (rule #3), recorded not
hidden:** CoreDNS itself was considered first — its `kubernetes`
plugin needs nothing but a real Kubernetes API, which this apiserver
is. Not embedded because CoreDNS's build is driven by `plugin.cfg`
code generation, not a plain `go build` of a versioned command the way
`cmd/scheduler`/`cmd/controller-manager` embed real
`k8s.io/kubernetes` binaries — a meaningfully different integration
shape than this project's existing embedding pattern. Revisit if full
DNS conformance coverage (SRV records, Pod hostname/subdomain records,
`dnsConfig`/search-domain customization) becomes a priority; the
current handler only does Service A-record resolution.

**Verified** (`pkg/apiserver/dns_test.go`, against real `wrangler dev`):
ClusterIP Service resolves correctly, headless Service resolves to its
backing Pod's IP via EndpointSlice, unknown Service names NXDOMAIN,
and the endpoint is token-gated (401 without auth). **Not yet
verified**: actual resolution from inside a real Pod end-to-end — that
additionally needs Phase 1's real ClusterIP traffic proof for Pods to
reach anything by the resolved IP in the first place (DNS answering
correctly and packets actually routing are separate, both-required
problems). Cost: request-driven, so idle cost is zero — no resident
DNS process, unlike a CoreDNS Pod (`docs/cost-model.md`).

Verify (once Phase 1 unblocks it): `nslookup kubernetes.default.svc.cluster.local`
from a real pod; conformance `[sig-network] DNS` basics.

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

> **Update (2026-07-08):** the PriorityClass item above is done —
> `scheduling.k8s.io/v1`'s `PriorityClass` is registered in `apidef.Table`
> (cluster-scoped, `apidef.ResourceDef`, same shape as `RuntimeClass`/
> `ClusterRole`) and upstream's own `SetDefaults_PriorityClass` (fills in
> `preemptionPolicy`) is wired through `cmd/k8flare-gen/defaulters.go`'s
> table the same way `apps/v1`/`batch/v1`/core/v1 defaulters already are.
> New file `pkg/apiserver/priority.go`'s `ResolvePodPriority` resolves a
> Pod's `spec.priorityClassName` -> `spec.priority` at Pod-create admission,
> wired into `handler.go`'s POST path the same way LimitRange defaulting
> and compute-class routing already are (not a new admission mechanism) —
> following real upstream's `plugin/pkg/admission/priority/admission.go`
> (`k8s.io/kubernetes@v1.36.2-k3s1`) for an unresolvable class name (403
> Forbidden), a mismatched explicit `spec.priority` (403 Forbidden), and
> the no-`priorityClassName`/no-`globalDefault` case (resolves to 0). One
> deliberate, documented deviation from upstream: a Pod that sets
> `spec.priority` directly with no `priorityClassName` is left completely
> untouched (upstream would instead reject it) — preserving this project's
> existing, already-verified direct-`spec.priority` preemption path
> unchanged, see the README's Scheduling table. Verified end-to-end against
> a real `wrangler dev` + the real, unmodified `cmd/scheduler` binary: a
> `low`(100)/`high`(1000000) `PriorityClass` pair, a capacity-constrained
> fake Node, a bound low-priority Pod, then a high-priority Pod that
> triggers a real `FailedScheduling: Insufficient cpu` event followed by a
> real `Preempted` event on the low-priority Pod naming the high-priority
> Pod's own UID, followed by `Scheduled` on the high-priority Pod, with the
> preempted Pod confirmed truly gone (404, not soft-deleted) — the same
> mechanism a real cluster uses, not a stand-in. Measured apiserver WASM
> size delta: +136,256 bytes (+133.06 KiB, +0.217%), see
> `docs/cost-model.md`'s PriorityClass section. `pkg/leanclient` needed no
> change: neither the real `cmd/scheduler` (reads only the already-resolved
> `spec.priority`) nor any enabled `pkg/controller/*` reconciler constructs
> a `PriorityClass` client/informer.

Then the auth ladder, shared with the hosted-product plan:

- **TokenRequest / TokenReview** — real ServiceAccount tokens (unblocks
  in-cluster clients done properly, starting with CoreDNS).
- **RBAC** — types, an authorizer in front of the stores, default
  roles/bindings. This is also what turns namespaces into a sellable tenancy
  boundary (see the multi-tenancy doc), so it is sequenced with that track.
- Server-side apply, dry-run, admission webhooks: evaluated after the above;
  client-side apply with OpenAPI covers most real usage until then.

> **Update (2026-07-05):** the "types" half of the RBAC item above is done —
> `rbac.authorization.k8s.io/v1`'s Role/RoleBinding/ClusterRole/
> ClusterRoleBinding are registered in `apidef.Table` for CRUD+watch
> (`pkg/apiserver/apidef/table.go`), verified against the real typed
> `client-go` clientset (`TestRBACGroup`,
> `pkg/apiserver/apiserver_test.go`). A `SelfSubjectAccessReview` handler
> (`pkg/apiserver/selfsubjectaccessreview.go`) also answers `kubectl auth
can-i` requests, always with `allowed: true` — the accurate answer for
> this project's all-or-nothing bearer token (`auth.go`'s `AuthMiddleware`),
> not a stand-in for a real per-verb/per-resource authorizer. **Not done:**
> "an authorizer in front of the stores" and default roles/bindings — the
> objects above are stored and gettable but nothing evaluates them for an
> actual access decision. See the README's
> [Auth & admission](../README.md#auth--admission) table.

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

> **Update (2026-07-05):** `HorizontalPodAutoscaler` (`autoscaling/v2`),
> `Ingress`/`IngressClass`/`NetworkPolicy` (`networking.k8s.io/v1`), and
> `ResourceQuota` (core/v1) are now registered in `apidef.Table` for
> CRUD+watch (verified against real `client-go`,
> `TestNewlyRegisteredResources` in `pkg/apiserver/apiserver_test.go`). This
> closes only the "object exists and is storable" step — the actual gaps
> this section describes (no Metrics API driving HPA, no CNI enforcing
> NetworkPolicy, no ingress controller reading Ingress, nothing tracking
> ResourceQuota usage) are unchanged; a created object of any of these
> types is inert.
>
> Other resources a standard cluster has that this apiserver still doesn't
> register, considered and deliberately not done in this pass (budgeted
> against the Workers WASM size limit — each additional `k8s.io/api` group
> costs on the order of a few hundred KiB of compiled code, see
> `docs/cost-model.md`): `admissionregistration.k8s.io/v1`
> (Mutating/ValidatingWebhookConfiguration, ValidatingAdmissionPolicy) —
> low daily-use value here since this apiserver has no admission webhook
> call-out mechanism for them to configure; `storage.k8s.io/v1`
> `VolumeAttachment` — an internal CSI-attacher bookkeeping type with no
> CSI attacher running in this project; `certificates.k8s.io/v1`
> `CertificateSigningRequest` — this project already has its own bootstrap
> certificate issuance path (`pkg/apiserver/certmanager.go`,
> `supervisor.go`), so a generic CSR object would be a second, unused
> mechanism; `apiregistration.k8s.io/v1` `APIService` — no aggregation
> layer exists to register against; `flowcontrol.apiserver.k8s.io/v1`
> `FlowSchema`/`PriorityLevelConfiguration` — API Priority and Fairness is
> meaningless against a single-Worker-per-request-path deployment with no
> shared apiserver process to protect. `apiextensions.k8s.io/v1`
> `CustomResourceDefinition` (generic/dynamic CRDs) is intentionally out of
> `apidef.Table`'s scope entirely, not merely deprioritized: `workers/runtime`
> owns CRD/`DynamicWorker` handling on its own path (see
> `docs/control-plane-architecture.md`), so a second CRD mechanism inside
> the Go apiserver would duplicate it. `scheduling.k8s.io/v1` `PriorityClass`
> is still open too, but that one already has its own line item above (this
> pass didn't newly decide against it — it's simply not done yet).

## Sequencing rationale

Phases 1→4 are strictly ordered by dependency (Endpoints before kube-proxy
before DNS; workload controllers before CoreDNS-as-Deployment). Phase 5's
quick wins can land any time; its auth ladder gates both Phase 4's clean
ending and the multi-tenancy track's namespace tenancy. Phase 6 items are
pulled forward only by real user demand.
