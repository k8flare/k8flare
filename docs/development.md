# Local development

Practical notes for working on k8flare locally: the flags that aren't
optional, the failure modes that look like flakes but aren't, and which
test lane covers what. Process and CI rules are in
[CONTRIBUTING.md](../CONTRIBUTING.md).

## Setup

Prereqs: Go 1.26+, Node 24+ with pnpm, binaryen (`wasm-opt`). Docker only if
you want Pod-on-Containers NodeVMs.

```sh
pnpm install
go mod download   # several GB cold: the k3s-flavored Kubernetes tree
make wasm         # build the WASM chunks
make dev          # wrangler dev on :8787
```

`wasm-opt` version matters. The pinned release (binaryen `version_129`, what
CI installs and what `mise` gives you via `aqua:web-assembly/binaryen`)
optimizes measurably better than the one Ubuntu's apt ships: the same
`gc-wasm` source measured 65,204,713 bytes locally but 67,222,931 bytes —
111KiB *over* the Loader cap — with apt's binaryen. Size reproducibility
depends on the version.

## `make dev`: the two flags that aren't conveniences

`make dev` runs:

```sh
npx wrangler dev -c packages/k8flare-worker/wrangler.jsonc --local \
  --enable-containers=false --persist-to .wrangler/state
```

- **`--local`** — `wrangler.jsonc` declares a `vpc_networks` binding named
  `MESH` with `"remote": true`. VPC/Mesh bindings have no local emulation,
  so a plain `wrangler dev` opens a *real* Cloudflare proxy session at
  startup: it hard-fails without credentials, and — worse — silently
  succeeds through your real account if you happen to have a cached
  `wrangler login`. Nothing in local development touches `MESH`.
- **`--enable-containers=false`** — the `containers` section is declared
  unconditionally, so dev refuses to start without a running Docker daemon.
  Drop this flag (and start Docker) only when you actually want NodeVMs; the
  node container image needs an `EXPOSE` line or dev refuses to start.

The Go test harness (`pkg/apiserver/apiserver_test.go`) passes the same two
flags, plus `--var KCM_DISABLED:1`.

## Durable Object state

Local DO state lives in `.wrangler/state` at the **repo root**
(`--persist-to`). Two consequences:

- `.dev.vars` is only read from the directory holding the wrangler config —
  `packages/k8flare-worker/.dev.vars`. A copy at the repo root is silently
  ignored.
- `go test ./pkg/apiserver/...` reuses that state without clearing it. If a
  previous run was interrupted, the next one fails deterministically with
  "already exists". That is not a flake:

  ```sh
  rm -rf .wrangler/state
  ```

  Do that first whenever a local run wedges or fails on startup, then
  re-run. Only conclude "flaky" afterwards.

One more emulator quirk: `wrangler dev`'s alarm emulation sometimes doesn't
fire under read-only polling. Before concluding an alarm is broken, issue one
write.

## Test lanes

| Lane | Runs | Covers |
|---|---|---|
| `make test` | `go test ./pkg/apiserver/...`, own `wrangler dev` with `KCM_DISABLED=1` | apiserver, storage, admission, RBAC, tokens — **no controllers** |
| `make test-kcm` | `TestKCMDynamicWorkerControlPlane`, `-timeout 15m` | real KCM/GC/sched dynamic workers |
| `make test-clusterop` | `TestClusterOperatorLifecycle`, `-timeout 15m` | cluster provisioning/teardown, with the workload controllers and scheduler off (`CM_DISABLED`/`SCHED_DISABLED`) |

The `KCM_DISABLED=1` kill switch (honoured in
`packages/k8flare-worker/src/storage/index.ts` and
`src/controllers/index.ts`) is what lets `make test`'s Pods sit untouched by
controllers — the tests are written assuming nothing reconciles them. This
is why "the Pod moves under `make dev` but nothing happens under `make
test`" is expected behaviour, not a bug.

Both `test-*` lanes pay a large one-time cost on the first poke: workerd
compiles the ~40MB WASM modules in-process. They're the local stand-in for
the conformance workflow's dynamic-worker variants on machines that can't
run a Linux kubelet.

## WASM chunks and the 64MiB cap

Five chunks are built into `packages/k8flare-worker/assets/wasm/`:
`apiserver`, `kcm`, `gc`, `sched`, `clusterop` — plus `selector.wasm`, which
is bundled directly into the Worker script rather than loaded through the
Loader.

Each Loader chunk is gated at build time against the Worker Loader's hard
64MiB (67,108,864 bytes) raw-module cap. `make wasm` prints the remaining
headroom per chunk and **fails the build** if one exceeds the cap. Some
chunks sit close to it, so adding a dependency can break the build outright;
check the printed headroom before and after.

The size levers, if you hit the cap: `wasm-opt -Oz` (mandatory — kcm doesn't
fit unoptimized), and `kubernetes.Interface` *width*, selected by build tag
(`-tags leanwidth` for apiserver/kcm/gc/clusterop, `-tags schedwidth` for
sched). Width is the big one: full-width kcm measured 98.6MB against 66.1MB
narrow. Background in
[platform-verification.md](platform-verification.md) (S14, S21).

Make skips a chunk whose inputs are unchanged. To force a rebuild:

```sh
make clean-wasm wasm    # or: make -B wasm, which is what CI runs
```

CI never relies on Make's mtime check — a fresh `git checkout` doesn't
preserve timestamps — so it always forces the rebuild.

## Module mirrors (`.build/`)

`go.mod`'s `k8s.io/kubernetes` and `k8s.io/client-go` replace directives
point into generated mirrors under `.build/` (`k8s-js-mirror`,
`clientgo-lean-mirror`, and friends). They are gitignored but they are
**build inputs**: every Go command in this repo, including a plain `go mod
download`, fails on a fresh checkout until they exist. `make gen-mirrors`
creates them, and it's an order-only prerequisite of every Go target, so
normal `make` usage handles it.

Caveat worth knowing before you regenerate by hand: on 2026-07-05 an
`rm -rf`-and-regenerate cycle lost the on-disk mirror state that was
producing a working 62.5MB KCM binary, and the committed tree only
reproduced a 98.6MB one — over the cap. Back the directories up first:

```sh
cp -Rc .build/k8s-js-mirror .build/k8s-js-mirror.bak-$(date +%s)
```

(The generators do claim their own idempotency, and the Makefile runs
`gen-mirrors` unconditionally, which sits in tension with that caveat. Until
that's reconciled, the backup costs nothing.)

## Code generation

```sh
make gen    # == go run ./cmd/k8flare-gen
```

Regenerates `pkg/apiserver/zz_generated_*.go`, the lean client-go stubs
under `pkg/leanclient/gen/`, `packages/k8flare-worker/src/k8s/gen`, and the
OpenAPI/discovery documents in `packages/k8flare-worker/assets/`. CI re-runs
the generator and fails on any diff, so commit the output. Never hand-edit a
file carrying the `Code generated by k8flare-gen. DO NOT EDIT.` header;
change `cmd/k8flare-gen` instead.

(Note: `go generate ./...` does nothing here — there are no `//go:generate`
directives. Use `make gen`.)

## Talking to the local server

`wrangler dev` serves plain HTTP, and client-go's `clientcmd` refuses to
send credentials over non-TLS — so real `kubectl` against a kubeconfig needs
a local TLS terminator (self-signed cert + reverse proxy). Go tests that
build a `rest.Config{BearerToken: ...}` directly are not subject to that,
which is why the test suites drive the server that way. `curl` works fine:

```sh
curl -H 'Authorization: Bearer k8flare-dev-token' localhost:8787/api/v1/namespaces
```

`k8flare-dev-token` is the fallback the default cluster accepts when no
token has been minted and `K3S_TOKEN` is unset — see
[SECURITY.md](../SECURITY.md) before exposing anything.

`kubectl apply --validate=false` is not needed: OpenAPI v2/v3 are served
from Static Assets, and server-side field validation is implemented and
defaults to Strict (`pkg/apiserver/fieldvalidation.go`).
