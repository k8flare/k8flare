# S10: scheduler/KCM WASM size decision experiment

Throwaway measurement scaffolding for Phase 10 Stage A (see the plan this
session worked from). Not wired into any real build; delete once Stage A's
Go/No-Go decision is recorded in docs/platform-verification.md.

Three configurations, each a minimal `//go:build js && wasm` main package
that imports `pkg/controllers` and calls the Run function(s) below (a
direct call is enough to force the linker to include their full transitive
dependency graph -- nothing needs to actually execute, this only measures
`go build` output size):

1. **kcm-only**: the real `workers/controllers` as it already exists
   (`RunControllerManager` only) -- no new file needed, build that package
   directly.
2. **scheduler-only** (`scheduler-only/main.go`): `RunScheduler` only.
3. **combined** (`combined/main.go`): both `RunScheduler` and
   `RunControllerManager`. Calls them with independent informer factories
   (not the shared-factory design Stage C would use for the real
   combined path) -- irrelevant for a size measurement, since compiled
   size is a function of which *code* is linked in, not which factory
   instance a function is called with at runtime.

## Measuring

```
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath -o /tmp/out.wasm <package>
wasm-opt -Oz --strip-debug --strip-producers --enable-bulk-memory --enable-nontrapping-float-to-int --enable-sign-ext --enable-mutable-globals -o /tmp/out.opt.wasm /tmp/out.wasm
ls -la /tmp/out.opt.wasm          # raw
gzip -c /tmp/out.opt.wasm | wc -c # gzip
```
