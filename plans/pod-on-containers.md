# Pod on Containers: a Durable Object as the kubelet

Replaces the NodeVM backend (a k3s agent inside a microVM per Pod) with a
Durable Object that runs the Pod's image directly through `ctx.container`
(`scheduling_policy: "durable_object"`). No k3s agent, containerd, flannel
or kube-proxy is involved. The model is AWS Fargate: an opted-in Pod gets
its own microVM, and a Pod the platform cannot run stays `Pending` with a
Warning Event that says why.

Evidence for every platform behaviour below is in
`experiments/do-kubelet-spike/FINDINGS.md` (measured 2026-09-30).

## Decisions

- **Images are not copied.** A Pod may use only images the cluster
  deployment declares under `containers[].images` in `wrangler.jsonc`,
  started as `ctx.container.images[<name>]`, plus
  `registry.cloudflare.com/<account>/<repo>@sha256:<digest>` and
  `cloudflare/debian-trixie`, which the runtime accepts directly.
  Anything else (Docker Hub, a tag without a digest) is not pulled: the
  Pod stays `Pending` with a Warning Event.
- **Resources are used as written.** The Pod's CPU and memory (requests,
  or limits where larger) become a custom instance
  `{vcpu, memoryMib, diskMb}` directly, with no rounding. Custom instances
  allow 1–4 vCPU, at least 3 GiB per vCPU, at most 12 GiB and 20 GB disk
  (limits page, 2026-09-30). Below 1 vCPU no custom shape exists, so the
  smallest named type that holds the Pod is used: `lite` (1/16 vCPU,
  256 MiB, 2 GB), `basic` (1/4, 1 GiB, 4 GB), `standard-1` (1/2, 4 GiB,
  8 GB). A Pod that fits neither (for example 2 vCPU with 2 GiB, or
  more than 4 vCPU) stays `Pending` with a Warning Event naming the
  violated constraint.
- **No files are placed in the container.** Placing volume files with a
  container snapshot works (FINDINGS.md round 3) but costs about 10 s per
  start, loses anything under tmpfs (`/run`, so the ServiceAccount token
  path), and needs `/bin/sh` in the image, so it is not used. Instead:
  - ConfigMap and Secret values reach the Pod only as environment
    variables (`env.valueFrom`, `envFrom`); a Pod with ConfigMap, Secret,
    downwardAPI, projected or emptyDir volumes stays `Pending` with a
    Warning Event.
  - The API server is reached as `https://kubernetes.default.svc`
    through the HTTPS interceptor. `KUBERNETES_SERVICE_HOST` is set to
    `kubernetes.default.svc` (interception by IP resets the
    connection), and the interceptor attaches the calling Pod's
    ServiceAccount identity itself, so the container holds no token.
    The container trusts the interception CA through
    `SSL_CERT_FILE` / `NODE_EXTRA_CA_CERTS` pointing at
    `/etc/cloudflare/certs/cloudflare-containers-ca.crt`, a file the
    platform provides.
  - `rest.InClusterConfig` (client-go) reads the token file and fails
    without it (`client-go/rest/config.go`), so libraries that use it
    do not work unmodified. This is a documented limitation of the
    Containers compute class.
- **The image's ENTRYPOINT/CMD come from the build.** `start()` takes one
  `entrypoint` list, while a Pod may set only `args` (which replaces CMD
  and keeps ENTRYPOINT). The deploy step
  records each declared image's ENTRYPOINT, CMD, WorkingDir, User and
  Env (`docker image inspect`) next to the Worker, and the DO reads them
  from there.
- **Starts retry with exponential backoff.** Transient start failures
  (`There is no container instance…`, `…temporarily unavailable`, and a
  snapshot restore when snapshots are used) are retried on DO alarms
  with exponential backoff, and the Pod gets an Event after the first
  failure.

## Shape

1. **Opt-in.** Unchanged: `k8flare.com/compute: containers` on the Pod or
   its Namespace. Admission sets `schedulerName: k8flare-containers` and
   the toleration for the virtual Node's taint.
2. **Placement is ours, not kube-scheduler's**, exactly like the Fargate
   scheduler. A `k8flare-containers` pass takes unbound Pods with that
   scheduler name, validates image, shape, and supported fields, and
   either binds them to the virtual Node `cloudflare` through the Binding
   subresource or records a Warning Event (`FailedScheduling`, reason in
   the message) and leaves them `Pending`. It re-evaluates when the Pod
   or the deployment's declared images change.
3. **Virtual Node `cloudflare`**, tainted `k8flare.com/pod-on-containers`
   `NoSchedule` and labelled as virtual. The node lifecycle controller
   leaves it out of lease-based NotReady handling, so nothing renews a
   lease on a timer.
4. **One `PodKubelet` DO per Pod** (named by Pod UID), woken by the Pod
   write observer when a Pod is bound to `cloudflare` or deleted. It:
   - resolves `env` / `envFrom` from ConfigMaps and Secrets and calls
     `start({ image, instance, entrypoint, env, enableInternet })` with
     the Pod's `command` + `args` as the entrypoint;
   - raises `setInactivityTimeout` so a quiet Pod is not stopped;
   - awaits `monitor()`; exit maps to `containerStatuses` (exit code,
     reason) and restarts follow `restartPolicy` with the kubelet's
     CrashLoopBackOff (10 s doubling to 5 min) on DO alarms;
   - runs probes: `httpGet` through `getTcpPort`, `exec` through `exec`;
   - writes Pod status (IP, conditions, ready) through the API;
   - on deletion sends SIGTERM, waits the grace period, then `destroy()`.
5. **Networking.**
   - Pod IPs are virtual (every container is `10.0.0.1` inside), taken
     from the virtual Node's `podCIDR`.
   - Egress: `interceptAllOutboundHttp` to a router entrypoint. ClusterIPs
     and Pod IPs of `cloudflare` Pods go to the target Pod's DO via
     `getTcpPort`; other addresses pass through (and NetworkPolicy egress
     is decided here).
   - Ingress, LoadBalancer Services and `pods/proxy` route straight to the
     Pod's DO instead of through the node tunnel.
6. **kubectl.** `exec` / `attach` use `exec(argv, {pty, stdin})`;
   `port-forward` uses `getTcpPort`. The API server routes these for the
   `cloudflare` Node to the Pod's DO instead of the node tunnel.
7. **Removal of the NodeVM backend** (`NodeVMSmall/Medium/Large`,
   `CFContainersScheduler`, `images/node`, `@cloudflare/containers`, the
   tier annotation) is the last, separate change, with a `deleted_classes`
   migration.

## First version rejects (Pending + Event)

More than one app container, init containers, `hostNetwork`, `hostPID`,
`hostPath`, persistent volumes, `privileged`, and egress to Services
over anything but HTTP/HTTPS (no `interceptOutboundTcp` yet).

## Open questions to settle by experiment before building on them

- **Logs.** stdout/stderr are sockets owned by the platform. Where they
  arrive (Workers Logs?) decides how `kubectl logs` is served. The spike
  also showed images that symlink log files to `/dev/stdout` (stock
  nginx) exit 1; with no image rewriting, their Dockerfiles must not do
  that. That is the image author's to fix, and the Event must say so.
- **Idle survival.** `setInactivityTimeout` tops out at 6 h. Whether a
  container with that timeout survives while no request reaches its DO
  is being measured; the DO re-arms an alarm well inside 6 h either way.
- **Leak tracking.** The platform's instance counts show 0 for running
  `durable_object` containers, so the cluster keeps its own record of
  which Pod DOs hold a container and reconciles it.

Settled by the spike (FINDINGS.md rounds 2 and 3): custom shapes accept
fractional vCPU (1.01, 1.3, 1.5); cluster names resolve and reach the
interceptor once `interceptAllOutboundHttp` is on, so HTTP needs no
cluster DNS; HTTPS to `kubernetes.default.svc` is intercepted with a
platform CA the container must trust (`/etc/cloudflare/certs/
cloudflare-containers-ca.crt`); `snapshotDirectory` and
`directorySnapshots` do not work yet; raw TCP into a container works
through `getTcpPort().connect()`.

## Progress: steps 3 and 4 (PodKubelet, 2026-10-01)

Code: `packages/control-plane-worker/src/podkubelet/` (`spec.ts` pure
rules, `kubelet.ts` the DO, `ledger.ts`, `api.ts`, `egress.ts`, `wake.ts`,
`images.generated.ts`), tests in `packages/control-plane-worker/test/`.

- **Wake path.** The Cluster DO already routes Pod writes to the
  `k8flare-containers` queue when the Pod opts in; it now also does so
  for `spec.nodeName: cloudflare` and `schedulerName: k8flare-containers`.
  `consumeContainers` then calls `wakePodKubelets`, which resolves each
  changed `/registry/pods/<ns>/<name>` to Pod UIDs (the live Pod if bound
  to `cloudflare`, plus the `PodLedger` entries for that name so a
  force-deleted Pod still reaches its DO) and calls `reconcile()` on the
  `PodKubelet` DO named by UID. The DO talks to the API as component
  `podkubelet` (`system:k8flare:podkubelet`, privileged), never
  `ADMIN_TOKEN`.
- **Contract consumed.** `containers.k8flare.com/image` (`images/<name>`
  → `ctx.container.images[name]`, otherwise a literal reference) and
  `containers.k8flare.com/instance` (named type or compact JSON). Images
  declared under `containers[].images` are exported to the apiserver
  worker as the `CONTAINERS_IMAGES` env var (JSON list of names) from
  `loader.ts`, derived from `images.generated.ts`; `make images` (part of
  `make gen`) runs `scripts/genimages.mjs`, which builds each declared
  Dockerfile and records ENTRYPOINT/CMD/WorkingDir/User/Env. A Pod on an
  image with no recorded metadata must set `command`
  (`CreateContainerConfigError` otherwise).
- **Pod IPs** come from the virtual Node's `spec.podCIDR`, falling back
  to `10.42.255.0/24` when the Node has none; `.0`, `.1` (host IP when the
  Node has no InternalIP) and `.255` are skipped. `PodLedger` (one per
  cluster) allocates them and records which DOs hold a container, since
  the platform's instance counts read 0 for `durable_object` containers.
- **Keep-alive.** Round 4 of the spike showed a container with
  `setInactivityTimeout(6 h)` stopped after about 25 minutes without a
  request reaching its DO, so the DO does not rely on the 6 h figure:
  `KEEPALIVE_INTERVAL_MS` (60 s) is a DO alarm that checks `running` and
  re-arms the inactivity timeout. **The safe interval still needs
  measuring**; 60 s is a starting point, not a measured value.
- **Egress.** `interceptAllOutboundHttp` and
  `interceptOutboundHttps("kubernetes.default.svc:443")` are given the
  Pod's own DO stub as the Fetcher, so the DO's `fetch()` knows which Pod
  is calling. `routeEgress` sends API traffic to the apiserver with a
  TokenRequest-minted token bound to the Pod (1 h, refreshed 5 min before
  expiry) and passes everything else to `fetch`; `clusterTarget` is the
  seam for ClusterIP / Pod IP routing to other Pod DOs. **Unverified on
  the platform:** whether a DO stub is accepted where the docs say
  "Worker entrypoint or service binding"; if not, swap in a
  `WorkerEntrypoint` and carry the Pod UID another way.
- **Not exercised locally.** Whether `wrangler dev` runs
  `scheduling_policy: durable_object` containers is unverified (the dev
  server was not started for this step; it needs `make wasm` assets), so
  the DO is tested with the container API, storage, alarms, ledger and
  apiserver faked (`test/kubelet.test.ts`); the platform-facing calls
  remain to be exercised on a real deployment (step 3's check column).
- **Deviations from the design text above:** exit-code parsing of
  `monitor()` rejections is by regex over the message (the exact text is
  not recorded in FINDINGS); kubelet `monitor()` errors matching the
  platform's validation/capacity phrases are treated as start failures
  (terminal or transient) rather than exits.

Remaining after this step: ClusterIP / Pod IP routing between Pod DOs,
`exec` / `attach` / `port-forward` / `logs` routing to the DO, Service
environment variables, a reconciler over the ledger for leaked
containers, README.

## Progress: steps 5 and 6 (routing and kubectl streams, 2026-10-01)

Code: `podkubelet/cluster.ts` (pure resolution), `protocol.ts` (pure
framing), `dial.ts`, `streams.ts`, the RPC methods on `PodKubelet`;
tests `cluster.test.ts`, `protocol.test.ts`, `streams.test.ts` and the
DO cases in `kubelet.test.ts`. Nothing ran on the platform: the DO is
tested with the container faked and the front with the DO faked.

- **Egress between Pods.** `routeEgress` asks the DO's `clusterTarget`
  before passing through. `resolveClusterTarget` takes the intercepted
  host: an IPv4 literal is looked up in the `PodLedger` (a `cloudflare`
  Pod IP) and then as `spec.clusterIP` through the Service field
  selector (`pkg/registry/core/service/strategy.go` exposes it); a name
  is read as `svc`, `svc.ns`, `svc.ns.svc` or `svc.ns.svc.cluster.local`
  (bare names use the calling Pod's namespace; every name reaches the
  interceptor, FINDINGS #15). The Service port is matched by number, the
  EndpointSlices labelled `kubernetes.io/service-name` give the target
  port by port name, and a ready endpoint with `nodeName: cloudflare` is
  forwarded to `podKubeletStub(uid).ingress(port, request)`, which calls
  `getTcpPort(port).fetch`; a Pod reaching its own Service is served
  locally rather than through a stub to itself. Headless Services match
  the requested port against the endpoint ports directly. Ready
  endpoints on other nodes answer 502 naming those nodes; no ready
  endpoint is 503; ExternalName is 502; a host that is neither a Pod IP
  nor a Service passes through to `fetch`. Cost: a `svc.ns`-shaped
  internet host (two labels) costs one Service GET per request.
- **Ingress to a Pod.** `/dial/cloudflare/<ip>/<port>/<path>` on the
  `NodeTunnels` entrypoint, which is where `pods/proxy`
  (`apiserver-core/proxydial.go`) and the edge (`edgehost/proxy.go`)
  send Service and Pod traffic, looks the IP up in the ledger and calls
  `ingress`. `X-Dial-TLS` is refused with 502 (no TLS into the
  container through `getTcpPort`). So LoadBalancer and Ingress to a
  `cloudflare` Pod go through the same seam, unexercised (step 7's
  check still stands).
- **kubectl streams.** `LocateStream` already names the node, so the
  front hands `node: cloudflare` to `servePodStream` instead of the
  tunnel. The front terminates the websocket and speaks the channel
  protocol itself, as `apiserver/pkg/util/proxy/websocket.go` does:
  channels 0-4, an empty first frame on stdout/stderr/error, `[255, ch]`
  half-closes stdin under `v5.channel.k8s.io`, resize frames carry
  client-go `TerminalSize` JSON, and the exit is a `metav1.Status` on
  channel 3 (`NonZeroExitCode`, cause `ExitCode`, the translator's
  message text). Binary `""`/`channel.k8s.io`/`v4` and the base64
  variants are also accepted; anything else closes with 1002.
  - `exec`: `PodKubelet.exec(argv, {stdin, stdout, stderr, tty, cols,
    rows, control})` returns `{stdout, stderr, status}` streams; `stdin`
    and `control` are streams the front writes into. The `control`
    stream carries resize JSON and its end means the client went away,
    at which point a still-running process gets SIGTERM.
  - `port-forward`: the kubelet's websocket protocol
    (`cri-streaming/pkg/streaming/portforward/websocket.go`): a
    data/error channel pair per `port`, both opened with the
    little-endian port; `PodKubelet.connectPort(port, input)` pipes the
    client bytes into `getTcpPort(port).connect()` and returns the
    socket's readable. kubectl itself speaks SPDY or the
    `SPDY/3.1+portforward.k8s.io` tunnel, neither of which is served
    here (readme-gaps: SPDY cannot be served), so this path serves
    websocket clients only.
  - `attach`: a Status on the error channel saying attach is not
    available (the container's stdio belongs to the platform) and a
    clean close, rather than an exec in disguise.
  - `logs`: `/node/cloudflare/containerLogs/...` returns 400 with the
    reason, which `kubectl logs` prints as `Error from server
    (BadRequest)`; the websocket log path closes with 1008 and the same
    text. FINDINGS #19: stdout is not reachable from the DO and where
    it lands is not established.
- **Unverified on the platform:** `Request`, `ReadableStream` and
  `WritableStream` crossing Durable Object RPC (`ingress`, `exec`,
  `connectPort`) and the lifetime of the socket behind the returned
  readable; the field selector `spec.clusterIP` against this apiserver;
  that a stub to the Pod's own DO is still the interceptor's Fetcher
  (step 3's open question). An e2e (`cloudflare` Pod wgets a Service
  backed by another `cloudflare` Pod, `kubectl exec` through the
  deployment) is the check for both steps and needs a real account.

Remaining: logs (needs a platform answer for container stdout), attach,
SPDY-tunnelled port-forward for kubectl, endpoints on real nodes
(a tunnel hop from the interceptor), Service environment variables, a
reconciler over the ledger for leaked containers, step 7's e2e through
the edge hostname, README, NodeVM removal (step 8).

## Steps and how each is checked

| # | Step | Check |
|---|---|---|
| 0 | Settle the open questions in the spike Worker | FINDINGS.md updated with measured answers |
| 1 | Instance shape and image resolution as pure Go with tests | `go test` table: resources → custom / named / rejection with reason; image → declared / digest / rejected |
| 2 | `k8flare-containers` placement pass + virtual Node + Events | host test: an unfit Pod stays Pending with the Event; a fit one is bound |
| 3 | `PodKubelet` DO: start, monitor, status, restart backoff, delete | local `wrangler dev` against a declared image; Pod reaches Running, a crash shows restarts, delete removes the container |
| 4 | Snapshot restore with exponential backoff | unit test of the backoff schedule; forced failure retried on alarms |
| 5 | Egress router and ClusterIP → Pod DO | e2e: a `cloudflare` Pod `wget`s a Service backed by another `cloudflare` Pod |
| 6 | exec / attach / port-forward routing | e2e with kubectl against the deployment |
| 7 | Ingress / LoadBalancer to Pod DOs | e2e through the edge hostname |
| 8 | README section; remove NodeVM backend | README matches behaviour; `make sizes`, full CI green |

## Progress

Steps 1 and 2 are done (`packages/containers`, the scheduler worker,
`packages/admission/computeclass.go`, `packages/workloads/nodehealth.go`).
The contract the `PodKubelet` DO consumes:

- Admission (`k8flare.com/compute: containers` on the Pod or its
  Namespace) sets `spec.schedulerName: k8flare-containers`, tolerates
  `k8flare.com/pod-on-containers=true:NoSchedule`, and sets
  `automountServiceAccountToken: false` unless the Pod set it. The
  ServiceAccount admission honours that field exactly as upstream
  `shouldAutomount` does (`plugin/pkg/admission/serviceaccount/admission.go`),
  so no `kube-api-access` volume is added and the DO has nothing to skip.
  The NodeVM nodeSelector, hostname pin and tier annotation are no longer
  written.
- The placement pass runs inside the scheduler worker after every
  kube-scheduler pump (the scheduler queue already wakes on every unbound
  Pod write). It lists Pods by `spec.schedulerName=k8flare-containers`,
  validates, and binds through `pods/binding` with the annotations on the
  Binding (the binding REST merges them into the Pod in the same write):
  - `containers.k8flare.com/instance`: `lite`, `basic`, `standard-1`, or
    `{"vcpu":1.5,"memoryMib":4608,"diskMb":8000}` for 1–4 vCPU. CPU and
    memory are the max over the container's request and limit;
    `ephemeral-storage` becomes `diskMb` (floored at 2000, at most
    20000). A Pod with no requests or limits gets `lite`. A Pod at or
    above 1 vCPU is never rounded: memory below 3072 MiB per vCPU, above
    12288 MiB, or more than 4 vCPU is rejected naming the constraint.
  - `containers.k8flare.com/image`: `images/<name>` for a name listed in
    the `CONTAINERS_IMAGES` var (a JSON array of the names declared under
    `containers[].images` in `wrangler.jsonc`, passed to the scheduler
    worker's env), otherwise the literal `cloudflare/debian-trixie` or
    `registry.cloudflare.com/<account>/<repo>@sha256:<64 hex>`.
  - Rejected Pods stay Pending with `PodScheduled=False`, reason
    `Unschedulable`, and one Warning Event `FailedScheduling` per distinct
    message (the count is bumped on repeats). Rejections in v1: more than
    one container, init or ephemeral containers, hostNetwork/hostPID/
    hostIPC, privileged, any volume (including projected), an image that
    is neither declared nor digest-pinned, a shape outside the limits.
- Virtual Node `cloudflare`: labels `type=virtual-kubelet`,
  `kubernetes.io/role=agent`, `k8flare.com/compute=containers`,
  `kubernetes.io/hostname=cloudflare`, `kubernetes.io/os=linux`,
  `kubernetes.io/arch=amd64`; taint `k8flare.com/pod-on-containers=true:
  NoSchedule`; capacity 4 CPU / 12Gi / 20G ephemeral / 110 Pods; Ready
  condition set once at creation; `status.addresses` holds only
  `Hostname=cloudflare` on purpose, because the `kubernetes` EndpointSlice
  publishes every Ready node's InternalIP on 6443
  (`apiserver-core/k8sep.go`). `spec.podCIDR` is assigned by the
  apiserver's node CIDR allocator (`apiserver-core/nodecidr.go`, a /24
  from the cluster CIDR); the DO reads it from the Node. The node health
  check skips nodes labelled `type=virtual-kubelet`, so the missing lease
  never marks it unreachable.
- kube-scheduler ignores these Pods: `Schedule` queues only Pods whose
  `schedulerName` is `default-scheduler`.

Not yet done: the pass re-evaluates a rejected Pod only when the scheduler
queue is woken again (any unbound Pod write), not when `CONTAINERS_IMAGES`
changes; the containers queue still wakes the NodeVM `CFContainersScheduler`,
which finds nothing to do until step 8 removes it.
