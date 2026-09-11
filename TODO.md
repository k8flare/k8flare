# TODO — the road from "alpha" to "a team can run this"

The bar this file is written against: **a team can put a workload they care
about on k8flare and not be paged about the control plane.** Nothing here is
style or polish; every item is something that, left alone, either loses data,
hides an outage, or makes a green CI run mean less than it appears to.

Source: an adversarial product review on 2026-09-11 against `main` @ `43011df`,
plus the open items already recorded in `docs/platform-verification.md`
(S28–S36). Where a claim has evidence, the evidence is cited; where the review
declined to judge for lack of reading, that is marked too.

The review's one-sentence verdict, kept here because it sets the priorities:

> k8flare's correctness rests on Cloudflare runtime behaviours that only
> manifest in production, and nothing in the repo observes production. The
> deployed control plane was broken from 2026-07-26 to 2026-09-09 while every
> local gate stayed green.

Status legend: `[ ]` not started · `[~]` in progress · `[x]` done (link the
commit) · `[!]` blocked or deliberately deferred (say why).

---

## P0 — blocks "a team can run this"

### P0-1 `[x]` Observe production, not just `wrangler dev`

**Problem.** There is no signal that the deployed control plane works. S30 sat
undetected for six weeks: DO-origin Loader calls failed, dynamic workers
reloaded every minute, `kubectl scale` was ignored for 3+ minutes, and the
safety-net alarm never parked — a standing violation of cost invariants #1/#3.
Every local gate was green throughout, because `cost-gate.yml` and every test
lane run against `wrangler dev`, which does not reproduce the production
runtime semantics that broke (`pkg/cfruntime/cloudflare/window.go` says so in
its own doc comment; the `PUMP_WINDOW_DROP_CLOSE` knob in
`packages/k8flare-worker/src/controllers/index.ts` exists only because dev
cannot produce the fault).

**Do.** A scheduled synthetic convergence probe against a real deployment:
create a Deployment → wait for a Pod to reach Running → delete it → assert the
cluster parks within N minutes → assert zero Worker/DO requests for the
following M minutes. Alert on failure. It must run on a schedule against the
real deployment, not in `wrangler dev`.

**Acceptance.** The probe fails when pointed at a deployment with the S30
defect reintroduced (verify by deploying a version with the fix reverted, or by
an equivalent fault injection), and passes against current `main`. Failure
reaches a human without anyone watching a dashboard.

**Done 2026-09-11**, except the scheduled half. Run end to end against the real
deployment: readyz passed with every component reported by size and sha256 and
the verdict cache visibly bounding the cost; a Pod reached Running in 5m13s on
a demand-started NodeVM; the scale to two converged in 2m47s; the workload was
gone 8m05s in. It then refused to assert idle cost because its own
demand-started nodes were still attached -- correct behaviour, wrong timing,
now fixed by waiting for them to detach (they took about two minutes).
**Still open**: `prod-probe.yml` has never run on a schedule, because that
needs a maintainer to set the secrets it documents. Until then nothing watches
production between manual runs.

### P0-2 `[x]` `/healthz` reports nothing about health

**Problem.** The gateway answers `/healthz`, `/livez`, `/readyz` without
touching storage or any component
(`packages/k8flare-worker/src/gateway/index.ts:45`), deliberately, to keep the
probe cheap. The consequence is that an operator's liveness check cannot
distinguish "control plane serving" from "control plane wedged" — which is
exactly the state production was in for six weeks.

**Do.** Keep a zero-cost liveness path, and add a real readiness path that
exercises storage and reports per-component status (apiserver, kine/DO, the
resident workers that are supposed to be loadable). Bound its cost and its
frequency so it cannot itself violate the idle-cost invariant; an endpoint that
only does work when asked is fine, a self-polling one is not.

**Acceptance.** With a deliberately broken Loader path, readiness goes
not-ready while liveness stays cheap. Documented in `docs/admin-guide.md` with
the cost per call.

### P0-3 `[x]` UID/resourceVersion preconditions are dropped on DELETE

**Problem.** `ResourceStore.upstreamMarkForDeletion`
(`pkg/apiserver/upstreamregistry.go:263-266`) constructs a fresh
`&metav1.DeleteOptions{PropagationPolicy: &policy}` and passes that to the
upstream store, discarding the caller's `Preconditions`. A client that sends a
UID-guarded DELETE — which is how clients avoid deleting a *recreated* object
with the same name — has that guard silently ignored. Upstream's own GC sends
UID preconditions. This is a data-loss shape, not a latency bug. Recorded as
open in S33.

**Do.** Thread the caller's full `DeleteOptions` (preconditions, grace period,
dry-run) through to the upstream store. Check every other place this repo
rebuilds an upstream options struct by hand for the same class of bug.

**Acceptance.** A test that deletes with a stale UID precondition gets `409`
(or upstream's exact behaviour) and the object survives; the same DELETE with
the correct UID succeeds. Plus an audit note listing the other options structs
checked.

### P0-4 `[~]` Resident controllers are discontinuous across pump windows

**Design proposed, awaiting review:** `docs/pump-window-design.md` (2026-09-11).
It recommends keeping bounded WASM execution and separating physical
connection lifetime from logical watch continuity — resourceVersion-resumable
watches first, then durable change notification from storage — and explicitly
refuses to promise that resumable watches alone retire the four
`gracefuldelete.go` guards. Seven stages, each independently shippable, each
with the production measurement that decides it.

**Problem.** The deepest issue and the reason the other symptoms keep
reappearing. kcm/gc/sched run as resident WASM pumped in bounded windows;
informer watches are torn down at every window boundary and re-established on
the next poke. Downstream symptoms already measured: node recovery after a
heartbeat outage takes ~5–6 minutes to clear `unreachable` taints (S31
addendum 2), the scheduler transiently binds Pods onto a node carrying
`unreachable:NoSchedule`, foreground deletion needed four separate apiserver-side
guards to stay inside a 90s budget, and local measurement still shows roughly
1 run in 5 near that budget (S36).

**Do.** This needs a design pass before code. Options to evaluate against the
cost invariants, with measurements: longer or overlapping windows; keeping the
reflector's watch alive across a window boundary; a resume-from-resourceVersion
path so a re-watch does not re-list; or accepting discontinuity and making the
controllers' work idempotent-and-resumable by design. Write the design down and
get it reviewed before implementing (CLAUDE.md's "design first").

**Acceptance.** Taint clearing after a node returns is bounded and measured in
production; no scheduler binding onto `NoSchedule`-tainted nodes; the
foreground-deletion guards in `gracefuldelete.go` can be reduced rather than
added to. Numbers recorded, production-measured.

---

## P1 — needed before the conformance story is credible

### P1-1 `[ ]` The required gate does not exercise the headline feature

**Problem.** `e2e-conformance.yml`'s REQUIRED variant is `host`: native
kube-scheduler and kube-controller-manager, with the WASM kcm/sched switched
off (`CM_DISABLED` / `SCHED_DISABLED`); only the gc dynamic worker runs. The
README sells resident WASM controllers built from real upstream packages, and
that configuration is **advisory-only** in CI. Reporting "required is green"
therefore says much less than it sounds like it does.

**Do.** Promote the dw variants to required — but only behind P1-2's evidence.
Until then, say so plainly wherever CI status is described (README,
CONTRIBUTING, and any status badge).

**Acceptance.** Either the dw variants are required, or every place that cites
the required gate states which configuration it covers.

### P1-2 `[ ]` Prove the dw variants are stable, don't sample-check them

**Problem.** The dw variants were red in one of the last three runs
(`34445918793`: kcm-dw hit a client-side connection reset, sched-dw blew the
90s foreground-deletion budget) and green in the two after
(`34484825880`, `34491769003`). n=2 is not stability, and local measurement
still shows ~1 in 5 near budget.

**Do.** Run the dw variants repeatedly — target ~10 consecutive green — and
treat any red as a defect to root-cause, not to re-run. Watch Actions quota;
batch or schedule rather than hand-dispatching.

**Acceptance.** ~10 consecutive green dw runs, or a root cause for each red.

### P1-3 `[x]` The most platform-fragile code has no unit tests

**Problem.** Every `_test.go` lives in `pkg/apiserver` and drives a real
`wrangler dev`. `pkg/cfruntime` — pump windows, the JS boundary, promise
lifetimes, the code that caused S31 and S34 — has **zero** unit tests. There
are **zero** TypeScript tests. A regression in the window registry is caught
only by a 5-minute end-to-end lane, if at all.

**Do.** Unit tests for `pkg/cfruntime/cloudflare` (window open/close/expiry,
`CurrentWindow` racing a close, abandoned promises, `WithLiveBinding` when no
window is open) and for the TS glue that carries logic (the Controllers DO's
poke/park policy, the storage DO's `afterWrite` predicates, watch stream
lifecycle).

**Acceptance.** The S31 and S34 fault shapes are each covered by a unit test
that fails when the fix is reverted. TS tests run in `ci.yml`.

**Done 2026-09-12, and the acceptance criterion was checked by actually
reverting each fix** in a throwaway worktree rather than by reading the tests:

| Fix reverted | Tests that failed |
|---|---|
| S31 — `EnvFromContext`'s fallback to the open pump window (`pkg/cfruntime/cloudflare/env.go`) | `TestEnvFromContextFallsBackToTheOpenWindow`, `TestBindingFromContextResolvesOnTheOpenWindow` |
| S34 first shape — `toJSResponse` returning plain values instead of a `Response` | `TestToJSResponseReturnsPlainValuesNotAResponse` |
| S34 second shape — no body on the Fetch spec's null-body statuses | `TestToJSResponseSendsNoBodyForBodilessStatuses` |

Each mutation failed only its own tests, so they discriminate rather than
tripping on any change. Coverage now stands at 17 Go unit tests across
`pkg/cfruntime` (3), `pkg/cfruntime/cloudflare` window registry (9) and env
resolution (5), plus 50 TypeScript tests; `make test-ts` runs in `ci.yml`
(line 109). One of those TS tests was itself flaky and was fixed the same day
— it compared an alarm interval against the 600s ceiling using a timestamp
sampled before the call, so it failed by exactly 1ms whenever the call was
slow enough.

### P1-4 `[x]` Destructive DO migrations replay on deploy

**Problem.** `packages/k8flare-worker/wrangler.jsonc` carries `migrations`
including delete+recreate cycles, `wrangler deploy` applies whatever is in the
file, there is no backup path, and `docs/adopter-quickstart.md` mitigates this
by telling operators to `git diff` before upgrading. That is a footgun handed
to the user.

**Do.** Make a destructive migration impossible to apply by accident: gate it
behind an explicit opt-in, or move already-applied migrations somewhere they
cannot be re-run, or provide an export/import path so state loss is
recoverable. Document the recovery story.

**Done 2026-09-11** for the first half. `npm run check:migrations` hashes the
block against `migrations.sha256`, runs in CI and ahead of `make deploy` /
`npm run deploy`, and refuses when it changed. Verified both directions: adding
a `deleted_classes` tag exits 1 with the recorded and actual hashes, the
override env var passes it, reverting passes again. It does not cover a direct
`wrangler deploy`, which is documented.

**Backup added 2026-09-11**: `cmd/k8flare-backup dump|restore` walks discovery
and round-trips every served object. Verified against the real deployment — 21
objects dumped, the namespace deleted, then restored with ConfigMap data,
labels and Deployment replicas intact. **Still open**: it cannot cover the CA
keypairs or the token vault, which live in Durable Object facets the
Kubernetes API does not serve, so a restore into a fresh deployment returns
workloads but not cluster identity. Nothing schedules it.

**Acceptance.** An operator who pulls and deploys cannot lose cluster state
without an explicit, separate action. A documented way to export and restore a
cluster's DO state.

### P1-5 `[x]` User-facing docs describe intent, verification docs describe reality

**Problem.** `docs/admin-guide.md` §1 says nothing runs when idle — false in
production for six weeks. §5 documents ~65k alarms/month on an unconverged
cluster with the cause "未特定", which is a standing cost-invariant #3
violation living in the operator guide. `README.md`'s doc table says
platform-verification covers "(S1–S26)"; the file runs to S36.

**Do.** Reconcile every user-facing claim against what the verification docs
actually establish, and mark anything verified only in `wrangler dev` as such.
Fix the stale index. Add a "currently known broken" page an adopter can read in
two minutes.

**Acceptance.** No user-facing claim is contradicted by
`docs/platform-verification.md`. A known-issues page exists and is linked from
the README.

### P1-6 `[x]` `platform-verification.md` has trustworthy history and an unusable present

**Problem.** 5,663 append-only lines, no current-state index. Rule 4 (never
rewrite a correction away) is right and should stay, but a newcomer cannot
learn what is true today without reading the whole archaeology layer.

**Do.** Add a current-state summary at the top — what is verified in
production, what is verified only locally, what is known broken — each line
pointing at the section that establishes it. The history stays untouched below.

**Acceptance.** A reader can answer "what works today?" from the first screen.

---

### P0-6 `[ ]` Stage 0 of the pump-window design (instrumentation and cost contract)

Attempted 2026-09-11 and **discarded**. Two delegated agents were each cut off
by provider rate limits mid-task and left unverified work; the salvaged result
passed `vet` and the unit lanes but killed the Worker at startup — every
wrangler-lane test failed with `connection refused`, so `wrangler dev` never
came up. The changes had also strayed past Stage 0 into the clientgo-lean
mirror and `pkg/controllers/restconfig`, which is Stage 2 territory.

Redo it scoped tightly: the three-boundary instrumentation (commit → informer
observed → controller acted) attributable to a pump window and component, with
no always-on cost, plus the cost-model entries and a probe-traffic
discriminator. Note that S37 removed the urgency: the node-recovery latency
this instrumentation was meant to localise is no longer a defect.

## P2 — known defects and accidental complexity

### P2-1 `[~]` `gracefuldelete.go`'s guards are compensating for the platform

Four hand-written guards (`RejectCreateWithTerminatingController`,
`refuseForegroundFinalize`, `FinishUnblockedForegroundOwners`,
`sweepOrphanStragglers`), each full-listing every namespaced store per
create/finalize, each with a doc comment ending "upstream needs neither
guard". Rule 3 is satisfied in letter — the real GC is unmodified — while its
cost migrates into hand-written apiserver code compensating for
platform-induced informer lag. Revisit after P0-4; the guards should shrink,
not grow. Also measure their rows-read cost, which is currently unmeasured.

**Measured 2026-09-11, and the answer is no — not yet.** The experiment
disabled `FinishUnblockedForegroundOwners` and repeated the required
garbage-collector focus locally. Five runs passed, then the wall time climbed
253s → 585s → 899s and run 6 died in `BeforeSuite` at the 900s timeout.

The decisive number came from the machine after the experiment stopped: 66
minutes later, with no test running, the Worker was still serving **451
requests per minute**. 1260 of the last 1289 were `GET
/api/v1/namespaces/gc-8627/pods/<name>` returning 404, against a namespace
that no longer exists. The real garbage collector retries forever, and nothing
completes the owner it is blocked on — which is precisely the job the guard
does (S36).

That is a cost-invariant #1 violation (~650k requests/day on a cluster that
can never converge), not a latency regression. **Removal stays blocked behind
P0-4**; re-measure once pump windows are continuous. Full write-up and the two
caveats (the run was killed by `timeout` so framework cleanup never ran; the CI
failure was a 90s timing budget that a fast laptop does not reproduce) are in
`docs/platform-verification.md` S43.

### P2-2 `[x]` `pendingPing` asymmetry in the storage DO

`packages/k8flare-worker/src/storage/index.ts:261`: `afterWrite` sets
`pendingPing:controllers` regardless of whether the `CONTROLLERS` binding
exists, but `pingControllers` does not clear it when unbound, so an unbound
config re-arms the safety-net alarm every 60s forever. `pingNodes` clears it.
Pre-existing, recorded in `docs/custom-code-inventory.md` §5.

### P2-3 `[x]` Event POSTs 400 on first write

The real kcm's event broadcaster gets `400 "Object 'Kind' is missing"` on its
first `POST events` because this apiserver does not infer kind from the URL
path the way upstream does. Retries succeed, so events are only partly lost.
Recorded in S32.

### P2-7 `[x]` Pod-on-Containers does not work in production

Found by the first scheduled run of the production probe (S39 and its
correction). A Pod annotated `k8flare.com/compute=containers` is admitted
correctly and the scheduler is poked — `CFContainersScheduler` answers `ok` —
but every `NodeVMSmall` request ends `canceled` with no log and no exception,
no node ever registers, and the Pod stays `Pending` with
`Unschedulable: no nodes available`. The same path worked earlier the same day,
so it is a regression or intermittent, not unimplemented. Two hypotheses were
tested and disproved: a `provisioning` container application (all three are
`ready`), and the scheduler never being reached (it is, the tail filter was
too narrow).

The probe no longer depends on this capability, so it is not blocking daily
monitoring — but Pod-on-Containers is a headline feature that currently does
not work.

**Fixed 2026-09-11** (S39 続報). `reconcile()` issued `stub.up()` once from the
poke's detached context and never retried, because the claim it persists first
made every later pass skip the Pod. An abandoned promise neither resolves nor
rejects, so nothing surfaced. Tracking `started` separately from `bound` lets
the existing 15s safety-net alarm re-issue the boot. Verified in production:
node registered at t+100s, Pod Running at t+160s, where before it stayed
Pending indefinitely. **Still open**: the mechanism behind the `canceled`
outcome itself is not identified, and `wrangler dev` cannot reproduce it — it
does not abandon detached DO subrequests, which is why no gate caught this.

### P2-4 `[x]` k3s agent's remotedialer tunnel 401-loops

**Done 2026-09-11 (S38).** The agent dials with no Authorization header at all
— pinned k3s calls `ConnectToProxyWithDialer(ctx, wsURL, nil, ...)` and relies
on an mTLS client certificate that Cloudflare strips — so the gateway's door
rejected every attempt: about 28,800 billed requests per day per attached node.
`/v1-k3s/connect` is now on the unauthenticated allowlist, which is what the
stub's doc comment always claimed. Measured in production: 401 retries 0,
`Remotedialer connected to proxy` once, 3m41s after the node started.
**Still open**: it admits an unauthenticated socket, and holds it in the shell
Worker rather than a hibernating Durable Object. The tunnel is unused, so the
right answer is for the agent not to dial at all; k3s has no switch for that.

### P2-5 `[x]` `PUMP_WINDOW_DROP_CLOSE` is a test knob in production code

Added so `wrangler dev` could reproduce a production-only fault
(`packages/k8flare-worker/src/controllers/index.ts`). Keep it only if it is the
cheapest way to hold that regression; if so, document it as a test seam and
make sure it cannot be enabled in a real deployment by accident.

**Kept, and fenced.** It is the cheapest seam: the fault is that production
tears a poke's IoContext down before `ctx.waitUntil`'s timer runs, and
`wrangler dev` never does that (S31 E1), so three regression tests
(`gcmultiowner_test.go`, `kcmdw_test.go`, `ioctxprobe_test.go`) can only reach
the wedge by dropping the close deliberately. Each passes it per invocation as
`wrangler dev --var`, so nothing about it lives in a deployment.

What was missing was the guard. `packages/wasm-build/src/check-test-vars.ts`
now refuses to deploy if `wrangler.jsonc` declares any harness-only var, wired
into `npm run deploy`,
`make deploy` and `ci.yml` alongside the migrations check. Verified both ways:
it passes on `main` and fails with the var added. It does not — and cannot —
catch a deliberate `wrangler secret put` of the same name; the accident it is
built for is a test invocation's `--var` being copied into a config.

The guard covers three more names than P2-5 asked for, because the same
accident has the same consequence for all of them: `KCM_DISABLED`,
`SCHED_DISABLED` and `CM_DISABLED` are harness kill switches that let a host
process stand in for a resident controller. Checked 2026-09-11 that all four
appear only as per-invocation `--var` in test lanes and `e2e-conformance.yml`,
never as deployment configuration, so the guard cannot block a legitimate
deploy. Unlike the fault knob they do not corrupt behaviour, they remove a
controller -- the error message says so rather than calling them all fault
injection.

### P2-6 `[~]` `deps-k3s-update` is failing on `main`

Diagnosed 2026-09-11 (run 34169780304): the bump job pushes the branch fine —
`deps/k3s-v1.36.4-k3s1` is on the remote — and then `gh pr create` fails,
because the repository has "Allow GitHub Actions to create and approve pull
requests" off (`can_approve_pull_request_reviews: false`) and no
`DEPS_UPDATE_TOKEN` secret is set. The step now says exactly that instead of
failing opaquely.

**Needs a human decision**: either enable that repository setting (which also
permits Actions to approve PRs — a security consideration), or create a
`DEPS_UPDATE_TOKEN` with `pull-requests: write`. Until then the workflow will
keep going red weekly, correctly. There is also a real pending k3s patch bump
sitting unmerged on that branch.

---

### P0-5 `[x]` `?dryRun=` was ignored entirely

Found by the P0-3 audit and fixed in the same pass: the apiserver parsed
`dryRun` nowhere, so a server-side dry run created the object, allocated its
ClusterIP and ran the post-create effects. Now validated with upstream's
`ValidateDryRun` and threaded through to the store, with the side effects
suppressed. Left open here because only the create/update/patch paths were
covered -- delete, deletecollection and the subresources still need the same
treatment, and none of it is verified against a real `kubectl --dry-run=server`.

### P1-7 `[x]` `pkg/cfruntime`'s root package cannot be unit-tested

`handler_js.go`'s `init()` reads `globalThis.context.binding` at program start,
so merely adding a test file to the package panics with
`syscall/js: call of Value.Get on undefined`. The consequence is that S34's
first fault shape -- the `toJSResponse` fix that builds the Response in JS
rather than Go -- has no unit test. Making it testable is a restructure of the
package's initialisation, not a seam.

**Done 2026-09-11.** `init()` now calls `registerBinding()`, which returns
false instead of dereferencing an absent `globalThis.context.binding`, so the
package accepts test files. `toJSResponse` is covered in both directions:
rebuilding it as a real `Response` (S34's first fault shape) fails the suite
with "toJSResponse built a Response; it must return plain values so the
bootstrap can construct one in the request's own context".

**Still open**: the unit tests run in node, not workerd, so they cover logic and
promise/stream semantics but not input gates, real IoContext teardown or DO
storage semantics. `@cloudflare/vitest-pool-workers` would close that gap at
the cost of a dependency.

### P1-8 `[~]` A k3s patch bump is sitting unmerged

`deps/k3s-v1.36.4-k3s1` (from the weekly automation on 2026-09-07) moves the
pin from k3s v1.36.3 to v1.36.4 and the `k8s.io/*` staging replaces with it.
It never became a PR because of P2-6's repository setting, so it has been
sitting on the remote while `main` moved on. A dependency bump that carries
upstream fixes should not rot.

**Verified locally 2026-09-11** on `deps/k3s-136-4` (that branch with current
`main` merged in): `make vet`, `make check`, `tsc` clean; `make gen` produces
no drift; all five WASM chunks under the Loader cap (apiserver headroom
2,389KiB, down 177KiB from 2,566KiB); `make test-unit`, `test-apiserver`,
`test-kcm` and `test-clusterop` all pass.

**Not merged**: the Definition of Done is conformance, and Actions capacity is
exhausted (see below). Merge once `e2e-conformance.yml` has run green against
this branch.

**Local verification 2026-09-12** (GitHub Actions capacity is still exhausted,
so this is the substitute for the conformance gate):

- Current `main` merged in; `make check`, `make vet` clean.
- `make clean-wasm wasm` reproduced every chunk byte-identical to the branch's
  existing build, and all five are under the 64MiB Loader cap — apiserver has
  the least headroom at 2389 KiB.
- `make test` green: apiserver 69.9s, `TestKCMDynamicWorkerControlPlane`
  414.3s, `TestClusterOperatorLifecycle` 70.1s, 50 TypeScript tests.
- The **required** garbage-collector focus: `Will run 7 of 7579`, then
  **7 Passed / 0 Failed in 141s**.
- The baseline focus fails locally — but see the rule below before reading
  anything into that.

**Decision rule, written before the comparison came back.** The local harness
cannot run the `host` variant that gates baseline in CI (no host scheduler or
controller-manager process; `docs/development.md`), so a local baseline
failure is unattributed on its own. Running the same focus against `main`:

- main fails the same specs → environmental, merge on the strength of the GC
  focus and `make test`, and say plainly that the host baseline was not
  locally reproducible.
- main passes them → a real regression in the bump; do not merge.
- main fails one and passes the other → not an average. Inviolable rule #5
  applies: take a second sample of each before concluding anything.

### P1-9 `[x]` Two compiled binaries were committed by accident

`k8flare-backup` (34.0 MB) and `prodprobe` (34.6 MB, twice) were committed to
`main` on 2026-09-11 before `.gitignore` covered them. Untracked and ignored the
same day; the blobs remain in history.

**Decided 2026-09-11: do not rewrite history.** Measured rather than assumed —
a fresh clone is **45 MiB** today, and the three blobs are most of it, so a
rewrite would bring it to roughly 12 MiB. Against that, the project's own audit
trail cites **24 commit SHAs** in `docs/platform-verification.md` alone, and
rule 4 exists precisely so those references stay followable. Rewriting
invalidates every one of them, and every descendant SHA, to save 33 MiB on a
repository that nobody has cloned yet. That trade is not worth it.

Revisit only if the repository grows another accidental blob — at which point
one rewrite can clear them all, and should be done *before* the docs accumulate
more references, not after.

## Out of scope / deliberately not doing

- `[!]` Replacing the hand-written REST layer with upstream
  `k8s.io/apiserver/pkg/endpoints`: measured NO-GO. Linking it costs +3.5MB
  after `wasm-opt` against ~2.5MB of headroom under the Worker Loader's
  67,108,864-byte cap, and the cap was re-measured on 2026-09-10 and is
  unchanged. See S29 and S35. Revisit only if the SSA/fieldmanager closure
  that `genericregistry` imports unconditionally can be trimmed.

## Not assessed

The review explicitly declined to judge these for lack of reading, so their
absence from this list is not a clean bill of health: `docs/cost-model.md`
body, S28/S29, `docs/user-guide.md`, `docs/development.md`, the `nodes/` and
`clusters/` TypeScript subtrees, and `cmd/agent`.
