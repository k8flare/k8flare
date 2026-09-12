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
is **apiserver** (48,784,426 bytes in a `make wasm` build on 2026-09-13, ~18MB of
headroom) — not `gc`, which the original note named and which sits 25MB
clear. `make wasm` prints every chunk's headroom; trust that over any
number written down here.

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

`make test` runs all four lanes below, in order, in about eight minutes
(S64 measured 68.9s / 296.2s / 75.3s for the three slow ones). Only
`test-unit` runs in-process; the other three each start their own
`wrangler dev`, and they cannot share one, because the apiserver lane needs
the controllers off and the other two need them on. Run a single lane by
name while iterating.

| Lane | Runs | Covers |
|---|---|---|
| `make test-unit` | `test-cfruntime` (`go test ./pkg/cfruntime/...` as a real `GOOS=js` test binary under node) + `test-ts` (`vp test`) | pump windows, the JS boundary and promise lifetimes, the Controllers DO's poke/park policy, the watch stream's lifecycle — **no wrangler, no WASM chunks** |
| `make test-apiserver` | `go test ./pkg/apiserver/...`, own `wrangler dev` with `KCM_DISABLED=1` | apiserver, storage, admission, RBAC, tokens — **no controllers** |
| `make test-kcm` | `TestKCMDynamicWorkerControlPlane`, `-timeout 15m` | real KCM/GC/sched dynamic workers |
| `make test-clusterop` | `TestClusterOperatorLifecycle`, `-timeout 15m` | cluster provisioning/teardown, with the workload controllers and scheduler off (`CM_DISABLED`/`SCHED_DISABLED`) |

**Nothing runs these lanes for you.** `ci.yml` is still wired to run the
same work as separate steps — the two `test-unit` halves, then the three
`go test` lanes, each retried once — and until 2026-07-31 it ran only the
apiserver lane, so the real controllers had no automatic gate before then.
But GitHub Actions has been off since 2026-09-12, so the lanes now run only
where you run them; see
[CONTRIBUTING.md](../CONTRIBUTING.md#ci-there-isnt-any-since-2026-09-12).

**Known flake on the three `wrangler dev` lanes, and why wrangler stays on
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
A/B-ing the test lanes — which, with Actions off, means A/B-ing them
locally and accepting that the flake was only ever observed on CI runners.
`ci.yml` and `deps-k3s-update.yml` retry each of the three steps once as a
safety net; a same-lane failure on both attempts is a real failure, not this
flake. Revisit the pin and the retries once #14641 is fixed and
miniflare 5 is stable. (The retries live in workflow files that no longer
execute; the pin is what still matters locally.)

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

The `test-kcm` and `test-clusterop` lanes pay a large one-time cost on the
first poke: workerd
compiles the ~40MB WASM modules in-process. They're the stand-in for the
`kcmdw`/`scheddw` conformance variants on a machine that can't run a Linux
kubelet — a smoke test, not the conformance gate itself.

## Starting and stopping the local conformance harness

There is no script. `scripts/` was removed in 2026-07-08 as a deliberate
simplification, grew back, and was removed again on 2026-09-13 — a shell
wrapper around four commands is a thing to keep working, and its one guard was
itself broken in a machine-specific way (it looked for the host binaries under
a hardcoded directory, so it counted zero and refused to start against two
processes that had come up perfectly).

Run the pieces. Each variant is the same four steps with different flags:

```sh
E=/path/to/assets          # holds e2e/kubeconfig.yaml, e2e/tls/, e2e/kubernetes/
go build -o /tmp/sched-now ./cmd/scheduler
go build -o /tmp/cm-now    ./cmd/controller-manager

# 1. the Worker, behind TLS, with the variant's controllers disabled.
#    host: both disabled. kcm-dw: --var CM_DISABLED:0. sched-dw: --var SCHED_DISABLED:0.
#    all-dw: neither flag, and no host binaries below.
npx wrangler dev -c packages/k8flare-worker/wrangler.jsonc --enable-containers=false \
  --var SCHED_DISABLED:1 --var CM_DISABLED:1 \
  --local --port 8443 --local-protocol https \
  --https-key-path $E/e2e/tls/dev.key --https-cert-path $E/e2e/tls/dev.crt \
  --persist-to /tmp/e2e-state &

# 2. the host control plane the variant asks for
/tmp/sched-now --server=https://127.0.0.1:8443 --token=k8flare-dev-token \
  --data-dir=/tmp/hcp/s --insecure-skip-tls-verify &
/tmp/cm-now    --server=https://127.0.0.1:8443 --token=k8flare-dev-token \
  --data-dir=/tmp/hcp/c --insecure-skip-tls-verify &

# 3. a node
docker restart k8flare-e2e-node
kubectl --kubeconfig=$E/e2e/kubeconfig.yaml get nodes -w   # wait for Ready

# 4. the focus, from .github/workflows/e2e-conformance.yml
$E/e2e/kubernetes/test/bin/e2e.test --kubeconfig=$E/e2e/kubeconfig.yaml \
  --provider=skeleton --num-nodes=1 --disable-log-dump --ginkgo.no-color \
  --ginkgo.focus="$GC_FOCUS"
```

**Count the host processes before you trust a result, and kill by PID when you
are done.** A host `kube-controller-manager` or `kube-scheduler` left pointing
at `127.0.0.1:8443` keeps writing to that cluster, and `pkill -f <name>`
silently matches nothing when the binary was built under a different name.
Sixteen of them once accumulated across one session and made a `replicas=2`
workload look like it churned sixty pods (`docs/platform-verification.md` S62).
The check is one command:

```sh
pgrep -fl 'sched-now|cm-now|k8flare-scheduler|k8flare-controller-manager'
```

Clean up with that same pattern, and **not** with `lsof -ti :8443`. That
matches every process holding a socket on the port, which includes the
*clients* connected to it -- on 2026-09-13 it selected OrbStack's network
helper carrying the e2e node's connection, and `kill -9` on the result took
the machine's whole Docker engine down for eight minutes, along with
containers belonging to other projects. The port is not the thing you
started; the binaries you built are.

`host` expects two, `kcm-dw` and `sched-dw` one each, `all-dw` none. Anything
else and the run is measuring somebody else's processes.

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

**This is the gate.** Upstream conformance is still the Definition of Done,
but since 2026-09-12 it runs here, on a laptop, not in CI: the maintainer
decided not to pay for GitHub Actions (`docs/platform-verification.md` S63),
nothing is waiting for billing to be restored, and every workflow run stops
in about four seconds at GitHub's billing gate. `e2e-conformance.yml` is kept
because its `GC_FOCUS` and `BASELINE_FOCUS` regexes are what the local run
uses; starting and stopping the run is four commands, above.

Verified on macOS (arm64): the garbage-collector focus passes **7/7 in
90–170 seconds** in the `host` variant (`docs/platform-verification.md` S48),
and S65 then measured all three variants — `host`, `kcmdw`, `scheddw` — at
7/7 on `GC_FOCUS` and 11/11 on `BASELINE_FOCUS`.

(An earlier revision of this line said "6/6". That was the focus-extraction
bug in S42 — the shell escaping described below dropped one spec, and the one
it dropped was the spec CI had actually been failing.)

The full recipe, from nothing:

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

# 4. The same focus the gate uses, lifted from the workflow.
./kubernetes/test/bin/e2e.test --kubeconfig=kubeconfig.yaml \
  --provider=skeleton --num-nodes=1 --disable-log-dump --ginkgo.no-color \
  --ginkgo.focus="$(the GC_FOCUS or BASELINE_FOCUS value in e2e-conformance.yml)"
```

**What this harness can and cannot stand in for.** As written above it runs the
resident WASM controllers, which reproduces the `scheddw` and `kcmdw`
variants. These were advisory while CI was the gate; since S65 measured them
identical to `host` on all 18 specs, the local gate treats all three as
required. To reproduce the `host` variant, build `./cmd/scheduler` and
`./cmd/controller-manager`, start `wrangler dev` with
`--var SCHED_DISABLED:1 --var CM_DISABLED:1 --local` (and
`--local-protocol https --https-key-path/--https-cert-path`, which removes the
need for the TLS proxy above), then point both binaries at it with
`--server=https://127.0.0.1:8443 --token=... --insecure-skip-tls-verify`. That
passes the garbage-collector focus 7/7 in about 90 seconds
(`docs/platform-verification.md` S48). Three of the eleven baseline specs fail
there **with the stock node image**, but for a reason outside the control
plane: on Apple Silicon that image's k3s assets are x86-64 and run under
emulation, where `prctl(PR_SET_SECCOMP, …)` returns EINVAL, so containerd
decides seccomp is unsupported and refuses to create any pod sandbox
(`docs/platform-verification.md` S53). Rebuild the node image with arm64 k3s
assets and it is 11/11 (S64, S65); the garbage-collector focus does not need
pods to run and passes as is. For the garbage-collector focus the variant
distinction does not matter: the gc dynamic worker runs in every variant,
because the host has no garbage collector. For anything sig-scheduling it
matters a lot — the
`SchedulerPredicates` specs in `BASELINE_FOCUS` are scheduling-sensitive and a
single small container node is not the runner CI used to use. Treat a local
baseline failure as "unattributed" until you have run the same focus against
`main` with the same harness.

**Copying the focus out of the workflow has a trap.** `GC_FOCUS` is a
single-quoted shell string, so the apostrophe in one spec name is written
`'\''` — the shell's escape, not part of the pattern. Paste it verbatim into a
regex and that spec silently stops matching, which cost a wrong result once
already (`docs/platform-verification.md` S42 訂正). Always confirm the count
first:

```sh
./kubernetes/test/bin/e2e.test ... --ginkgo.dry-run --ginkgo.focus="$FOCUS" | grep 'Will run'
# GC_FOCUS must say "Will run 7 of", not 6.
```

Two things to know before you trust a local run:

- **Pods do not reach Running on Apple Silicon until you rebuild the node
  image.** The stock image ships amd64 k3s assets, and under emulation
  `prctl(PR_SET_SECCOMP, …)` returns EINVAL, so containerd concludes seccomp
  is unsupported and refuses to create a sandbox
  (`docs/platform-verification.md` S53; rebuilding with arm64 assets fixed it,
  S54). The garbage-collector tests examine object lifecycle, not workloads,
  so they pass either way — but a focus that needs a Pod to execute needs the
  rebuilt image.
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
make clean-wasm wasm    # or: make -B wasm, which is what `npm run build:wasm` does
```

Never rely on Make's mtime check after a fresh `git checkout` — it doesn't
preserve timestamps, which is why `ci.yml` always called
`npm run build:wasm` rather than `make wasm`.

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
OpenAPI/discovery documents in `packages/k8flare-worker/assets/`. Nothing
re-runs the generator for you any more (`ci.yml` still has the drift check,
but Actions is off — see
[CONTRIBUTING.md](../CONTRIBUTING.md#ci-there-isnt-any-since-2026-09-12)), so
run it yourself, confirm `git status` is clean, and commit the output. Never
hand-edit a file carrying the `Code generated by k8flare-gen. DO NOT EDIT.`
header; change `cmd/k8flare-gen` instead.

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
