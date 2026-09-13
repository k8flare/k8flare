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
  bytes per binary. Current: front 39.0MB (with the RBAC authorizer);
  group workers 28.7–43.0MB; openapi 57.6MB; customresources 59.1MB;
  node-tunnel 14.4MB (bundled into the shell); scheduler 55.1MB (109.9MB before the lean clientset and informer factory overlays and the two files that dragged the fake clientset and cri-client in);
  printers-core 43.0MB, the other printer groups 13–27MB. Before the
  clientset-scheme / APF / StorageVersion overlays the one-binary apiserver
  was 57.5MB and the CRD handler 73.5MB; `k8s.io/api` alone was 21.4MB of
  code in every binary because those packages' `init` registrations keep
  every type reachable once the package is imported.
- **One dynamic worker per API group, loaded on first use.** The front
  worker only authenticates and routes; each served group, the CRD handler,
  OpenAPI, the scheduler and each printers group is its own binary, so a
  request loads only what it touches and every binary stays under the cap.
  Two js overlays make that possible without touching what upstream does:
  the clientset scheme registers nothing (each worker registers the groups
  it imports), and the APF filter and StorageVersion manager no longer pull
  every group's informers and typed clients.
- **One resident Go instance per isolate**, dispatched per request by the
  Loader bootstrap. Go timers and fetches only live inside a request
  context, and the Go runtime keeps a single scheduled wake-up, so once a
  context ends every goroutine waiting on a timer stalls until the next
  request enters the instance (observed 2026-09-13: a custom resource
  create finished only when the next request arrived, and workerd reported
  the isolate as hung). The bootstrap therefore keeps each request's
  context open for a pump window after responding and re-enters Go every
  250ms during it: 5s for group workers, 30s for customresources and the
  scheduler, whose controllers otherwise only run while pumped. Watches stream from it (Content-Encoding: identity, or
  the runtime gzips JSON and holds the stream until it closes).
- **The agent is k3s.** `packages/agent` embeds `k3s/pkg/agent` unchanged except
  the `deps.KubeConfigOverride` hook, because TLS terminates at the edge and
  client certificates never reach the control plane. Node identity on the
  API is the bearer token `node:<name>:<node password>`, the secret k3s
  already registers with the supervisor.
- **Local verification only** while GitHub Actions is off: `make test`
  starts `wrangler dev` itself; the node path is checked by hand against
  `devtls` and an OrbStack VM. The harness tests are smoke tests for the
  worker plumbing; the definition of done is upstream's conformance suite
  run through Sonobuoy with a narrowed focus, against the VM cluster, once
  the pieces below stop moving.

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
  bootstrap, chunk assembly, routing and the tunnel hand-off. The DO stub is
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
the controllers (kube-controller-manager) and the production deploy check.

## Known limitations

- **Per-kind code that remains**, all in `packages/apiserver-core` and
  registered through the registry's hooks: `podStrategy` (upstream's
  graceful-delete rule, which upstream keeps on the internal Pod type),
  `assignPodCIDR` (the nodeipam controller's job until controllers run),
  the namespace bootstrap, `pods/binding` (upstream's BindingREST lives on
  the internal Pod type), and the scheduler wake-up. The served resources
  themselves are generated from upstream's discovery documents
  (`scripts/genresources`), and field labels, defaults and PodLogOptions
  come from upstream's `AddToScheme`.
- **The scheduler runs only while woken.** A Pod written without a node
  wakes the scheduler worker, which runs the real kube-scheduler and its
  informers in that isolate for a bounded window per wake-up; nothing keeps
  it alive at idle. A poke holds its request until the active and backoff
  queues drain (20s at most) and answers 202 while work remains, which the
  entrypoint turns into the next poke. Pod writes without a node and every
  Node write poke it, so a Pod created before its Node is retried when the
  Node arrives. The scheduler's informers are why apps/v1, policy/v1,
  resource.k8s.io/v1 and replicationcontrollers are served: an informer
  on an unserved resource never syncs and the scheduler never starts. Controllers, Services, kube-proxy and cluster DNS are
  still absent; Pods need `dnsPolicy: Default`.
- **CRD OpenAPI v3 is published** by the customresources worker
  (upstream's openapiv3 controller); the front merges its `/openapi/v3`
  root with the openapi worker's and routes group documents by group.
  `/openapi/v2` still covers built-in groups only. Conversion webhooks are
  untested.
- **Kubelet access goes through the k3s tunnel.** The agent's
  remotedialer WebSocket to `/v1-k3s/connect` is authenticated with its
  node token and handed to the node's `NodeTunnel` Durable Object, which
  runs remotedialer's server (packages/node-tunnel, a wasm module bundled
  into the shell Worker) and proxies `/node/<name>/<kubelet path>` through
  the session. `pods/log` uses it; exec/attach/port-forward and a
  vault-signed kubelet client certificate (today: bearer token +
  InsecureSkipVerify inside the tunnel) are the follow-ups. After
  hibernation the DO closes the agent's socket so k3s reconnects.
- **Authorization is RBAC** (`packages/apiserver-authz`): `system:masters`
  passes unconditionally, everyone else is checked against Roles,
  RoleBindings, ClusterRoles, and ClusterRoleBindings read live from kine,
  unioned with upstream's bootstrap policy. There is no Node authorizer,
  so `system:node` is statically bound to the `system:nodes` group.
- **The Cluster DO compacts on write**: once the log is 1,000 revisions
  past the last compaction the write deletes every older row that is not
  the latest for its key (and every older tombstone) in one statement, so
  the table stays at live keys plus the last 1,000 revisions. A watch from
  a revision below the compaction point gets 410 `Expired` and client-go
  relists; lists always read the latest state.
- **`Content-Encoding: identity`** is verified against workerd and wrangler
  dev only; the production edge is untested.
- `kubectl get` columns come from upstream's printers, one dynamic worker
  per API group (`packages/printers-*`, generated by `scripts/genprinters`
  from the same served-resource list), reached over a Service Binding RPC
  to the `Printers` entrypoint of the same Worker. Only the served kinds are
  linked; a kind added to `api/discovery` needs `make gen`.
- `/openapi/v2` and `/openapi/v3` are computed, not static: the apiserver
  forwards them over the `OPENAPI` Service Binding to the `OpenAPI`
  entrypoint, whose dynamic worker (`packages/openapi`) runs the same
  route installer and kube-openapi's builders. The document set is what
  the installer serves, so CRDs later mean feeding the worker their
  schemas (apiextensions' openapi builder + kube-openapi's aggregator),
  not regenerating a file. Its CRD informer talks to its own handler
  through an in-process loopback client: the same request sent through the
  Service Binding back into the same dynamic worker never returned.

## Known edge cases / watch-fors

- `TestSchedulerWakesOnNode` once failed with "connection reset by peer"
  on the Node create only when the whole lane ran; it passes alone (3/3).
  Not understood yet; watch for it and do not retry it away.

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
- Edge mTLS with a Cloudflare-managed CA (forwarding the agent's CSR to the
  client-certificate API) remains the candidate that would remove the
  kubeconfig hook entirely; it needs a zone hostname and cannot be tested
  locally.
