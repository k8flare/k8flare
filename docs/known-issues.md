# Known issues

What is broken or unproven **today**, for someone deciding whether to run this.
Two minutes, not an archaeology dig. Last reviewed 2026-09-11 against `main`.

`TODO.md` is the working list with acceptance criteria;
`docs/platform-verification.md` has the measurements behind every claim here.
This page is the short version, and it is the one that must never be stale.

## Read this first

k8flare is an **alpha**. The apiserver and its Durable Object storage are
exercised continuously; the controllers running as resident WASM are newer and
have produced most of the defects found so far. A defect class keeps recurring
and is worth understanding before you weigh anything below: **work that
outlives the request which started it gets abandoned silently**, and
`wrangler dev` does not reproduce it, so local gates stay green while
production is broken. Three separate instances were found and fixed in
September 2026 (S31, S34, S39). Assume there are more.

## Broken or unproven

| Issue | Impact | Status |
|---|---|---|
| Conformance can now be run locally, including the required variant | Not an issue — noted because the table above used to imply the Definition of Done was maintainer-only. `docs/development.md` has the recipe, and it reproduces the **required `host` variant**, not just the advisory ones: with a node that can actually run pods (S53/S54) the **baseline focus passes 11/11** in about six minutes. The garbage-collector focus passes 7/7 only on a node that *cannot* start pods; with a working node it is **5/7**, and those two failures are unexplained. It is still not CI. Check `--ginkgo.dry-run` says "Will run 7 of" before trusting a result — copying the focus out of the workflow drops a spec if its shell escaping is not undone. | Added 2026-09-11 (S42, its correction, and S48) |
| The gate now exercises the WASM controllers | Resolved 2026-09-12 (`docs/platform-verification.md` S65). The local gate runs all three control-plane variants — `host`, `kcm-dw`, `sched-dw` — and each passes the required garbage-collector focus 7/7 three times and the baseline focus 11/11. The earlier report that `sched-dw` failed came from stray processes of the maintainer's own (S62). Caveat: three runs each is not the thirty the pump-window design asks for; what is shown is that the dynamic workers are not worse than host processes. | Resolved |
| The conformance focus set is small | Roughly a dozen upstream tests (a baseline group plus 7 garbage-collector tests), not the full suite. Passing it does not mean "conformant Kubernetes". | By design, grown deliberately — `docs/general-purpose-k8s-plan.md` |
| No backup of cluster identity | `cmd/k8flare-backup` round-trips every object the API serves, verified in production. It **cannot** cover the CA keypairs or the per-cluster token vault, which live in Durable Object facets. Restoring into a fresh deployment returns your workloads but not your cluster's identity: nodes holding certificates signed by the old CA will not rejoin. | Partial (`TODO.md` P1-4) |
| Idle cost is only partly verified continuously | The daily probe asserts that the control plane converges, and that nothing **writes** across a quiet window afterwards (a proxy needing only the cluster token). Asserting that nothing **runs** — request and alarm counts — still needs a Cloudflare Analytics token a maintainer must set. An alarm chain that wakes and does no work would pass today's check. | Partial (`cmd/prodprobe/README.md`) |
| Unconverged workloads kept the alarm chain alive | A cluster with work that can never converge woke roughly every 40 seconds — about 65,000 alarms a month, against a designed ceiling of 600 seconds. Cause found 2026-09-11: every poke reset the backoff counter, and on such a cluster the controllers never stop writing, so the counter never survived. Fixed. | Fixed locally, **not yet re-measured in production** (`docs/platform-verification.md` S40) |
| Foreground deletion needs four hand-written guards | `pkg/apiserver/gracefuldelete.go` compensates for controllers that only run inside pump windows. Each guard's own comment notes upstream needs no such thing. They work; they are accidental complexity. Measured in production 2026-09-11: deleting a 10-pod ReplicationController with `--cascade=foreground` costs **440 Durable Object LIST calls across 27 resource kinds**, most of them unrelated to what was deleted (S41). Removing them is worse: with one guard disabled, the cluster never stopped retrying — **451 requests/minute an hour after the last test, all 404s against a namespace that no longer existed** (S43). | Open, and measured; removal blocked behind the pump-window design (`TODO.md` P2-1) |
| The k3s tunnel endpoint is unauthenticated | `/v1-k3s/connect` had to be exempted from the door, because the agent dials with no Authorization header and relies on an mTLS certificate Cloudflare strips. The endpoint is a stub that accepts a socket and does nothing, so what it admits is an idle socket, not a capability — but it is reachable by anyone, and the socket is held by the shell Worker rather than a hibernating Durable Object. | Open, deliberate (`docs/platform-verification.md` S38) |
| No dynamic storage provisioner | PersistentVolumeClaims stay `Pending`, exactly as on a real cluster with no provisioner configured. PV, PVC and StorageClass exist as CRUD resources. | By design for now (`CLAUDE.md`) |
| Resident controller logs are invisible in production unless you ask for them | A dynamic worker's console output does not reach the `wrangler tail` of the script that loaded it, so kcm, gc, sched and clusterop look identical whether they are working or doing nothing — the most likely reason the six-week outage above went unnoticed. Deploying with `PUMP_TRACE=1` relays their trace lines through the platform's `tails` field; verified in production 2026-09-12, and verified to cost nothing when off. Ordinary deployments still run blind. | Partially addressed (`docs/platform-verification.md` S45, S46) |
| The first workload after a **WASM-changing** deploy waits 20+ minutes | Measured 2026-09-12: a Deployment created on a control plane that had not run since a deploy that changed the controller WASM took **more than 20 minutes** to be reconciled; the next one took 11 seconds. The wait is the one-time compile of ~44MB. A deploy that leaves the WASM byte-identical (TypeScript or config only) costs nothing — the same test converged in **23 seconds** — because the compile is cached by content across Worker versions. So this bites on version bumps, not on routine deploys. | Open, measured (`docs/platform-verification.md` S50) |
| `/readyz` does not mean the controllers can work | It checks that each component's WASM manifest asset exists, not that the dynamic worker is loaded. After a deploy the control plane answers `readyz` 200 for several minutes while it still compiles ~44MB of controller WASM and reconciles nothing. Making readyz force a load is not an option: it is anonymous, so that would let anyone trigger the compile. | Open, documented (`docs/platform-verification.md` S47) |
| No release tag | There is no tagged version. Pin a commit. | Open |
| The repository clones at 45 MiB | Two compiled binaries were committed by accident and remain in history. They are untracked and ignored now. History was deliberately **not** rewritten: it would save 33 MiB but invalidate the 24 commit SHAs the verification log cites. | Decided, not a defect (`TODO.md` P1-9) |
| CI is not used | GitHub Actions is deliberately not used (maintainer decision, 2026-09-12, on cost). The conformance gate runs locally instead, in the required `host` variant, on a node that can run pods — `docs/development.md` has the recipe and `docs/platform-verification.md` S63 the current result (required garbage-collector focus, 3 runs, 7/7 each). | By design |

## Fixed recently, worth knowing about

These were live in production and are listed so you can tell whether a
deployment you are looking at predates the fix.

| Was broken | Until | What it looked like |
|---|---|---|
| The deployed control plane could not reach its own dynamic workers | 2026-09-09 | Controllers silently did nothing for six weeks; `kubectl scale` ignored for minutes; the idle alarm never parked. Every local gate was green throughout (S30) |
| Resident controllers' outbound I/O was pinned to the request that created them | 2026-09-09 | Informers stopped receiving updates after the first pump window; only a fresh load recovered (S31) |
| A `dryRun` request wrote for real | 2026-09-11 | `kubectl --dry-run=server` created objects, allocated ClusterIPs and ran post-create effects (P0-5) |
| UID preconditions on DELETE were discarded | 2026-09-11 | A UID-guarded delete could remove a recreated object of the same name (P0-3) |
| Pod-on-Containers never started a node | 2026-09-11 | Pods annotated for the Containers backend stayed `Pending` indefinitely (S39) |
| The k3s agent retried a 401 every 3 seconds | 2026-09-11 | About 28,800 billed requests per day per attached node (S38) |

## Security posture

`SECURITY.md` is the authority. The two things people most often miss: a
deployment with no `K3S_TOKEN` secret accepts a well-known development token,
and the cluster token is equivalent to `system:masters`.
