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
