# Contributing to k8flare

k8flare is pre-production and maintained by a single maintainer. Bug
reports, reproductions, and focused patches are all welcome — but please
open an issue before starting anything larger than a small fix, so you
don't spend time on a direction that conflicts with the project's cost
model (below).

## The two rules that override everything else

1. **Idle cost must approach storage cost alone.** Nothing may run on an
   idle cluster: no resident process, no polling alarm, no non-hibernating
   WebSocket. Alarms are event-armed and self-disarm.
2. **Reuse upstream Kubernetes, don't re-implement it.** New control-plane
   behaviour should come from a real `k8s.io/...` package compiled into
   one of the WASM entrypoints. "Could we embed the real thing?" is the
   first question on any feature.

The full rule set lives in [CLAUDE.md](CLAUDE.md) (it is an AI-agent brief,
mostly in Japanese); the cost rules are expanded in
[docs/cost-model.md](docs/cost-model.md).

## Branches and commits

- Never commit to `main`. Work on a topic branch: `feat/*`, `fix/*`,
  `docs/*`, or `chore/*`. (CLAUDE.md lists only the first three; `chore/*`
  is in active use and this list is the current one.)
- Commit messages, code, and comments are in **English**.
- **No tool-attribution trailers.** `Co-Authored-By: Claude ...` and
  equivalents are not accepted; strip them before pushing.
- Keep diffs surgical — don't reformat or "improve" code your change
  doesn't touch.

## Local gates

Run these before opening a PR. Prereqs: Go 1.26+, Node 24+ with pnpm,
binaryen (`wasm-opt`).

| Command | What it does | Cost |
|---|---|---|
| `make check` | `vp check` — formatting + lint + TypeScript | seconds |
| `make vet` | `go vet` in three passes: host, `GOOS=js -tags leanwidth`, `GOOS=js -tags schedwidth` | seconds (after mirrors exist) |
| `make test` | All four lanes below, in order | ~8 min once the chunks exist — the WASM build is the expensive part |
| `make test-unit` | `test-cfruntime` (the `GOOS=js` pump/boundary tests, as a real wasm test binary under node) + `test-ts` (vitest over the Worker's DO policy and watch lifecycle) | seconds; no `wrangler dev`, no WASM chunks |
| `make test-apiserver` | apiserver integration suite; boots its own `wrangler dev` with `KCM_DISABLED=1` and drives it with real client-go | ~70s (S64 measured 68.9s) |
| `make test-kcm` | Same harness with the real KCM/GC/sched dynamic workers enabled | ~5 min (S64 measured 296s and 361s on two runs); the generous `-timeout 15m` is headroom for the first poke compiling three ~40MB WASM modules inside workerd |
| `make test-clusterop` | Cluster-operator lifecycle against the real clusterop dynamic worker | ~75s (S64), same `-timeout 15m` headroom as `test-kcm` |

`make check` and `make vet` are cheap; run them always. `make test` is the
correctness gate, and you run it yourself — nothing runs it for you (see
"CI" below). Use the individual lane names while iterating.

The three `wrangler dev` lanes stay separate processes because their
configurations are incompatible: `test-apiserver` runs with
`KCM_DISABLED=1`, since those tests assume nothing reconciles their Pods,
and the other two need the controllers on.

The first `make wasm` is the expensive step (the KCM `wasm-opt -Oz` pass
alone is ~2 min, and a cold machine downloads several GB of Go modules);
after that, Make skips any chunk whose sources didn't change.

More local-dev detail — why `make dev` needs `--local`, what to do when a
run wedges, which mirrors are build inputs — is in
[docs/development.md](docs/development.md).

## CI: there isn't any (since 2026-09-12)

**GitHub Actions is switched off.** On 2026-09-12 the maintainer decided
not to pay for it (`docs/platform-verification.md` S63, which also records
that nothing in this repository is waiting for billing to be restored).
Every run now stops in about four seconds at GitHub's billing gate — *"The
job was not started because recent account payments have failed or your
spending limit needs to be increased"* — so `ci.yml`, `e2e-conformance.yml`,
`cost-gate.yml`, `smoke-nodes.yml`, `prod-probe.yml` and `release.yml` are
all inert, whatever their triggers say. Do not read a red check on a pull
request as a real failure, and do not expect a green one.

The workflow files are kept rather than deleted: `e2e-conformance.yml` in
particular is where the conformance focus regexes live, and the local gate
copies them out of it.

**The gate is local.** Upstream conformance is still this project's
definition of done — what ended is CI enforcing it. A maintainer runs it
before merge with `scripts/e2e-harness.sh`:

- the three control-plane variants (`host`, `kcmdw`, `scheddw`). All three
  are required: `docs/platform-verification.md` S65 measured them
  identical and removed the old advisory/required split.
- the `GC_FOCUS` (7 specs) and `BASELINE_FOCUS` (11 specs) regexes from
  `e2e-conformance.yml`.

The full recipe, including the traps, is in
[docs/development.md](docs/development.md#running-upstream-conformance-locally).
It needs Docker and the upstream `e2e.test` binary, so not every
contributor can run it. If you can't: run `make check`, `make vet` and
`make test`, then say in the pull request which lanes you ran and whether
your change could affect Kubernetes semantics — that is what tells the
maintainer to run the conformance gate before merging, and expect the merge
to wait on it. (`docs/platform-verification.md` S64 is the first worked
example of that flow.)

CLAUDE.md's inviolable rule #1 still reads "conformance CI is the
definition of done". That wording predates 2026-09-12; the rule's subject —
the upstream conformance suite — has not changed, only where it runs.

## Generated code

Files under a `gen/` directory, and any file whose header says
`Code generated by k8flare-gen. DO NOT EDIT.`, are never hand-edited. Change
the generator in `cmd/k8flare-gen/`, then regenerate:

```sh
make gen        # == go run ./cmd/k8flare-gen (plus the .build/ mirrors)
```

Commit the regenerated output. `ci.yml` still carries the drift check that
re-runs the generator and fails on any diff in
`pkg/apiserver/zz_generated_*.go`,
`packages/k8flare-worker/src/k8s/gen`, and the
`packages/k8flare-worker/assets/{openapi,api,apis}` documents — but it does
not execute (see "CI" above), so run `make gen` and confirm `git status` is
clean yourself before pushing. Build output under
`packages/k8flare-worker/assets/wasm/` is likewise produced by `make wasm`,
not edited.

## Cost-sensitive changes

Adding or changing any of `setAlarm`, `setInterval`, `sleepAfter`,
`onActivityExpired`, a cron trigger, a `scheduled()` handler, or
`waitUntil` needs a note in [docs/cost-model.md](docs/cost-model.md)
**before** the implementation: the idle monthly cost and the active unit
cost (requests, duration GB-s, rows read-written, alarm invocations, Worker
Loader $/unique/day, Containers vCPU/GiB-seconds). Estimates are marked as
estimates and replaced by measurements later; superseded estimates are kept,
not deleted.

The same applies to anything that would run on Cloudflare Containers, which
bill by wall clock: hot paths (apiserver, gateway, watch) must never move
there.

## Where things live

- `pkg/` — Go control-plane logic, including the WASM entrypoints under
  `pkg/*/cmd/*-wasm/`.
- `cmd/` — standalone binaries that run outside Workers (`agent`,
  `scheduler`, `controller-manager`, `k8flare-gen`).
- `packages/k8flare-worker/` — the only deployed Worker and the only
  TypeScript workspace. Keep it to routing, auth, Durable Object glue, and
  bindings; business logic belongs in Go.

## Security issues

Don't open a public issue. See [SECURITY.md](SECURITY.md).
