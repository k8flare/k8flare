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
| The required CI gate does not exercise the WASM controllers | The `host` conformance variant runs upstream's native kube-scheduler and kube-controller-manager, with the WASM ones disabled. The resident controllers the README describes are covered only by advisory variants. | Advisory variants have been green for 6 consecutive runs; promotion to required is waiting on more samples (`TODO.md` P1-1, P1-2) |
| The conformance focus set is small | Roughly a dozen upstream tests (a baseline group plus 7 garbage-collector tests), not the full suite. Passing it does not mean "conformant Kubernetes". | By design, grown deliberately — `docs/general-purpose-k8s-plan.md` |
| No backup of cluster identity | `cmd/k8flare-backup` round-trips every object the API serves, verified in production. It **cannot** cover the CA keypairs or the per-cluster token vault, which live in Durable Object facets. Restoring into a fresh deployment returns your workloads but not your cluster's identity: nodes holding certificates signed by the old CA will not rejoin. | Partial (`TODO.md` P1-4) |
| Idle cost is not continuously verified | The daily probe asserts that the control plane converges. Asserting that it then *parks* needs a Cloudflare Analytics token a maintainer must set; without it that half is skipped. Parking has been measured by hand four times, never continuously. | Needs a maintainer credential (`cmd/prodprobe/README.md`) |
| Unconverged workloads keep the alarm chain alive | A cluster with work that can never converge (for example a Deployment with no nodes) wakes roughly every 40 seconds — about 65,000 alarms a month. The cause is unidentified. This contradicts cost invariant #3. | Open, measured (`docs/platform-verification.md` S26b) |
| Foreground deletion needs four hand-written guards | `pkg/apiserver/gracefuldelete.go` compensates for controllers that only run inside pump windows. Each guard's own comment notes upstream needs no such thing. They work; they are accidental complexity, and they full-list every namespaced store on some paths. | Open, sequenced behind the pump-window design (`TODO.md` P2-1) |
| The k3s tunnel endpoint is unauthenticated | `/v1-k3s/connect` had to be exempted from the door, because the agent dials with no Authorization header and relies on an mTLS certificate Cloudflare strips. The endpoint is a stub that accepts a socket and does nothing, so what it admits is an idle socket, not a capability — but it is reachable by anyone, and the socket is held by the shell Worker rather than a hibernating Durable Object. | Open, deliberate (`docs/platform-verification.md` S38) |
| No dynamic storage provisioner | PersistentVolumeClaims stay `Pending`, exactly as on a real cluster with no provisioner configured. PV, PVC and StorageClass exist as CRUD resources. | By design for now (`CLAUDE.md`) |
| No release tag | There is no tagged version. Pin a commit. | Open |

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
