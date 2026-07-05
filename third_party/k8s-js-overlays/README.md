# k8s-js-overlays

Patches exactly one upstream file so `k8s.io/kubernetes/pkg/scheduler` (and
everything that imports it) compiles for `GOOS=js`: real kube-scheduler is
otherwise unbuildable for Cloudflare Workers' Go/WASM target, because
`pkg/scheduler/scheduler.go` unconditionally imports
`pkg/scheduler/backend/cache/debugger`, whose `signal.go`
(`//go:build !windows`) references `syscall.SIGUSR2` -- a signal that does
not exist for GOOS=js. The fix is the same one-line idea
`signal_windows.go` already uses for GOOS=windows (`os.Interrupt` instead
of a real signal; `compareSignal` only gates a debug-only cache-compare
handler nothing else in this repo relies on).

## Why this isn't a `go build -overlay` (and what it is instead)

The obvious tool for a single-file patch is `go build -overlay`. It does
not work here: `k8s.io/kubernetes` resolves via this repo's root `go.mod`
`replace` to `github.com/k3s-io/kubernetes@v1.36.2-k3s1`, which lives in
`GOMODCACHE`. `go build -overlay` refuses, unconditionally, to touch any
path beneath `GOMODCACHE` (verified by actually trying it, not assumed --
see `docs/platform-verification.md`'s honest-correction entry for the
exact error and the sanity checks that isolated it to the GOMODCACHE
guard specifically). This is deliberate on Go's part: it protects the
module cache's go.sum-verified integrity from being silently overridden
at build time.

The only remaining lever is a module-level `replace k8s.io/kubernetes =>
<local directory>` -- which is exactly the "vendor/fork the whole module"
commitment the project avoided for a long time (see the honest-correction
entries in `docs/platform-verification.md` for the earlier, abandoned
attempt). What makes it tractable now is that the local directory doesn't
need to be *committed*: `scripts/gen-k8s-js-mirror.sh` generates it on
demand as a full local copy of the pinned upstream module (APFS
copy-on-write clone on macOS, a plain copy on Linux CI), swaps in the two
files below, and the result is gitignored (`.build/`). Nothing about
`k8s.io/kubernetes`'s ~5,200 files is checked into this repository -- only
the two small overlay files and this note are.

**Consequence you need to know about**: `go.mod`'s `k8s.io/kubernetes`
replace now points at `./.build/k8s-js-mirror`, a directory that must
exist on disk before *any* Go build in this repository works -- not just
the WASM scheduler build. `npm run build:wasm` regenerates it
automatically; if you're doing a host-only build (`cmd/agent`,
`cmd/scheduler`, `cmd/controller-manager`, `go vet ./...`) in a fresh
checkout before ever running a wasm build, run
`scripts/gen-k8s-js-mirror.sh` by hand first, or every Go command in this
repo fails with "cannot find module" style errors. This is now the
top-line footgun in `CLAUDE.md`'s local-dev-pitfalls list; if you're
debugging a mysterious "package not found", check `.build/k8s-js-mirror`
exists before anything else.

## Files

- `signal_notjs.go` -- upstream `signal.go`, unmodified except
  `//go:build !windows` -> `//go:build !windows && !js`.
- `signal_js.go` -- new file, `//go:build js`, `var compareSignal
  os.Signal = os.Interrupt` (mirrors `signal_windows.go`'s fallback).
- `upstream-module.txt` -- `<module> <version>` of the upstream fork to
  mirror from, kept independent of `go.mod`'s replace target so the
  generator script can still resolve the pristine upstream source after
  `go.mod` itself has been repointed at the generated mirror. Update this
  alongside a k8s version bump (`docs/k8s-version-bump.md`).
- `upstream-signal.go.sha256` -- sha256 of upstream's `signal.go` at the
  time these overlay files were last hand-verified against it. The
  generator script refuses to run if upstream's file has drifted from
  this hash, so a k8s bump that changes `signal.go`'s content forces a
  human to re-review it (rather than silently re-applying a stale patch)
  before updating this hash.

## Regenerating

```
scripts/gen-k8s-js-mirror.sh
```

Safe to re-run any time; it fully deletes and recreates
`.build/k8s-js-mirror`. If it fails on the sha256 check, see
`docs/k8s-version-bump.md`.
