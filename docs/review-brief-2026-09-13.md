# Review brief — `fix/facet-write-durability`, 2026-09-13

Twenty-three commits, +4,456 / −2,697 across 67 files, not merged. Read this with
`git diff main...HEAD`.

This exists because a reviewer arriving cold at 4,000 lines reviews the diff;
a reviewer arriving at the five arguments reviews the work. What follows is
what to attack, what was already attacked and survived, and — separately —
the three things I got wrong while writing it, because those are where a
fresh reader is most likely to find a fourth.

## The one-line summary

The apiserver stopped interpreting REST itself and now serves from
`k8s.io/apiserver`'s own installer; the namespaced-write path stopped guessing
what a half-landed write meant; and about 1,200 lines of hand-written code
that nothing reached were deleted. The required conformance focus passes
7/7 three times over, plus baseline 11/11, and all four test lanes are green.

## The five concerns, and which commits carry each

### 1. Storage: a namespaced write no longer has an unanswerable state

`097a15e` `2ed21c6` `49a9538` `681cf1d`

A namespaced write commits to two storage systems with no transaction across
them — the parent Durable Object's log assigns the revision, the namespace
facet takes the value. Every defect this path has had came from a reader
deciding what a parent row with no facet row means. **That question has no
answer**: an apply in flight and an apply that never happened look identical
from one side.

Three designs, two withdrawn under external review:

| | Design | Why it failed |
|---|---|---|
| 1 | Envelope first; "facet lacks it ⇒ the write failed" | An apply in flight looks the same. Successful writes were dropped from watch streams and never redelivered |
| 2 | An in-memory set of revisions with an apply in flight; readers stop below the lowest | The pending check and the facet observation are taken at different instants. Review reproduced four sequences; two were the same mistake repeated in another function |
| 3 | **Outbox**: the write lands in the parent WITH its value and is trimmed only after the facet acknowledges | Live |

The rule that replaces the inference, and it holds for every read: **parent
first**. A row that still has its value is the record; a row that does not was
acknowledged before this read began, so the facet has it. Facet-first is never
safe — the acknowledgement and the trim can both land in between.

This removed machinery rather than adding it: no pending set, no ceiling, no
probe, no orphan-envelope resolution, no writes on a read path, and no
namespace fan-out (the parent's log is now the key set for every prefix).

**Attack surface for a reviewer.** The trim race on each read path
(`storeGetCurrent`, `storeList`, `storeListRaw`, `storeReplay`,
`broadcastEvent`); whether a facet failure returning *success* breaks any
caller that distinguished them; `isOffloaded` being value-presence rather than
a column, and the write `storeInsert` refuses because it would be ambiguous;
whether the fill's exact-revision fetch can still miss a row the parent chose.

**Already found by review and fixed, do not re-derive**: filling by key or
prefix instead of by revision (a concurrent update, a list at a past revision,
or a tombstone all make the facet's newest row the wrong one); `broadcastEvent`
filling before narrowing, which dragged unrelated namespaces into one write's
failure domain; and `FILL_BATCH = 400` against **workerd's SQLite cap of 100
bound parameters** — a bug no local test could have caught, because
`node:sqlite` binds as many as it is given. Found by reading the pinned
workerd binary.

### 2. The apiserver serves from the real installer

`27716c4` `8f830c1` `ff7d5ea` `4a2b368` `b1ae542`

`server.go` routed every group-version to a 789-line hand-written handler.
It now routes to `endpoints.APIGroupVersion.InstallREST` — 446 routes across
14 group-versions — over the same `genericregistry.Store` instances.

Everything k8flare-specific moved to the extension points upstream reserves:

| Rule | Now runs as |
|---|---|
| Namespace must exist on create; priority; compute class; LimitRange | `admission.Interface` |
| ClusterIP allocate / release | `BeginCreate` / `AfterDelete` |
| Seed a namespace's ServiceAccount and root CA | `AfterCreate` |
| Namespace dependent sweep | `AfterDelete` |
| Refuse a premature foreground finalize | `BeginUpdate` |
| Refuse a premature orphan finalize | `BeginUpdate` |
| Protect the management Cluster | a store decorator |

Two pieces of glue, both deliberate:

- **The request-info filter is inlined** (six lines, upstream's own body)
  rather than imported. `endpoints/filters` reaches `apiserver/pkg/util/webhook`,
  which needs the OTLP tracing wrapper this build spends 20.3MB *not* linking.
  Measured: importing it puts the chunk **1.4MB over the hard 64MiB cap**.
- **One patched line in the mirror.** Upstream hardcodes the request scope's
  hub version to the internal one; this apiserver registers external versions
  only, so every `PATCH` failed with *no kind is registered for the internal
  version*. The mirror now falls back to the served version when the scheme
  has no internal kind — what apiextensions does. Carried as a one-condition
  patch with a sha256 pin, not a whole-file overlay, so an upstream change to
  `installer.go` cannot hide behind a fork.

**Attack surface.** Whether the admission plugin's ordering matches what the
handler did (the namespace read feeds compute-class routing; LimitRange
defaulting sizes the Pod that `AssignContainersNode` then measures); the
status subresource's `PrepareForUpdate` and what it does and does not restore;
whether `APIGroupVersion.Authorizer` being nil leaves a hole (it does — see
P0-9); the deviation where field validation defaults to Strict where upstream
defaults to Warn.

**Already found and fixed**: the status strategy first restored `ObjectMeta`
wholesale, which overwrote the request's `resourceVersion` and **silently
swallowed the optimistic-concurrency 409** that kubelets and controllers
depend on.

### 3. Authentication: a node's token is no longer an administrator

`eb48d60` `b87a3ca`

A valid cluster token authenticated as `system:masters`, which short-circuits
authorization before any RBAC rule is read, and the k3s agent joins with that
same token — so a node's config file was an admin credential.

Tokens in the vault now carry a role; a token with no role is an administrator,
so every deployed vault and the development fallback are unchanged. An
agent-role token is `system:nodes` and cannot reach the `X-Remote-User` path.
Inbound `X-Remote-*` is stripped at the door, which had to happen first —
otherwise an agent token could simply name itself an administrator.

**This is a mechanism, not a rollout: nothing mints an agent-role token yet.**

**Attack surface.** Every place a token's role is consulted, and every place
one is not. Review already found `handleTokenReview` still answering
"admin / system:masters" for an agent token, which would have made the whole
split decorative — a kubelet could ask the apiserver who it was and be told it
was root. Look for the next one of those.

### 4. Deletions

`4a2b368` `6bac2b6` `a80873e` `5fc538a` `a1dc2dc`

About 2,400 lines, each unit confirmed to have no consumer first.

- the hand-written router and its subresource dispatcher, once the installer
  served everything (1,265)
- `scripts/` entirely (547): a zsh wrapper around four commands, and 14KB of
  macOS launchd shell scheduling a probe that had never once run successfully.
  The Makefile's header has said "scripts/ was removed 2026-07-08" since that
  removal; it grew back
- 441 lines no build reaches, found with `deadcode` run **once per entrypoint
  under that entrypoint's own build configuration** and unioned. The largest
  item, `pkg/leanclient/clientset/stubs.go` (272), is behind
  `!leanwidth && !schedwidth` — which **nothing builds**, and which no
  reachability tool reports because a file excluded by tags appears in no
  run's dead list
- the watch authorizer (82), dead since it was written because it only ever
  acted on a header nothing sets
- `env.ts` 129 → 96, with the bindings taken from `wrangler types`. The
  generated types are **more accurate**: `SELF`/`STORAGE` carry the
  entrypoint's type rather than `Fetcher`, so a non-existent RPC method stops
  compiling

Two hazards shaped what is *not* deleted, and a reviewer should check I
respected them: **interface satisfaction** (a first pass swept
`KineStorage`'s methods, which nothing calls by name because `storage.Interface`
calls them) and **build constraints** (see above).

Larger proposals were killed in review and are absent: deleting
`pkg/leanclient` missed three importers; replacing the garbage collector's
hand-built informer factory would have dropped per-GVR pump-window
instrumentation and changed the resync period.

### 5. The gate

`b2d410c` `5f30d6b` `3889ea8` `c63cfd5` `e5a5555`

The required garbage-collector focus was failing **41% of the time** — seven
of seventeen runs. Root cause, measured at one-second resolution on a
50-replica cascade:

```
t=7s   finalizers=["orphan"]   50 dependents, 32 still owned   (GC stripped 18)
t=8s   finalizers=GONE         28 still owned
t=10s  pods=0
```

The collector cleared the orphan finalizer having stripped 18 of 50, because
its informer held about 22. **This is the first direct measurement of what
P0-4 costs** — informers torn down at every pump-window boundary — and the
number is now in `docs/pump-window-design.md`.

The guard for it has existed since 2026-07-11. I rewired it into
`BeginUpdate`'s FinishFunc gated on `success` — a branch that cannot execute,
because clearing the last finalizer makes `GuaranteedUpdate` return
`errEmptiedFinalizers`. Now: refuse with `Conflict` while any dependent still
carries the UID, sweep from `AfterDelete`, and ask the unfiltered dependent
question (`blockingDependent` required `blockOwnerDeletion`, which is
foreground's question, not orphaning's).

**Codex reviewed that fix and found two more P1s, both the same class as the
bug being fixed — a hook placed on a path that does not run when expected.**
They are fixed in `e5a5555` and are the most useful thing in this brief:

- The sweep was still dead. The UID travelled from `BeginUpdate` to
  `AfterDelete` in a map, and `BeginUpdate`'s FinishFunc runs with `ok=false`
  *before* `deleteWithoutFinalizers` — so the entry was always removed before
  the consumer read it. **I had replaced one unexecutable branch with
  another.** The map was also unnecessary: `deleteWithoutFinalizers` hands
  `AfterDelete` the object it deleted from storage, which still carries the
  finalizer. Deleting the map fixes the P1 and a dry-run leak together, and
  the sweep ran for the first time today (measured, S69's third correction).
- **The sweep is now deleted** (`e9…`, see below). Once it ran, the argument
  for it collapsed: `AfterDelete` fires *after* the owner's storage DELETE
  succeeds, so it strips references the collector may already have acted on —
  the comment claiming "the owner is removed AFTER the sweep" was the reverse
  of the code. It also never fires at all when another finalizer outlives the
  orphan one. ~26 DO LISTs per accepted clear, unmeasured benefit, no upstream
  equivalent. **The refusal alone closes the hole, and it is not race-free
  either** — its own LIST-to-CAS window is open and now recorded in
  `known-issues.md` rather than covered by machinery that did not cover it.
- The guard could wedge an owner **forever**. It waited on any dependent
  carrying the UID, including Events — which the real collector never
  monitors (`DefaultIgnoredResources`), so that reference is never stripped
  and the `Conflict` never lifts. Deterministic, not flaky, and an owner that
  deleted fine before the guard existed. Foreground had the same hole. Both
  now skip the collector's ignored set, pinned against upstream by a test.

## Four things I got wrong this session

Listed because the pattern is more useful than any single finding, and because
a fifth one is probably in the diff. Note that #4 was found by a reviewer, in
the commit that fixed #3 — which is the argument for this document existing.

1. **"The TypeMeta stamp is causing the flake."** Plausible — it changes every
   stored object. Removing both call sites and rebuilding left the failure
   rate unchanged. Ruled out by experiment, not argument.
2. **"The orphan finalizer is not being stamped."** Read from a `curl -X
   DELETE -d` that was not sending `DeleteOptions` the way I assumed. `kubectl
   --cascade=orphan` showed the finalizer present all along. **The instrument
   was wrong**, and I nearly spent hours on a regression that did not exist.
3. **"The gate is green — 7/7 three times."** At a 41% failure rate, three
   consecutive passes happen one time in five. Recorded as S68 and wrong.
   Three passes is not a stability claim.

4. **"The sweep is a belt-and-suspenders second line."** It had never
   executed — not in the design the gate caught, and not in the one that
   replaced it. The negative control I read as "the sweep saves 30 of 50 pods"
   was measuring a build with no guard at all, so it says nothing about the
   sweep. Withdrawn; what the sweep saves is still unmeasured.

A fifth, structural: a negative control returned "pass" because I had not
rebuilt the WASM — the running Worker still had the code I thought I had
removed. Any measurement in this repo that does not rebuild the chunk is
measuring the last build.

## Two P0s still open, with their measured state

**P0-8 / P0-9 — authorization.** `APIGroupVersion.Authorizer` is nil, so the
installer's routes run no authorization; the hand-written `AuthzMiddleware`
still gates them. Watch has none at all: a cluster-token holder sees
everything, a ServiceAccount cannot open one. Measured feasibility of the
upstream pieces is in `docs/auth-upstream-feasibility.md` — the bootstrap
token authenticator links for **+22KB**, but the Node authorizer needs six
shared informers this dynamic worker cannot hold under the cost invariants,
and NodeRestriction asserts internal API types this build refuses to register.
**Size is not the constraint; informers and internal types are.**

**P0-1 — nothing observes production.** `cmd/prodprobe` exists and works;
nothing schedules it and nothing in this repository will. Stated plainly in
`docs/known-issues.md` rather than implied by machinery that has never run.

## What would change my mind about merging

- A sequence where the outbox's parent-first rule still loses or duplicates a
  watch event.
- A caller that distinguished a facet-write failure from success and now
  cannot.
- Anything in the installer switch that changes a response body a real client
  parses, that the conformance focus does not cover.
- An agent-role token reaching an administrator identity by a path
  `handleTokenReview` and `AuthMiddleware` do not cover.
- A fourth instance of the hook-on-the-wrong-path class. Three shipped on this
  branch; the audit in `docs/platform-verification.md` S70 enumerates every
  `Store` hook with the upstream path that fires it, and found two more
  (older) defects that way — a dry-run DELETE freeing a live Service's
  ClusterIP, and a failed create leaking one. Attack that table.
