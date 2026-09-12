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
make wasm         # regenerates the .build/ mirrors, then builds the chunks
make dev          # wrangler dev on :8787
```

Run `make` first, not `go`. Every Go command here resolves `k8s.io/*`
through `replace` directives that point into `.build/`, which does not
exist in a fresh checkout — so a bare `go mod download` or `go build`
fails until a `make` target has run `gen-mirrors` (see "The .build/
mirrors" below). `make wasm` does that for you; the cold run also pulls
the k3s-flavored Kubernetes tree, a few GB.

`wasm-opt` version matters. The pinned release (binaryen `version_129`, what
CI installs and what `mise` gives you via `aqua:web-assembly/binaryen`)
optimizes measurably better than the one Ubuntu's apt ships: the same
source measured 65,204,713 bytes locally but 67,222,931 bytes — 111KiB
*over* the Loader cap — with apt's binaryen (found 2026-07-10, when CI's
Build WASM step failed on a build that passed locally). Size
reproducibility depends on the version.

The chunk that gets hurt is whichever is closest to the cap, which today
is **apiserver** (42.8MB, ~21MB of headroom) — not `gc`, which the
original note named and which now sits 25MB clear. `make wasm` prints
every chunk's headroom; trust that over any number written down here.

## `make dev`: the two flags that aren't conveniences

`make dev` runs:

```sh
npx wrangler dev -c packages/k8flare-worker/wrangler.jsonc --local \
  --enable-containers=false --persist-to .wrangler/state
```

- **`--local`** — `wrangler.jsonc` declares a `vpc_networks` binding named
  `MESH` with `"remote": true`. VPC/Mesh bindings have no local emulation,
  so a plain `wrangler dev` tries to open a *real* Cloudflare proxy session
  at startup. What that does depends on what credentials it finds
  (measured 2026-07-30):
  - cached `wrangler login` resolving to **several accounts**, no
    `account_id` in the config → hard failure: *"More than one account
    available but unable to select one in non-interactive mode."*
  - cached login resolving to **one account** → it succeeds, and your
    "local" development quietly runs through your real Cloudflare account.
  - **no credentials at all** (CI) → it does not attempt the session and
    dev starts normally. This is why `.github/workflows/cost-gate.yml`
    works without `--local`; do not conclude from that that you can drop
    the flag locally.

  Nothing in local development touches `MESH`, so `--local` removes the
  question entirely.
- **`--enable-containers=false`** — the `containers` section is declared
  unconditionally, so dev refuses to start without a running Docker daemon.
  Drop this flag (and start Docker) only when you actually want NodeVMs; the
  node container image needs an `EXPOSE` line or dev refuses to start.

The Go test harness (`pkg/apiserver/apiserver_test.go`) passes the same two
flags, plus `--var KCM_DISABLED:1`.

## `new_module_registry`

`wrangler.jsonc` sets `"compatibility_flags": ["new_module_registry"]`.
Every harness that starts its own `wrangler dev` points at that one config,
so nothing has to pass it on the command line — but if you run workerd or
wrangler some other way, carry the flag over. Without it, modules compile
eagerly at isolate startup, which puts the 4.58MB `selector.wasm`
(`src/k8s/selector-wasm.ts`, loaded through a dynamic `import()`) back on
every request's startup path. Adopters on an older wrangler whose bundled
workerd does not know the flag will see the runtime refuse to start; see
`docs/platform-verification.md` S35 for what the flag does and does not
buy here.

## Durable Object state

Local DO state lives in `.wrangler/state` at the **repo root**
(`--persist-to`). Two consequences:

- `.dev.vars` is only read from the directory holding the wrangler config —
  `packages/k8flare-worker/.dev.vars`. A copy at the repo root is silently
  ignored. Start from `packages/k8flare-worker/.dev.vars.example`.
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

`make test` runs all three lanes below, in order, in about three minutes.
They cannot share one `wrangler dev`: the apiserver lane needs the
controllers off and the other two need them on. Run a single lane by name
while iterating.

| Lane | Runs | Covers |
|---|---|---|
| `make test-apiserver` | `go test ./pkg/apiserver/...`, own `wrangler dev` with `KCM_DISABLED=1` | apiserver, storage, admission, RBAC, tokens — **no controllers** |
| `make test-kcm` | `TestKCMDynamicWorkerControlPlane`, `-timeout 15m` | real KCM/GC/sched dynamic workers |
| `make test-clusterop` | `TestClusterOperatorLifecycle`, `-timeout 15m` | cluster provisioning/teardown, with the workload controllers and scheduler off (`CM_DISABLED`/`SCHED_DISABLED`) |

`ci.yml` runs the same three on every pull request. Until 2026-07-31 it
ran only the first, so the real controllers had no automatic gate at all —
the job that does exercise them, `cost-gate.yml`, is dispatch-only and a
fork contributor cannot trigger it.

**Known flake on these three CI steps, and why wrangler stays on
4.106.0**: intermittent `Error: Network connection lost.` failures
(never reproduced locally) are
[cloudflare/workers-sdk#14641](https://github.com/cloudflare/workers-sdk/issues/14641) —
`wrangler dev`'s ProxyWorker↔UserWorker connection pool doesn't override
workerd's 5s idle keep-alive timeout, so a request landing on that
boundary intermittently dies (confirmed upstream on wrangler
4.99.0–4.114.0, all miniflare 4.x). In a 2026-08-09 A/B on CI, the KCM
lane crashed on 4/5 runs under wrangler 4.120.0 (miniflare
`5.20260801.1-alpha`) and 0/1 under 4.106.0 (miniflare `4.20260630.0`,
one sample). That is why wrangler is held at 4.106.0 — the crash has
not been observed on the current pin, though one clean run doesn't
prove it can't happen there too. The hold lives in `pnpm-lock.yaml` —
`package.json`'s `^4.106.0` range admits newer 4.x — so don't accept
any wrangler bump without checking which miniflare major it pulls and
A/B-ing the test lanes on actual CI runners. `ci.yml` and
`deps-k3s-update.yml` retry each of the three steps once as a safety
net; a same-lane failure on both attempts is a real failure, not this
flake. Revisit the pin and the retries once #14641 is fixed and
miniflare 5 is stable.

The KCM and clusterop lanes' client requests are individually bounded
(`rest.Config{Timeout: 30 * time.Second}` in `kcmdw_test.go` and
`clusterop_test.go`): a stuck request fails fast and the `waitFor`
poll's own budget governs, instead of one hung connection riding out
the full 15-minute `go test -timeout`. A lane that still dies at that
outer timeout is not this flake — treat it as a real bug.

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

## The harness lifecycle script

`scripts/e2e-harness.sh` owns starting and stopping the local conformance
harness. Use it rather than assembling the pieces by hand:

```sh
export HARNESS_ASSETS=/path/to/dir   # holds e2e/kubeconfig.yaml, e2e/tls/, e2e/kubernetes/
export HARNESS_SCHED=/path/to/k8flare-scheduler          # go build ./cmd/scheduler
export HARNESS_CM=/path/to/k8flare-controller-manager    # go build ./cmd/controller-manager

scripts/e2e-harness.sh start host      # or kcmdw | scheddw | alldw
scripts/e2e-harness.sh status
scripts/e2e-harness.sh stop
```

`start` calls `stop` first and then **refuses to continue unless exactly the
right number of host processes is running** (host 2, kcm-dw 1, sched-dw 1,
all-dw 0). `stop` kills by PID and by name and **exits non-zero if anything
survives**. Both guards exist because sixteen stray controller-managers once
accumulated across a session's experiments and made a `replicas=2` workload
look like it churned sixty pods (`docs/platform-verification.md` S62).

## Experiments that start processes

Kill what you started, by PID, and check. A local harness experiment that
starts a host `kube-controller-manager` or `kube-scheduler` against
`127.0.0.1:8443` keeps writing to that cluster until it is killed — and
`pkill -f <name>` silently matches nothing if the binary was built under a
different name. Sixteen stray controller-managers accumulated across one
session's experiments and made a `replicas=2` workload look like it churned
sixty pods; the real controller had created two
(`docs/platform-verification.md` S62). Separate ports and data directories do
not help when every process points at the same apiserver.

```sh
pgrep -fl 'controller-manager|scheduler|wrangler' | grep -v Chrome
```

## Experiments that modify source

Run them in a `git worktree`, never in your main checkout. An experiment that
disables a guard to see what breaks leaves a change that looks like nothing in
`git status` once you have moved on, and `git add -A` on an unrelated commit
will carry it to `main` (`docs/platform-verification.md` S43). A worktree also
lets the experiment keep its own `.wrangler/state`, so it cannot collide with
a test lane.

## Running upstream conformance locally

The Definition of Done is `e2e-conformance.yml`, which only a maintainer can
dispatch — and which, as of 2026-09-12, cannot run at all: every job on the
repository fails in four seconds because the organisation's GitHub billing
needs attention. The same upstream binary runs on a laptop. Verified on macOS
(arm64); the garbage-collector focus the required gate uses passes **7/7 in
90–170 seconds**, in the required `host` variant
(`docs/platform-verification.md` S48).

(An earlier revision of this line said "6/6". That was the focus-extraction
bug in S42 — the shell escaping described below dropped one spec, and the one
it dropped was the spec CI had actually been failing.)

```sh
# 1. The upstream e2e binary, at the version go.mod pins.
K8S=$(awk '{print $2}' pkg/k8s-js-overlays/upstream-module.txt | sed -E 's/-k3s[0-9]+$//')
curl -sfL "https://dl.k8s.io/$K8S/kubernetes-test-darwin-arm64.tar.gz" | tar xz   # or linux-amd64

# 2. A TLS front for wrangler dev. client-go refuses to send credentials over
#    plain HTTP, so the dev instance needs a terminator; any will do.
openssl req -x509 -nodes -newkey rsa:2048 -days 1 -keyout dev.key -out dev.crt \
  -subj "/CN=127.0.0.1" \
  -addext "subjectAltName=IP:127.0.0.1,DNS:localhost,DNS:host.docker.internal"

# 3. wrangler dev behind it, then a node.
make wasm
npx wrangler dev -c packages/k8flare-worker/wrangler.jsonc --local \
  --enable-containers=false --port 8788 --persist-to /tmp/e2e-state
docker build -t k8flare-node:local packages/k8flare-worker/images/node
docker run -d --name e2e-node --privileged --cgroupns=private \
  --tmpfs /run --tmpfs /var/run -e K3S_TOKEN=k8flare-dev-token \
  k8flare-node:local   # point -server at https://host.docker.internal:<tls port>

# 4. The same focus the required gate uses, lifted from the workflow.
./kubernetes/test/bin/e2e.test --kubeconfig=kubeconfig.yaml \
  --provider=skeleton --num-nodes=1 --disable-log-dump --ginkgo.no-color \
  --ginkgo.focus="$(the GC_FOCUS or BASELINE_FOCUS value in e2e-conformance.yml)"
```

**What this harness can and cannot stand in for.** As written above it runs the
resident WASM controllers, which reproduces the `sched-dw` and `kcm-dw`
variants — **advisory** in CI, not the `host` variant that is the required
gate. To reproduce the required variant instead, build `./cmd/scheduler` and
`./cmd/controller-manager`, start `wrangler dev` with
`--var SCHED_DISABLED:1 --var CM_DISABLED:1 --local` (and
`--local-protocol https --https-key-path/--https-cert-path`, which removes the
need for the TLS proxy above), then point both binaries at it with
`--server=https://127.0.0.1:8443 --token=... --insecure-skip-tls-verify`. That
passes the required garbage-collector focus 7/7 in about 90 seconds
(`docs/platform-verification.md` S48). Three of the eleven baseline specs still
fail there, but for a reason outside the control plane: on Apple Silicon the
node image's k3s assets are x86-64 and run under emulation, where
`prctl(PR_SET_SECCOMP, …)` returns EINVAL, so containerd decides seccomp is
unsupported and refuses to create any pod sandbox (`docs/platform-verification.md`
S53). Rebuild the node image with arm64 k3s assets to lift that; the required
garbage-collector focus does not need pods to run and passes as is. For the garbage-collector focus the distinction does not
matter: the gc dynamic worker runs in every variant, because the host has no
garbage collector. For anything sig-scheduling it matters a lot — the
`SchedulerPredicates` specs in `BASELINE_FOCUS` are scheduling-sensitive and a
single small container node is not the runner CI uses. Treat a local baseline
failure as "unattributed" until you have run the same focus against `main`
with the same harness.

**Copying the focus out of the workflow has a trap.** `GC_FOCUS` is a
single-quoted shell string, so the apostrophe in one spec name is written
`'\''` — the shell's escape, not part of the pattern. Paste it verbatim into a
regex and that spec silently stops matching, which cost a wrong result once
already (`docs/platform-verification.md` S42 訂正). Always confirm the count
first:

```sh
./kubernetes/test/bin/e2e.test ... --ginkgo.dry-run --ginkgo.focus="$FOCUS" | grep 'Will run'
# GC_FOCUS must say "Will run 7 of", not 6.

Two things to know before you trust a local run:

- **Pods do not reach Running** on macOS. Nested containerd inside an emulated
  amd64 container fails with `seccomp is not supported`. The garbage-collector
  tests examine object lifecycle, not workloads, so they are unaffected — but a
  focus that needs a Pod to execute will not pass here.
- **Without a node, 5 of the 7 GC specs still run.** The other two skip with
  `there are currently no ready, schedulable nodes`, which is a precondition,
  not a failure of this control plane.

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

`go.mod`'s `k8s.io/kubernetes`, `k8s.io/client-go` and `k8s.io/apiserver`
replace directives point into generated mirrors under `.build/`
(`k8s-js-mirror`, `clientgo-lean-mirror`, `apiserver-js-mirror`). They are
gitignored but they are **build inputs**: every Go command in this repo,
including a plain `go mod download`, fails on a fresh checkout until they
exist.

`make gen-mirrors` creates them, and it's an order-only prerequisite of
every Go target, so normal `make` usage handles it. It runs every time and
that's fine — the generators sha256-pin each patched upstream file, and the
copy is clonefile-backed, so regenerating ~12k files costs about 3s. The
output is reproducible from the committed tree; if you find a
`.build/*.bak-*` directory lying around, it's debris from a 2026-07-05
investigation (resolved the same day) and can be deleted.

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
send credentials over non-TLS. Go tests that build a
`rest.Config{BearerToken: ...}` directly are not subject to that, which is
why the test suites drive the server that way, and `curl` works fine:

```sh
curl -H 'Authorization: Bearer k8flare-dev-token' localhost:8787/api/v1/namespaces
```

Real `kubectl` against a kubeconfig needs a TLS terminator in front. Node
is already a prerequisite, so no extra tooling is required:

```sh
mkdir -p /tmp/k8ftls && cd /tmp/k8ftls
openssl req -x509 -newkey rsa:2048 -nodes -keyout key.pem -out cert.pem \
  -days 365 -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

cat > proxy.mjs <<'EOF'
import { createServer } from "node:https";
import { readFileSync } from "node:fs";
import { request } from "node:http";

createServer(
  { key: readFileSync("key.pem"), cert: readFileSync("cert.pem") },
  (req, res) => {
    const up = request(
      { host: "127.0.0.1", port: 8787, path: req.url, method: req.method, headers: req.headers },
      (r) => { res.writeHead(r.statusCode, r.headers); r.pipe(res); },
    );
    up.on("error", (e) => { res.writeHead(502); res.end(String(e)); });
    req.pipe(up);
  },
).listen(6443, () => console.log("https://localhost:6443 -> http://127.0.0.1:8787"));
EOF

node proxy.mjs &
```

Then point a kubeconfig at it:

```sh
export KUBECONFIG=/tmp/k8ftls/kubeconfig
kubectl config set-cluster k8flare --server=https://localhost:6443 \
  --certificate-authority=/tmp/k8ftls/cert.pem --embed-certs
kubectl config set-credentials dev --token=k8flare-dev-token
kubectl config set-context k8flare --cluster=k8flare --user=dev
kubectl config use-context k8flare

kubectl get ns
kubectl create deployment tlsprobe --image=nginx
```

Verified end to end with kubectl v1.33.9 on 2026-07-30: `get`, `create`,
and streaming `get -w` all work through it. Use it for local development
only — it terminates TLS with a throwaway certificate and forwards to a
plain-HTTP dev server.

`k8flare-dev-token` is the fallback the default cluster accepts when no
token has been minted and `K3S_TOKEN` is unset — see
[SECURITY.md](../SECURITY.md) before exposing anything.

`kubectl apply --validate=false` is not needed: OpenAPI v2/v3 are served
from Static Assets, and server-side field validation is implemented and
defaults to Strict (`pkg/apiserver/fieldvalidation.go`).
