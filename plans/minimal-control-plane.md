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
  bytes per binary. `wasm-opt -Oz` is the slow step (43s for the
  scheduler on 16 cores; `-Os` would save 7s and cost 5.3MB, `-O1` lands
  at 63.8MB, so -Oz stays); a warm `go build` is 1.5s and byte-identical
  when the binary's own inputs did not change, so the Makefile keeps the
  intermediates (`.SECONDARY`), records the raw binary's sha256 next to
  each `.opt.wasm` and skips wasm-opt when it matches, and bounds
  wasm-opt to `BINARYEN_CORES=2` under `--jobs=8` (8 unbounded wasm-opt
  processes on 16 cores took over 30 minutes for the full set). Current: front 39.0MB (with the RBAC authorizer);
  group workers 28.7–43.1MB; openapi 58.5MB; customresources 59.1MB;
  controllers 48.8MB (with the garbage collector);
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
  250ms during it: 5s for group workers, 30s for customresources, the
  scheduler and the controllers, whose controllers otherwise only run
  while pumped. A watch stream whose request context ended is not reported
  closed to Go either (a body read blocks forever), so the resident
  scheduler and controllers give every informer watch a 15s lifetime
  through `BindingTransport.WatchLifetime`; the reflector re-watches from
  its last resource version, and the instances themselves stay resident
  (one `RegisteredNode` event per node, node health probes that outlive a
  poke). A Go instance that exits or fails to instantiate is logged by the
  Loader bootstrap and dropped, so the next poke instantiates again, and
  the poke chains log their failures; before that a dead controllers
  instance was invisible (2026-09-14, after a burst of node and namespace
  deletes, no controller acted for 15 minutes and nothing was logged). The
  first death the logging caught was the core group worker: Go's wasm
  runtime reported "all goroutines are asleep - deadlock!" with the
  `ws-message` callback blocked on a full 256-slot kine watch channel
  while every other goroutine waited on a `setTimeout(0)` that could not
  fire with JS stuck inside Go; the isolate had been frozen for the whole
  advisory run before the detector fired and the bootstrap
  re-instantiated it. No JS callback in the bridge may block now: the
  WebSocket callback appends to a queue and a Go goroutine drains it into
  `Messages`; a watch whose consumer never drains (a response stream that
  died unnoticed) is closed once 32 MiB is queued, which ends the kine
  watch and makes the reflector relist. The resident scheduler and
  controllers also use upstream's client QPS/burst (50/100 and 20/30)
  instead of rest.Config's 5/10, which throttled them under e2e load. The
  next hour-long run ended with the core group worker at 4.1 GB of Go
  heap ("fatal error: out of memory") because a watch's server side never
  learned that its client had gone: the DO's `watchers` count (now in
  `GET /stats`) grew by 36 every 30s at idle, one per informer re-watch,
  and killed `kubectl -w` sockets stayed too. `BindingTransport` now ties
  every fetch to an `AbortController` fired by the request context or by
  closing the body, so a client's cancel reaches the front, the group
  worker's `stream-cancel`, and the DO socket, and the Loader bootstrap
  hands each request's `signal` to Go. Measured in wrangler dev
  (2026-09-14): neither the ReadableStream `cancel` nor the request abort
  fired once, for a killed external `curl -N` watch or for a Go-side
  abort across a service binding (0 callbacks over 3 minutes while the
  socket count rose from 55 to 112), so no cancellation signal reaches a
  callee in this runtime and the hooks stay in place only for Track 8 to
  test at the edge. The constraint is per request, not per isolate: a
  subrequest stream is delivered only while the request that opened it is
  alive (its handler plus the pump `waitUntil`); once that poke's window
  closes, every stream it opened is silently dead even while later pokes
  keep the isolate busy (measured: an external `curl -N` saw the DO close
  its watch as EOF after 69s, while the continuously poked controllers
  isolate never saw the same close on its own streams and its caches went
  stale, 0/15 required specs). So each accepted poke ends every stream the
  resident worker still holds (`BindingTransport.AbortOnWake` tracks them,
  `bridge.EndTrackedStreams` closes the pipes and aborts the fetches) and
  the informers re-watch from their last resource version inside the new
  request. The server side is bounded by the DO: any watch socket older
  than `WATCH_LEASE_MS` (60s) is closed on the next write or watch accept,
  and a socket whose reader falls 1 MiB behind is closed by the group
  worker (the 4.1 GB heap was about 120 dead sockets each buffering to
  the earlier 32 MiB cap). Node status writes poke the resident workers
  only when the spec, labels, allocatable or Ready condition changed, so
  kubelet heartbeats no longer drive the re-watch rate. wrangler dev does
  not enforce the 128 MB isolate limit; production does, so the leak
  would have taken minutes there instead of an hour. A goroutine parked
  in `await(reader.read())` on a dead stream never wakes; that leaks a
  few KB per informer per wake and is accepted.
- **A Cron Trigger wakes the resident workers every minute** (decision
  2026-09-14, replacing "no alarms, no polling"): `scheduled()` in the
  shell awaits `Scheduler.poke()` and `Controllers.poke()`, so timers that
  only advance while pumped (nodelifecycle's monitor, workqueue AddAfter
  retries, CronJob, the garbage collector's 30s discovery sync) get one
  window per minute even with no writes. Cost: 1,440 invocations a day
  per worker, each a poke that returns 204 within 1.5s when idle plus its
  pump window. The garbage collector (upstream's, over a metadata
  informer per served resource and a deferred discovery RESTMapper)
  joins the controllers worker on the same wake model; its monitors are
  re-established per wake like every other informer.

- **Advisory e2e (2026-09-14, per focus group, one wrangler per group):**
  Garbage collector 3/7 after the collector landed (was 1/7): deletion
  cascades work, and the orphan and foreground cases were failing because
  the generic store left `EnableGarbageCollection` false, so it never
  wrote the `orphanDependents`/`foregroundDeletion` finalizers and every
  policy degraded to background; upstream defaults that flag to true
  (`pkg/server/options/etcd.go`) and both the generic store and the CR
  stores now set it. LimitRange defaults 0/1 (no LimitRanger admission,
  internal types); ConfigMap 2/6 and Secrets 1/5 (empty-key validation
  lives in upstream's internal-type strategies; pod log reads returned
  "unknown", unexplained); Namespaces [Serial] 5/8 (100 namespaces do not
  delete fast enough, and a Service-removal case timed out); ReplicaSet
  4/7 and Deployment 8/11 (`resourcequotas` and the scale subresource are
  not served; reaching a replica pod from the runner needs pod
  networking); Pods and Job exceeded the 25-minute cap; Services and
  ServiceAccounts did not run.

- The store's `strategy` does not implement
  `GarbageCollectionDeleteStrategy`, so a delete with no propagation
  policy is background for every resource; upstream's v1
  ReplicationController defaults to orphan instead. kubectl has sent an
  explicit policy since 1.20, so neither kubectl nor e2e sees the gap.

- The CRD specs fail once the cluster has been idle for more than the
  60s watch lease: the establishing controller in the customresources
  worker stops reacting, so new CRDs never reach Established and the
  discovery wait times out (26 creates against 3 status writes in one
  run). Measured 2026-09-14: a CRD created immediately after a restart is
  Established in 1s; after 180s of idle none is, at one or four ginkgo
  processes, with or without load in between, and the worker logs
  nothing at all for two minutes. The lease closes the socket while the
  isolate is suspended, and `WebSocket.Close` only asked JS to close and
  waited for the close event to run `finish()`, so that event never
  arrived and the Go side kept the watch: kine's reader stayed blocked
  and the reflector believed its watch was healthy. `Close` now finishes
  the socket itself, and sockets dialed outside a request (an informer's
  own watch, never one serving a client) are closed at the start of the
  next wake so the reflector re-dials inside a live request.

- The 60s lease also means an external `kubectl -w` is disconnected once
  a minute (measured: EOF at 69s) and reconnects from its last resource
  version, which is within watch semantics but visible in client logs. Watches stream from it (Content-Encoding: identity, or
  the runtime gzips JSON and holds the stream until it closes).
- **The agent is k3s.** `packages/agent` embeds `k3s/pkg/agent` unchanged except
  the `deps.KubeConfigOverride` hook, because TLS terminates at the edge and
  client certificates never reach the control plane. Node identity on the
  API is the bearer token `node:<name>:<node password>`, the secret k3s
  already registers with the supervisor.
- **Local verification only** while GitHub Actions is off: `make test`
  starts `wrangler dev` itself; the node path is checked by hand against
  `devtls` and an OrbStack VM. The harness tests are smoke tests for the
  worker plumbing; the definition of done is upstream's `e2e.test` run by
  `make e2e` with the narrowed focus sets in `scripts/e2e/focus.go`
  (Sonobuoy would need Services, kube-proxy and CoreDNS first). The
  framework's `BeforeEach` waits for the `default` ServiceAccount of each
  test namespace, which only kube-controller-manager's serviceaccount
  controller creates, so the gate depends on the controllers worker: the
  first run (2026-09-14) failed all 16 required specs there, one baseline
  was taken with `--e2e-verify-service-account=false`, and the controllers
  worker carries the serviceaccount controller and is poked on namespace
  writes so the flag is not needed. The same `BeforeEach` then waits for
  `kube-root-ca.crt`, so the worker also runs root-ca-cert-publisher with
  the CA the supervisor serves at `/cacerts` (the cluster's server CA; a
  production edge presents Cloudflare's certificate instead, which only
  matters once pods can reach `kubernetes.default`). Namespace deletion follows
  upstream's life cycle: the core worker's `namespaces` deleter marks the
  namespace Terminating (the `registry.Deleters` hook, transcribed from
  upstream's namespace REST) and serves `namespaces/finalize`, and the
  controllers worker runs upstream's namespace controller, which empties
  the namespace through the metadata client and discovery before the final
  delete; without it, deleted namespaces left their pods behind and the
  scheduling specs' "stable cluster" wait never returned. Every
  group worker rejects creates in a Terminating namespace with the
  Forbidden status upstream's NamespaceLifecycle admission produces
  (`registry.NamespaceLifecycle`, one kine read of the namespace per
  create); without that cause the root CA publisher and serviceaccount
  controllers recreate their objects while the namespace controller
  empties the namespace, and the two back off against each other for
  minutes. Custom resource creates go through the customresources worker
  and are not guarded yet. With the serviceaccount, root-ca-cert-publisher and namespace
  controllers in place the required set passes 15/15 (2026-09-14,
  `PROCS=4`, 104s).

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
  the namespace and kubernetes-Service bootstrap, `pods/binding`
  (upstream's BindingREST lives on the internal Pod type), and the
  scheduler and controller wake-ups. PodCIDRs come from the real nodeipam
  controller now. The served resources
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
  Node arrives. The same write hooks poke the controllers worker
  (`packages/controllers`): writes to pods, nodes, services, endpoints,
  replicationcontrollers, apps/batch/discovery resources and leases, so the
  real controllers (deployment → replicaset → pods, nodeipam PodCIDR,
  nodelifecycle taints, endpoints and endpointslices, jobs and cronjobs)
  run only while there is work; `Idle()` reads the workqueue depths through
  client-go's workqueue metrics provider. The scheduler's informers are why apps/v1, policy/v1,
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

- Field selectors read the object's JSON, so an absent boolean field has
  no value; upstream renders the zero value as "false"
  (`spec.unschedulable=false` is how the e2e framework lists schedulable
  nodes). `attrsFor` now treats a missing field as "false" whenever the
  selector compares against "true"/"false".
- `limitranges` is served, but the LimitRanger admission plugin is not
  (it works on internal Pod types); the e2e spec that expects defaults to
  be applied to a Pod is advisory for that reason.

- `wrangler dev` closes an idle keep-alive connection after 5s, and a
  client that reuses it right then gets "connection reset by peer" or EOF
  (measured 2026-09-14: a second request on the same connection succeeds
  after 4.8s idle and fails after 5.0s; a fresh connection always works).
  `TestSchedulerWakesOnNode` sleeps exactly 5s between two POSTs, which is
  why it failed intermittently. The harness clientset therefore caps its
  transport's `IdleConnTimeout` at 2s; kubectl and e2e.test reach the
  worker through `devtls` and were never affected.

- `namespaces` still advertises `deletecollection`: the generic store's
  `DeleteCollection` calls its own `Delete`, not the Terminating deleter,
  so `kubectl delete ns -l ...` removes the namespaces without emptying
  them. Upstream has no such verb on namespaces; hide it or route it
  through the deleter.

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
- **Controllers as a portable core plus two thin `cmd/` entrypoints.**
  `packages/controllers` already has this shape even though only the wasm
  side is built: `controllers.go` imports only client-go and a
  `*rest.Config`, no `syscall/js` or `bridge.*`; `cmd/controllers-wasm`
  is the only file that knows about poke/pump-window/`BindingTransport`.
  Every new controller should start from the same split: the reconciler
  package stays buildable with a plain `GOOS=linux go build` against any
  cluster, `cmd/<name>-wasm` (js) wraps it in `bridge.Serve` plus the
  poke loop, and a second `cmd/<name>` (no build tag) wires
  `rest.InClusterConfig()` (or a kubeconfig flag) and blocks in
  `Run(ctx)` — an ordinary container, deployable on stock k8s with no
  k8flare code in the image. Anything that only makes sense against
  Cloudflare goes behind an interface the core calls; only the wasm
  `cmd/` supplies the Cloudflare implementation, the plain `cmd/` supplies
  a different one (or none).
- **`type: LoadBalancer` Services.** `packages/agent` already sets
  `DisableLoadBalancer: true` (k3s's own ServiceLB/klipper-lb is off).
  Planned: a `loadbalancer` controller built with the pattern above — the
  portable core watches Services of `type: LoadBalancer` and writes
  `.status.loadBalancer.ingress`; a `Provisioner` interface it calls is
  what actually exposes the Service, and only the wasm `cmd/` gets a
  Cloudflare-backed implementation. First cut, since the Worker is already
  the single edge ingress: expose the Service as another routed
  hostname on the same Worker rather than provisioning a real external
  LB (one hostname per Service, e.g. derived from name/namespace, matches
  real `LoadBalancer` semantics — one external identity per Service —
  better than a single shared hostname with path routing). On stock k8s
  the same core, given a host-network or MetalLB-style `Provisioner`,
  behaves like an ordinary ServiceLB replacement (or is skipped where a
  real cloud LB controller already owns the class).
- **Cloudflare Access as a third authenticator.** Today authentication is
  only `AdminToken` and `NodeToken` (`packages/apiserver-auth`); there is
  no notion of a human user. Planned: an `authenticator.Request` that
  reads the `Cf-Access-Jwt-Assertion` header (not `Authorization`, per
  Cloudflare's own guidance — the `CF_Authorization` cookie is
  browser-only and not guaranteed to arrive), verifies against the team's
  JWKS (`https://<team>.cloudflareaccess.com/cdn-cgi/access/certs`,
  selecting the key by the JWT's `kid`), checks `iss` and `aud`, and
  returns `user.Info{Name: <email claim>}`. RBAC needs no new code for
  this — bind the email directly as a `User` subject in an ordinary
  RoleBinding/ClusterRoleBinding, the same as OIDC users on stock k8s.
  Group-based bindings need one more call to `/cdn-cgi/access/get-identity`
  with the same JWT, since group membership is not guaranteed to be in the
  compact JWT itself. This authenticator only fires for requests that came
  through an Access-protected hostname; kubectl needs a way to carry that
  header (a service token, or an exec credential plugin that runs
  `cloudflared access login`-style token retrieval) — it is a path for
  human users alongside, not instead of, the admin and node tokens.
- **Pod-on-Containers: a second, Cloudflare-Container-backed node type.**
  Already built once, pre-`feat/minimal-rewrite` (see `main`,
  `main-legacy-full-history`, `backup/pre-rewrite-2026-09-13`; commits
  around `7d122b5`/`29ace6a`), as `workers/nodes`: a `VirtualNode`
  Durable Object registers a fake Node (`cf-containers-<pool>`), heartbeats
  its Lease on a ~10s alarm, and reconciles Pods scheduled to it by
  list-and-diff against the apiserver on that same alarm (a documented
  latency tradeoff — no push path from the storage layer). Each Pod became
  a `PodContainer{Small,Medium,Large}` DO wrapping one real Cloudflare
  Container instance; **image and instance size are fixed at deploy time
  on Cloudflare Containers** (a platform constraint, not k8flare debt), so
  a Pod's summed resource requests rounded up to the nearest size tier and
  were checked against a deploy-time image allowlist. It was verified
  end-to-end against real wrangler dev + real Docker at the time (two real
  bugs found and fixed that way: Lease `renewTime` needing microsecond
  precision, and a SIGTERM'd container misclassified as Succeeded). v1's
  known gaps: no Pod IP, `restartCount` stuck at 0, ~10s reconciliation
  latency, no UDP (blocks CoreDNS on this node type). **`main` later
  recorded this as broken** (a correction to the provisioning explanation,
  then a recorded regression) — reviving it starts with reading that
  history, not re-diagnosing from scratch against the current, much-changed
  apiserver/controllers/RBAC. Unlike the portable-controller pattern above,
  this backend is Cloudflare-only by nature — there is no vanilla-k8s
  equivalent to running a Pod as a Cloudflare Container, so only the
  wasm/DO side is meaningful; no matching plain `cmd/` to design.
- **R2 storage for Pod-on-Containers Pods**, also already built and
  verified once (`0de2d66`): on PVC mount, `VirtualNode.reconcileOnePod`
  called an internal `mint-r2-credentials` endpoint (namespace + claim name
  only — no CSI attribute schema needed on this path) and injected the
  scoped, temporary result as standard `AWS_ACCESS_KEY_ID`/
  `AWS_SECRET_ACCESS_KEY`/`AWS_SESSION_TOKEN` plus `R2_ENDPOINT`/
  `R2_BUCKET`/`R2_PREFIX` env vars via `@cloudflare/containers`'
  `startOptions.envVars` — the app talked to R2 with a normal S3 SDK, no
  mount at all. Credential refresh was a known rough edge:
  `restartPolicy: Always` Pods past their credential's TTL got proactively
  restarted to re-mint (not zero-downtime; an in-image refresh sidecar was
  left as follow-up). Cloudflare Containers has since gained FUSE support
  (Nov 2025), so a Pod's own image can now additionally mount its scoped
  R2 credential as a real filesystem with a bundled FUSE adapter
  (tigrisfs/s3fs/gcsfuse) instead of, or alongside, using the S3 SDK
  directly — same POSIX/performance caveats as any object-storage-over-FUSE
  setup apply
  (https://developers.cloudflare.com/containers/examples/r2-fuse-mount/,
  https://developers.cloudflare.com/changelog/post/2025-11-21-fuse-support-in-containers/).
  Minting a scoped credential and handing it to a Container's env is just
  an API call, no privileged node access, so this whole CSI-equivalent role
  for Pod-on-Containers is DynamicWorker-native already — unlike the
  `mountpoint-s3-csi-driver` track above, which necessarily runs on the
  real k3s node.
- **A Cloudflare-native alternative to the custom node-tunnel.** Today's
  `packages/node-tunnel` is a from-scratch reimplementation of remotedialer's
  server (patched into `github.com/rancher/remotedialer` by `scripts/mirror`
  to add `ServeConn`, since a DO's hibernatable WebSocket isn't an
  `http.Hijacker`) solely because a Worker cannot otherwise dial an
  unroutable node. Cloudflare Tunnel alone doesn't remove that constraint —
  it's origin-to-Cloudflare only, and a Worker still reaches it either via a
  routed public hostname (back out through the internet, not a direct bind)
  or through **Workers VPC** (`vpc_services`/`vpc_networks` bindings,
  supporting both `fetch()` and raw `connect()` — the same shape
  `BindingTransport.DialTLSContext` already needs), which can reach a
  destination "regardless of how it's connected: Tunnel, Mesh node, or WAN
  on-ramp." **Cloudflare Mesh** (GA'd April 2026) is the better conceptual
  match specifically: every enrolled node gets a stable, private per-node
  "Mesh IP" reachable over TCP/UDP/ICMP with either side initiating, which
  fits "the control plane dials this exact node" better than Tunnel's
  hostname-routing shape. **The one fact that decides whether this is
  viable at all is unconfirmed: Workers VPC's docs only show plain Worker
  `fetch()` handlers, never a call from code running inside a Durable
  Object** — and this project's `TUNNEL` binding is invoked from Go/wasm
  running inside the `NodeTunnel` DO via `worker-bridge`, so DO-callability
  has to be spiked (one OrbStack node enrolled in Mesh, a `vpc_services`
  binding, a `connect()` call from inside a DO) before anything else here
  is worth planning. Workers VPC is also still **beta** ("features and APIs
  may change"), a real risk for the production edge, not just local dev.
  Today's per-node bearer token (`node:<name>:<password>`) selecting one
  `NodeTunnel` DO by name would also need to become
  credential-selects-Mesh-IP/service-id instead — a redesign, not a
  drop-in swap. If the DO spike succeeds, the payoff is real: dropping
  ~200+ lines of custom remotedialer-server/session-adapter code and the
  `ServeConn` mirror overlay in favor of a Cloudflare-maintained connection
  path.
- **Admission control: the hook point already exists empty, not missing.**
  `packages/apiserver-installer/installer.go` and
  `packages/customresources/customresources.go` already call
  `admission.NewChainHandler()` with zero arguments — adding a plugin is
  just constructing it and passing it in, the same "call upstream's real
  constructor" pattern RBAC/controllers already used, at a point that
  already exists. The plugin packages are already vendored under
  `.build/apiserver-mirror/pkg/admission/plugin/` (`webhook/`,
  `namespace/lifecycle/`, `resourcequota/`, `policy/`, `cel/`,
  `authorizer/`) with clean-looking imports (no etcd/grpc/otel), though
  `webhook/generic` pulls in `pkg/admission/plugin/cel` for match
  conditions — a second, real CEL dependency to actually build, unlike the
  ShardSelector CEL this project already stubs out. `Validating`/
  `MutatingWebhookConfiguration` support both a raw `url` (a plain
  outbound HTTPS call, no new networking needed) and a `service`
  reference (blocked on the still-absent Services/kube-proxy path) — url
  webhooks are the clean first cut. Note: the specific "reject a create in
  a Terminating namespace" behavior that would have been NamespaceLifecycle's
  job is already covered by a narrower, hand-written
  `registry.NamespaceLifecycle` hook (one kine read of the namespace per
  create) — so **LimitRanger is the cleanest next full-plugin adoption**
  (still genuinely missing, per Known edge cases above), not
  NamespaceLifecycle. ServiceAccount's admission-time token-volume
  injection (distinct from the serviceaccount *controller*
  `packages/controllers` already runs) wasn't located in this pass and
  needs a follow-up look before assuming it's covered too.
- **Gateway API, not classic Ingress, as routing logic inside the front —
  not a `Provisioner` like LoadBalancer.** Gateway API is GA and still
  active (v1.6, June 2026, added TCPRoute/UDPRoute); classic
  `networking.k8s.io/Ingress` is legacy at this point. Upstream reuse here
  is weaker than the RBAC/controllers precedent: `sigs.k8s.io/gateway-api`
  ships the API *types* as a Go module, not a reusable reconciler the way
  `k8s.io/kubernetes/pkg/controller/*` was — this project would write its
  own HTTPRoute/Gateway reconciliation. **Correction to an earlier draft
  of this note**: dispatch and domain-assignment are two separate problems,
  and only one of them is free. `wrangler.jsonc` has no `routes`/custom-domain
  config today, and `index.ts`'s `fetch` branches on path only
  (`/v1-k3s/connect` vs. everything else) — there is no Host-header
  dispatch yet. Once traffic for a hostname already reaches this Worker,
  Host/path → Service/Pod dispatch can live entirely as internal routing
  logic (one more Loader dynamic worker, keyed by Host header the same way
  `/v1-k3s/connect` is special-cased today) — that part is free, matches
  the user's "DynamicWorker for routing" instinct, and needs no
  `Provisioner`-interface like LoadBalancer's. But *getting* a brand-new
  hostname to reach this Worker at all is a real, one-time Cloudflare API
  call per hostname (or per wildcard), not automatic. Two mechanisms,
  by who owns the domain: **Custom Domains** (API-provisionable — a
  Terraform resource takes `hostname`+`service`+`zone_id` — but needs a
  zone *this operator* owns, e.g. `*.apps.<operator-domain>` as one
  wildcard Custom Domain covering every future Gateway/HTTPRoute with zero
  further calls) for the common case; **Cloudflare for SaaS /
  Custom Hostnames** (GA, bundled non-Enterprise; the tenant CNAMEs to a
  fallback origin and Cloudflare issues the cert) only for genuine
  bring-your-own-external-domain, which is overkill until actually needed.
  Plain Workers **Routes** don't fit — they front an existing non-Worker
  origin, which doesn't apply here. The user's other alternative — a
  second, separately-*deployed* Worker script dedicated to Gateway-routed
  traffic — would buy real blast-radius isolation from the control plane,
  but costs a categorically new capability (calling the Workers API to
  deploy/manage another script is a much bigger permission surface than
  anything built so far, which is entirely self-contained in one Worker +
  its own bindings); defer unless the isolation is actually needed. MVP:
  one wildcard Custom Domain under an operator-owned zone + Host-header
  dispatch as an internal dynamic worker. cert-manager mostly falls away
  regardless (TLS terminates at the edge either way).
  (https://developers.cloudflare.com/workers/configuration/routing/custom-domains/,
  https://developers.cloudflare.com/cloudflare-for-platforms/cloudflare-for-saas/)
- **Cluster DO backup/DR is mostly already solved — the real gap is
  narrower than "disaster recovery."** SQLite-backed Durable Objects (what
  `Cluster` already is) have native point-in-time recovery for the last 30
  days: `ctx.storage.getBookmarkForTime(timestamp)` →
  `ctx.storage.onNextSessionRestoreBookmark(bookmark)` → `ctx.abort()` to
  apply it, covering both SQL and KV storage, scoped per-DO-instance (a
  perfect match — there is exactly one `Cluster` DO). Writes are also
  already synchronously replicated to multiple nearby-datacenter replicas
  before being acknowledged. What this project would actually be adding on
  top is retention *beyond* 30 days and a portable/human-inspectable
  export, not baseline durability. Recommended first step, cheap: wire the
  native PITR API behind one admin endpoint
  (`POST /restore?to=<timestamp>`). Second, lower-priority step: a
  periodic snapshot to R2 needs no new query logic (the existing
  "current value per live key" read already does it, e.g.
  `SELECT name, value FROM kine WHERE id IN (SELECT MAX(id) FROM kine
  GROUP BY name) AND deleted=0`) and needs no revision-continuity
  handling on restore — replaying rows into a fresh DO gets fresh revision
  numbers, which every kine/client-go watcher already treats exactly like
  a big compaction event (410 Expired → relist). No `ctx.storage.setAlarm()`
  is used anywhere in `cluster.ts` today, so a periodic (non-manual)
  snapshot job needs a new wake source — a Cron Trigger is the natural
  fit. Not urgent: the schema/compaction semantics are still moving, and
  locking in an R2 export format now risks a redesign later.
  (https://developers.cloudflare.com/durable-objects/api/sqlite-storage-api/,
  https://developers.cloudflare.com/changelog/2025-04-07-sqlite-in-durable-objects-ga)
- **`k8s.io/kube-aggregator` is already pinned but doesn't fit; metrics-server
  doesn't need it anyway.** `go.mod` already replaces
  `k8s.io/kube-aggregator` with the same k3s-io fork as everything else
  (indirect dep, unused). Its real proxying
  (`handler_proxy.go`) dials a resolved backend address over real TLS —
  built for arbitrary, runtime-registered targets, which doesn't map onto
  this project's Service Bindings, fixed at deploy time in
  `wrangler.jsonc`. No workload can make itself a new routable aggregation
  target at runtime the way real `APIService` allows; this project's
  existing hand-rolled `groupRouter`/`forwardTo` (resolve group name → a
  small, fixed set of Service Bindings → stream-proxy) is the more honest
  fit than importing real kube-aggregator wholesale. **metrics-server
  doesn't need any of that resolved first**: build it exactly like
  `scheduler`/`controllers` — one more fixed-route DynamicWorker (e.g.
  `METRICS`) serving `metrics.k8s.io/v1beta1`, reusing
  `packages/node-tunnel`'s existing `/node/<name>/<kubelet-path>` proxy
  (today used for `pods/log`) to reach each kubelet's `/metrics/resource`.
  The one real mismatch: metrics-server polls every node on a fixed
  ~15s wall-clock interval, not reactively on writes — the same shape as
  the already-identified CronJob gap, needing a Cloudflare Cron Trigger
  (still absent from `wrangler.jsonc`) rather than a poke. HPA is a
  separate, bigger follow-on: `packages/controllers/controllers.go`'s 14
  wired controllers do not include `horizontalpodautoscaler` — it would
  need upstream's real `NewHorizontalController` added the same way the
  other 14 were, plus a metrics client pointed at `METRICS`.
- **ClusterUpgrade has three separable concerns; the Durable Object data
  question is the hard, novel one.** (1) Control-plane code: a plain
  `wrangler deploy` is effectively an instant full cutover for new
  requests by default; Cloudflare's gradual deployments can split traffic
  by percentage with optional version affinity if a slower rollout is
  wanted. This project's own architecture adds a wrinkle generic Workers
  docs don't cover: resident dynamic-worker instances (front `apiserver`,
  `scheduler`, `openapi`, etc.) can stay warm across a deploy, so an old
  front talking to a new group worker (or vice versa) over their internal
  RPC shape is a real skew window this project invented by splitting into
  per-group binaries — worth testing deliberately, not assumed away by
  Cloudflare's own version-skew tooling (which is about *which Worker
  version* handles a request, not this project's own inter-binary
  protocol). (2) **The Cluster DO's data is the hard part.** Current
  `wrangler.jsonc` migration tags (`new_sqlite_classes` for `Cluster`,
  `NodeTunnel`) are confirmed to only govern which DO *classes* exist —
  changing an existing class's code needs no migration tag at all, but
  Cloudflare's own docs say it's the project's responsibility that new
  code stays backwards-compatible with what's already stored; there is no
  `PRAGMA user_version`, so schema versioning is entirely userland (a
  tracking table, `blockConcurrencyWhile()` for a real migration pass).
  With exactly one `Cluster` DO holding the whole cluster's live kine log
  (unlike etcd's N-replica rolling upgrade), there is no "old version
  keeps serving while new version validates" option. Recommended: give
  every kine row a schema-version field from day one and migrate-on-read
  in `cluster.ts`, before there's real production data to be locked out
  of reading. (3) k3s agent/kubelet version skew is the well-trodden part
  — this project adds no skew gating beyond upstream's own installer, so
  it's no worse than real k8s's already-documented kubelet-to-apiserver
  skew policy; can wait.
  (https://developers.cloudflare.com/workers/configuration/versions-and-deployments/gradual-deployments/,
  https://developers.cloudflare.com/durable-objects/reference/durable-objects-migrations/)
- **Durable Object Facets are real (open beta, Apr 2026, Workers Paid,
  under Cloudflare's "Dynamic Workers" umbrella with this project's own
  Worker Loader binding) — and this project already built and verified a
  real design around them**, on `main`/`main-legacy-full-history`, not
  `feat/minimal-rewrite`. Mechanics confirmed both from current Cloudflare
  docs and this project's own historical design doc
  (`docs/multi-tenancy-and-hosting.md` on
  `origin/docs/multi-tenancy-and-roadmap`, dated July 2026): a DO loads a
  named child "facet" lazily (`ctx.facets.get(name, initCallback)`, the
  callback only runs if that facet hasn't started or has hibernated) with
  its **own isolated SQLite** the parent can't read, reached via a local
  RPC hop (not a network round-trip) — but **every facet shares the
  parent's 10 GB storage cap and is reachable only through the parent's
  single thread**. Facets buy isolation and lifecycle, not throughput or
  storage headroom; anything needing its own 10 GB or its own thread has
  to be a top-level DO. The old design used this correctly: the Cluster DO
  hosted only small, isolation-worthy facets (`ca-vault` for CA
  keys/node-password hashes, `events-log` for high-churn Event isolation),
  while the Namespace DO (one per namespace) was deliberately a
  **top-level DO, not a facet**, because namespace data needs real
  throughput headroom — directly relevant to the Cluster-DO-backup/DR note
  above, since splitting `ca-vault`/high-churn writes into facets shrinks
  what one DO instance has to hold without needing the full multi-tenant
  apparatus below. The storage layer also recorded a real correctness
  lesson worth reusing verbatim: after two more clever, abandoned attempts,
  it landed on **parent-first, no inference** — the parent's log row is
  always authoritative, trimmed only after the facet acknowledges receipt;
  never ask the facet first, because "the acknowledgement and the trim can
  both land in between." Decision 2026-09-14: not adopted yet. Go reaches
  storage only through the `STORAGE` binding's HTTP and WebSocket contract
  (`/list`, `/watch`, `/insert`, `/compact`, `/stats`), and the single-DO
  assumption is four `idFromName("default")` lines in
  `packages/control-plane-worker/src`, so a per-namespace DO is a
  cluster-store-internal change that can land later without touching Go.
  Trigger: `GET /stats` storage growth toward the 10 GB cap, or p99
  write/list latency under e2e load showing the one DO thread saturating;
  neither is near today (required set 15/15 in 104s on one DO).
- **Full multi-tenancy (a `Cluster` CRD + `clusterop` controller) was also
  built and verified end-to-end, separately from facets** — reusable
  independently, not a package deal. `k8flare.com/v1alpha1 Cluster` was a
  compiled-in type (no CRD codegen in that era), cluster-scoped in the
  storage keyspace; `clusterop` reconciled `Cluster` objects into full
  tenant control planes (allocates the DO tree name, initializes the token
  vault, publishes credentials as a Secret, maintains `/c/<id>` resolution,
  tears down via finalizer — Scheduler-first, "because Containers are
  wall-clock billed"). Routing was a `/c/<id>` path prefix (the Rancher
  `/k8s/clusters/<id>` precedent) backed by a `ClusterRegistry` DO
  (`id -> uid, state`; the UID indirection means a recreated cluster never
  reuses DO/facet names). DynamicWorker Loader IDs became per-cluster too
  (`apiserver:<doName>@sha`) — loading was tenant-scoped, not just
  storage. The whole seam was one indirection layer
  (`clusterenv.ts` retargeting `idFromName("default")` per-request), so
  every downstream module stayed single-cluster-shaped and unaware of
  multi-tenancy. Verified at the time: cross-cluster token rejection, data
  and watch isolation, rotation/revocation, and teardown + same-id
  recreation yielding a genuinely fresh cluster. No recorded regression or
  abandonment reason was found for this (unlike Pod-on-Containers, which
  has one) — reads as deliberate descoping for `feat/minimal-rewrite`'s
  "as little code of our own as possible" goal, not a technical wall, but
  that's an inference from absence of failure evidence, not a stated fact,
  so verify before assuming it's simply revivable as-is.
- **Is swapping the hardcoded `idFromName("default")` string alone safe
  multi-tenancy? No — it's the one indirection point that has to exist,
  but by itself it's unsafe.** Current code has grown to **five** such
  call sites, not four as stated above (`packages/control-plane-worker/
  src/loader.ts:5`, `apigroups.ts:11`, `customresources.ts:7`,
  `index.ts:34` and `:53` — corrects the count in the bullet above), all
  in the plain TS Worker, none in Go. That split isn't a style choice:
  `pkg/controllers/clusterop/bridge.go`'s own doc comment cites
  `docs/platform-verification.md` S2 for the reason — **a Loader-loaded
  dynamic worker can't be handed a `DurableObjectNamespace` binding at
  all, only plain values and `Fetcher`s survive the env clone** — so
  `packages/controllers`, `packages/apiserver*`, etc. can never call
  `idFromName` themselves; they only ever receive an already-resolved
  `STORAGE` Fetcher-shaped binding, exactly like the old `clusterop`
  controller (also a resident Go/wasm dynamic worker) having to proxy
  every DO-allocation/registry/vault write through an authenticated
  `/internal/clusters/*` HTTP bridge back into the TS layer instead of
  touching `env.CLUSTER` itself. On today's code specifically, three
  things make a bare id-swap unsafe:
  1. **Global secrets, not per-tenant ones.** `ADMIN_TOKEN` /
     `READONLY_TOKEN` / `JOIN_TOKEN` are Worker-wide secrets (absent from
     `wrangler.jsonc`'s `vars`, so `wrangler secret put`-managed — one
     value for the whole deployment), and the apiserver's bearertoken
     auth compares against them directly. Route different requests to
     different Cluster DOs today and every tenant still accepts the
     *same* admin/join token — a full cross-tenant auth bypass, not an
     isolation win. The old design's fix: a per-cluster **token vault
     living inside each tenant's own Cluster DO** (`clusterop.go`'s
     `EnsureVault`/`MintToken`/`RevokeToken`), reusing exactly the
     `/vault/...` kine-KV prefix this repo already uses today for node
     passwords (`checkNodePassword`, `index.ts:33`) — the storage-side
     isolation primitive already exists, it's just not yet used for
     admin/join tokens.
  2. **`NODE_TUNNEL.idFromName(nodeName)` is already parameterized, but
     only by node name.** Two tenants each naming a node `node1` collide
     on the identical NodeTunnel DO today, and the join wire format
     (`node:<name>:<password>`, `index.ts:26`) carries no cluster
     identifier at all — confirmed zero hits for `tenant`/`clusterId`
     anywhere in `packages/apiserver-supervisor`. This is a genuinely new
     gap the old design never had to close, since node-tunnel postdates
     it; closing it means a wire-format change (e.g.
     `node:<clusterId>:<name>:<password>`) before `authenticateNode` can
     even know which vault to check.
  3. **Name reuse would bleed data.** A raw `idFromName(tenantId)` where
     `tenantId` is a user-chosen string means deleting and recreating a
     tenant of the same name resolves to the *same* DO instance,
     inheriting its old SQLite state. The old code's defense was minting
     the actual DO-name suffix from the Kubernetes object's own immutable
     UID (`doNameFor`: `fmt.Sprintf("%s@%s", cl.Name, cl.UID)`,
     `clusterop.go`), allocated exactly once and persisted in
     `.Status.DoName`, with the external-facing `/c/<id>` route keyed
     only on the UID half (`uidFromDoName`) — "switching the id" has to
     mean switching to an operator-allocated, never-reused compound key,
     not a bare tenant-supplied slug.
  What genuinely *is* close to free: Cloudflare's own DO namespace needs
  no provisioning step — `idFromName` on a new string just lazily
  allocates a fresh, fully SQLite-isolated instance on first
  `.get().fetch()`, no platform-side "create cluster" call required.
  Every hard part above is this project's own application-level
  assumption, not a platform limit. Sequencing worth reusing verbatim
  from `clusterop.reconcileActive`: write the `/c/<id>` registry entry
  *before* minting the vault (a routing entry with no credentials yet
  safely 401s; credentials with no route safely 404s forever — never
  serve a half-provisioned tenant), and on rotation, revoke a superseded
  token only after its replacement is fully distributed.
- **What this means for "zero to scale" concretely**: DynamicWorkers and
  DOs are already zero-cost-until-addressed by Cloudflare's own runtime
  (no new design needed there); facets add a third, more fine-grained
  zero-to-one primitive *within* one DO. A user-facing CRD declaring
  "run workload X on DynamicWorker/DO-Facet compute, 0↔N" still needs a
  real controller mapping that onto Cloudflare primitives — the same
  *provisioning* category as Pod-on-Containers' `VirtualNode`/
  `PodContainer` DOs or the LoadBalancer controller's `Provisioner`, not
  the portable-controller-core pattern (DynamicWorkers/Facets are
  Cloudflare-only primitives with no vanilla-k8s equivalent, so — like
  Pod-on-Containers — there is no matching plain `cmd/` to design here
  either). Shape sketch: a supervisor DO (mirroring `VirtualNode`'s
  existing ~10s Lease/reconcile-alarm pattern) lazily creating one facet
  per workload via `ctx.facets.get(name, initCallback)`, trading
  Pod-on-Containers' ~100x-slower/heavier container cold start for a much
  cheaper isolate — a better fit for small, short-lived, event-driven
  workloads than anything needing a full container.
  (https://blog.cloudflare.com/durable-object-facets-dynamic-workers/,
  https://developers.cloudflare.com/dynamic-workers/usage/durable-object-facets/)
- **Pod-to-pod networking: Cloudflare Mesh is a real WireGuard/Tailscale
  replacement candidate for flannel's transport — unlike cluster DNS
  below, this one looks genuinely promising.** `packages/agent` doesn't
  override flannel's backend at all, so it inherits stock k3s's default,
  **`vxlan`** (not WireGuard); more importantly, the only milestone
  reached so far (see Goal) is **one** OrbStack VM — multi-node pod
  networking is entirely unexercised here today, not just undocumented.
  The decisive fact, confirmed from current docs: Mesh enrollment installs
  a **VPN profile** via "a headless version of the Cloudflare One Client,"
  giving the node real OS-level TCP/UDP/ICMP reachability to other Mesh
  IPs for *any local process* — architecturally the same shape as
  WireGuard/Tailscale, and a materially different (better) fact than the
  Workers-VPC-from-a-DO open question above, since this doesn't route
  through Workers/DOs at all. flannel's **`host-gw`** backend adds no
  encapsulation of its own — just a kernel route to each peer's pod CIDR
  via that peer's node IP — so pointing flannel at `host-gw` with each
  node's *Mesh IP* as its node-internal IP should work with no k3s/flannel
  code changes: Mesh supplies the encryption and transport, `host-gw`
  supplies the pod-CIDR routing. Not verified end-to-end and no existing
  Cloudflare precedent exists for Mesh-as-CNI-transport specifically (all
  current framing is dev-agent/SSH/DB-access, not Kubernetes networking) —
  this is new territory. Sequence after single-node work stabilizes
  (multi-node is untested regardless of transport). Cheapest validation:
  enroll two OrbStack VMs in Mesh, confirm plain `ping`/`iperf3` between
  their Mesh IPs as ordinary OS traffic (no Workers/DO involved at all),
  then try `--flannel-backend=host-gw` with those Mesh IPs as the node
  IPs. (https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/get-started/,
  https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/client-devices/)
- **Cluster DNS: no meaningful Cloudflare-native substitute for CoreDNS
  exists today — don't stretch for one.** The real constraint: kubelet
  points every pod's `/etc/resolv.conf` at the cluster-dns ClusterIP, and
  whatever answers it must be a real UDP/TCP **:53** listener speaking
  plain DNS wire protocol (DNS is UDP-first; TCP is only the &gt;512-byte
  fallback) — a fundamentally different transport shape from a Worker's
  HTTP request/response model. Checked and ruled out: Cloudflare's newest
  Spectrum-routed-to-Worker-socket feature (private beta, Aug 2026) is
  **TCP only** — the announcement itself names UDP as future work, so even
  a mature version misses the half DNS actually needs; plain (GA) Spectrum
  is a dumb byte-forwarder to a real origin, so routing it at a hand-run
  DNS server doesn't eliminate a workload, it just relabels one. Cloudflare's
  DNS product has no dynamic, API-driven per-query answering mechanism —
  only static zone records (useless for Service/EndpointSlice-derived
  answers that change every second) and Cloudflare One's Gateway resolver
  policies (a client's own *outbound* DNS routing/filtering, a different
  problem entirely, not a way to answer *inbound* in-cluster queries).
  Realistic path: real upstream CoreDNS as an ordinary Deployment+Service
  once Services/kube-proxy exist (the same gating dependency already
  flagged for Ingress and admission control) — a small, well-understood,
  upstream-maintained fixed cost, consistent with this project's own
  reuse-upstream philosophy, not overhead worth hand-rolling a replacement
  for. `node-local-dns`-style caching solves a multi-node conntrack/latency
  problem this project doesn't have yet.
  (https://blog.cloudflare.com/grpc-workers/,
  https://developers.cloudflare.com/cloudflare-one/traffic-policies/resolver-policies/)
