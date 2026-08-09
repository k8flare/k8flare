# Bumping the pinned Kubernetes version

This project pins `k8s.io/kubernetes` (and the ~70 `k8s.io/*` staging
packages) in `go.mod`, replaced onto the `k3s-io/kubernetes` fork so
`cmd/agent`/`cmd/scheduler`/`cmd/controller-manager` embed the exact same
patched build k3s itself ships. `pkg/apiserver` and `cmd/k8flare-gen`
(Phase 3) were built specifically to make bumping that pin a mechanical
process instead of a manual audit of every hand-written list. This is that
process.

## Steps

1. **Update the pin.** Bump the `k8s.io/kubernetes` version (and the
   matching `k3s-io/kubernetes`/`k3s-io/k3s` replace targets, and every
   `k8s.io/*` staging replace alongside it — they must all move together)
   in `go.mod`, then `go mod tidy`.

2. **Regenerate.** `go run ./cmd/k8flare-gen`. This:
   - Re-reads `k8s.io/kubernetes`'s resolved version (`go list -m`) into
     `pkg/apiserver/zz_generated_version.go`, so `GET /version` reports the
     new version automatically.
   - Re-copies the new pin's real OpenAPI v2/v3 documents into
     `packages/k8flare-worker/assets/openapi/`.
   - Re-emits `pkg/apiserver/zz_generated_defaulters.go` from
     `pkg/apiserver/apidef.Table` crossed with
     `cmd/k8flare-gen/defaulters.go`'s hand-maintained group->package map.

3. **Build — this is where a breaking upstream change is caught.**
   `npm run build:wasm`. If a type in `pkg/apiserver/apidef/table.go`
   (New/NewList closures reference real `k8s.io/api/...` types directly)
   was renamed or removed upstream, this fails to compile right here,
   loudly, instead of silently at runtime — that's the point of the table
   being real Go code and not a YAML/JSON list. Also record the gzip size
   (`wc -c .build/wasm/apiserver.wasm`) against the 64MiB Worker Loader
   budget; a version bump can move it either way.

4. **Check for a new or removed defaulters package.** If step 3 fails on
   an unresolvable `k8s.io/kubernetes/pkg/apis/<group>/<version>` import,
   or if `go vet`/`go build` on `cmd/k8flare-gen` itself fails the same
   way, a group's versioned defaulting package appeared, moved, or
   disappeared upstream — update the `defaulterPackages` table in
   `cmd/k8flare-gen/defaulters.go` (the one hand-maintained mapping this
   process doesn't derive automatically, since Go import paths aren't
   mechanically derivable from a `schema.GroupVersion`) and re-run step 2.

5. **Verify.** `go vet ./pkg/apiserver/...` and
   `go test ./pkg/apiserver/... -count=1` (spins up a real `wrangler dev`
   stack and drives it with real `client-go` — see `CLAUDE.md`'s command
   list for prerequisites). `vp check` for the TypeScript side.

6. **Confirm nothing drifted.** `go run ./cmd/k8flare-gen && git diff --exit-code`
   — this is also a required CI job (`ci.yml`); a clean regen with no
   further diff means every derived artifact is consistent with the new
   pin.

7. **Match the conformance e2e.test binary.** Nothing to do since
   2026-08-09: `.github/workflows/e2e-conformance.yml` derives the
   `https://dl.k8s.io/vX.Y.Z/kubernetes-test-linux-amd64.tar.gz` version
   from `pkg/k8s-js-overlays/upstream-module.txt` at run time (it used to
   be a hand-bumped URL). The rationale stands: a mismatched e2e.test
   binary tests against API behavior this apiserver's vendored types
   don't match — which is why the version is derived, not floated.

8. **Run conformance CI.** This is the Definition of Done (`CLAUDE.md`'s
   inviolable rule 1) — the required baseline focus set must stay green,
   and don't shrink it to make a bump pass.

## Automation (patch releases)

Since 2026-08-09, `.github/workflows/deps-k3s-update.yml` replays steps
1–3 and 5–6 automatically (7 became a no-op the same day): weekly (or on dispatch) it asks the official
k3s update channel for the latest stable release on the currently-pinned
minor line, adopts that release's own `go.mod` pins via
`packages/wasm-build/src/sync-k3s-deps.ts` (the same "mirror k3s's
replace set verbatim" rule step 1 describes), regenerates mirrors and
`cmd/k8flare-gen` artifacts, runs ci.yml's full validation battery
in-workflow, and opens a `deps/*` PR.

What it deliberately does NOT automate:

- **The overlay sha256 review gates.** If an upstream file the overlays
  patch changed, `make gen-mirrors` fails the workflow (which opens an
  issue) and the bump falls back to this manual process — the pins are
  refreshed by a human after re-reviewing the transform, never by the
  bot.
- **Minor-line bumps** (`v1.36` → `v1.37`): dispatch the workflow with an
  explicit `k3s_tag` input if you want the mechanics replayed, but expect
  the review gates and possibly `defaulterPackages` (step 4) to need
  hands.
- **Step 8.** `e2e-conformance.yml` is dispatch-only; the PR body's
  checklist reminds the reviewer it is still the Definition of Done.

## What a bump commonly changes

- **A resource's Kind/type is renamed or removed**: caught at compile time
  in step 3 (see above) — fix `pkg/apiserver/apidef/table.go`'s entry.
- **A new stub-type informer requirement appears** (the DRA/ResourceSlice/
  ServiceCIDR pattern — a real scheduler or controller-manager informer
  that must sync against _some_ registered list, even an empty one, or it
  hangs forever in `WaitForCacheSync`): this doesn't fail a build, it hangs
  a live process. `CLAUDE.md` rule 2 applies here directly — run the real
  scheduler/controller-manager against the new pin and watch for a stuck
  `WaitForCacheSync`, don't assume the existing stub-type list is still
  complete. Add the new stub `apidef.ResourceDef` (with a `StubReason`)
  when found.
- **A group's versioned defaulting behavior changes**: `defaults_test.go`
  exercises the fields this project cares about (see e.g. the
  `ImagePullPolicy` Always-vs-IfNotPresent split for untagged images) —
  a real behavior change here should update the test's expectation to
  match upstream, not be worked around.
- **OpenAPI document filenames change** (`cmd/k8flare-gen/openapi.go`'s
  `v3UpstreamFileName`/`v3ServedPath` assume the
  `api__v1_openapi.json` / `apis__<group>__<version>_openapi.json`
  naming convention `k8s.io/kubernetes/api/openapi-spec/v3/` has used
  since it was introduced): step 3 fails loudly with a "not found" error
  naming the missing file if this ever changes.
