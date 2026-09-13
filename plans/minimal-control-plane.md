# Plan: A minimal k8s control plane on Cloudflare that a stock k3s agent can join

## Goal

Run the Kubernetes control plane on Cloudflare Workers and Durable Objects
with as little code of our own as possible: k8s.io/apiserver serves the API,
k3s's own agent runs the node, and k8flare is only the glue between them and
the platform. The first milestone, reached on 2026-09-13, is one OrbStack VM
joining, going Ready, running a Pod that answers on its Pod IP and streams
its log, and removing it again.

## Locked decisions

- **k8flare is glue, not a reimplementation.** Where upstream code has to
  change, the change is a sha256-pinned overlay applied by `scripts/mirror` at
  build time, never an edited copy. No per-resource handling in k8flare code
  where upstream can do it; the remaining per-kind branches are listed under
  Known limitations as debt.
- **External types only.** Linking upstream's registries (internal types,
  `printers/internalversion`) was measured at 92MB after wasm-opt against
  the Worker Loader's 64MiB cap. The API is k8s.io/apiserver's installer over
  `genericregistry.Store`, and only the defaulters from
  `k8s.io/kubernetes/pkg/apis/*/v1` are linked (+3.5MB).
- **Size is a gate, not a guideline.** `make wasm` fails above 67,108,864
  bytes. Current: 57,345,308 bytes.
- **One resident Go instance per isolate**, dispatched per request by the
  Loader bootstrap. Watches stream from it (Content-Encoding: identity, or
  the runtime gzips JSON and holds the stream until it closes).
- **The agent is k3s.** `packages/agent` embeds `k3s/pkg/agent` unchanged except
  the `deps.KubeConfigOverride` hook, because TLS terminates at the edge and
  client certificates never reach the control plane. Node identity on the
  API is the bearer token `node:<name>:<node password>`, the secret k3s
  already registers with the supervisor.
- **Local verification only** while GitHub Actions is off: `make test`
  starts `wrangler dev` itself; the node path is checked by hand against
  `devtls` and an OrbStack VM.

## Current-state anchors

- `packages/apiserver-registry/zz_generated_resources.go` (from `scripts/genresources`): the
  served surface, filtered from upstream's discovery documents.
  `packages/apiserver-registry` builds a generic store for each; pods get upstream's
  graceful-delete rule, nodes a static PodCIDR in `BeginCreate`.
- `packages/apiserver-kine`: `storage.Interface` over the Cluster DO's
  revisioned key-value log; `Watch` dials the DO over a WebSocket and emits
  the WatchList bookmark at the end of the snapshot.
- `packages/apiserver-supervisor`: the nine k3s join endpoints,
  CSR signing, node passwords, CAs in the DO under `/vault`.
- `packages/worker-bridge`: the Go↔Loader bridge (streamed responses, WebSocket client).
- `packages/cluster-store`: the Cluster DO; `packages/control-plane-worker`:
  bootstrap, chunk assembly, routing and the parked tunnel. The DO stub is
  handed to the dynamic worker's env directly; the old finding that a
  Loader env cannot carry a DO covered namespaces, not stubs.
- `scripts/mirror/main.go`: the overlays (apiserver storage factory, tracing
  exporter, CEL parser, installer hub version, k3s kubeconfig hook).

## Design

See README.md for the component list. Two things that are not obvious from
the code:

1. **Why WatchList matters.** client-go's reflectors ask for
   `sendInitialEvents=true` and wait for a BOOKMARK annotated
   `k8s.io/initial-events-end`. Without it nothing syncs, and the kubelet's
   protobuf informers were the only ones that appeared to work because the
   runtime does not compress protobuf. The DO sends `snapshot-end`; Go turns
   it into the bookmark.
2. **Why wrangler dev runs without CLAUDECODE.** Its AI-agent mode buffers
   JSON responses for its Local Explorer, which stalls JSON watches the same
   way gzip does.

## Commit sequence

Done, on `feat/minimal-rewrite`:

1. Size gate (`scripts/mirror`, then under hack/; probe measurements in the commit message).
2. apiserver on the Loader with the Cluster DO as its store; ConfigMap verbs
   through client-go.
3. Watch from the resident instance.
4. Supervisor, node identity, the agent embedding.
5. First pod: join, run, reach, log, delete.
6. Auth fixes (node tokens cannot self-register; empty join token opens
   nothing) and the pod lifecycle test.

Next:

7. The `/simplify` review: upstream registrations instead of hand-written
   conversions, generated resource table, shared JS callbacks, the DO's
   single-insert write path.

Next:

8. Repository layout: root `wrangler.jsonc`, `packages/{component}-{part}`
   (plans/repository-layout.md). Done 2026-09-13.

Next: see Known limitations, in this order — compaction of the DO log,
the kubelet tunnel, RBAC, then the scheduler and controllers.

## Known limitations

- **Per-kind code that remains**: `podStrategy` (upstream's graceful-delete
  rule, which upstream keeps on the internal Pod type) and `assignPodCIDR`
  (the nodeipam controller's job until controllers run) are the two
  `if resource == ...` branches left. The served resources themselves are
  generated from upstream's discovery documents (`scripts/genresources`), and
  field labels, defaults and PodLogOptions come from upstream's
  `AddToScheme`.
- **No scheduler, controllers, Services, kube-proxy, or cluster DNS.** Pods
  need `spec.nodeName` and `dnsPolicy: Default`.
- **`pods/log` reaches the kubelet on its InternalIP over plain HTTP**
  (`--kubelet-plain-port`), which only works while the Worker runs on the
  same machine as the VM. The `/v1-k3s/connect` tunnel is accepted and
  parked; nothing dials back through it.
- **Authorization is allow-all** for any authenticated identity.
- **The Cluster DO never compacts**: every write appends a row, and `list`
  and the watch snapshot scan the whole history of a prefix.
- **`Content-Encoding: identity`** is verified against workerd and wrangler
  dev only; the production edge is untested.
- `kubectl get` shows only NAME and CREATED AT (default table convertor).

## Known edge cases / watch-fors

- A `k3s` binary must have run once on the node: the agent uses the
  containerd, runc and CNI binaries it unpacks. The VM's stock binary is
  v1.36.2+k3s1; the agent is built against the v1.36.5-dev pin.
- WARP connected inside the VM captures DNS and routing; `host.orb.internal`
  stops resolving.
- After deleting a pod, `crictl pods` keeps the sandbox record until the
  kubelet's GC runs; containers are gone at once.
- The nodes watch with `fieldSelector=metadata.name=` arrives at storage as a
  non-recursive single-key watch (`Store.Watch` optimizes `MatchesSingle`).

## Accepted tradeoffs / future work

- Static PodCIDR allocation in the apiserver instead of the real nodeipam
  controller, until controllers run.
- Hand-written review APIs (SSAR/SAR/TokenReview always allow) until RBAC.
- The kubelet log proxy will move onto the k3s tunnel (remotedialer) once
  the server side of `/v1-k3s/connect` exists.
- Edge mTLS with a Cloudflare-managed CA (forwarding the agent's CSR to the
  client-certificate API) remains the candidate that would remove the
  kubeconfig hook entirely; it needs a zone hostname and cannot be tested
  locally.
