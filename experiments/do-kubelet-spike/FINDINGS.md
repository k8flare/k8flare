# DO-as-kubelet spike (2026-09-30)

Question: can a Durable Object act as the kubelet for one Pod, running the
Pod's image with `ctx.container` (`scheduling_policy: "durable_object"`),
with no k3s agent, containerd, flannel or kube-proxy anywhere?

Measured against a real deployment in the KOOFFICE account
(`k8flare-do-kubelet-spike`, deleted afterwards), workers-types
5.20260930.2, wrangler 4.144.0. Timings are from `start()` to the first
successful `getTcpPort(port).fetch()`, measured inside the DO.

| # | Check | Result |
|---|---|---|
| 1 | `start({image: "docker.io/...:tag"})` | Rejected: `Image reference must be digest-pinned` |
| 1b | `start({image: "docker.io/...@sha256:..."})` | Fails: `internal error`. The docs agree: the `durable_object` policy pulls only `registry.cloudflare.com/<account>/...@sha256:...` and `cloudflare/debian-trixie`. The default policy can pull Docker Hub / ECR / Artifact Registry, but fixes the image per application at deploy time. |
| 2 | `cloudflare/debian-trixie` cold start | 614 ms, 615 ms |
| 2b | Account-registry image (nginx) cold start | 532 ms, 744 ms |
| 2c | Stock `nginx` image with its own ENTRYPOINT/CMD | Exits 1, every time. PID 1's fd 1/2 are sockets (`socket:[39]`), so `open("/dev/stderr")` fails, and nginx's `error.log -> /dev/stderr` symlink cannot be opened. Wrapping in `sh -c '... > /tmp/log 2>&1'` makes it work. Any image that symlinks logs to `/dev/stdout` breaks the same way. |
| 3 | Custom `instance: {vcpu, memoryMib, diskMb}` | Minimum 1 vCPU and 3072 MiB per vCPU. Smaller Pods must use named types (`lite` reports 8 CPUs, 458 MiB). |
| 4 | `exec(argv)` | 16–43 ms. `pty: true` gives `/dev/pts/0`, 24x80. |
| 5 | `interceptOutboundHttp("10.43.99.1:80", fetcher)` then `wget` in the container | Reaches the Worker entrypoint, 43 ms. |
| 5b | `interceptOutboundTcp` | `is not a function` in production today, despite being in the types. |
| 6 | Pod-to-pod: container A → intercepted virtual IP → Worker → DO B → `getTcpPort(80)` | Works, 31 ms round trip. |
| 7 | `snapshotContainer` (50 MB written) | 11.0 s to take, 52 MB. |
| 7b | Restore into the same DO | 929 ms, file contents intact. |
| 7c | Restore into a fresh DO (fork) | 613 ms, contents intact. Two earlier attempts right after taking the snapshot failed (one exited cleanly with no entrypoint, one `connection temporarily unavailable`); cause not isolated. |
| 7d | Snapshot does not keep the entrypoint | Restore without `entrypoint` exits immediately. Pass it on every start. |
| 8 | `signal(15)` to a PID 1 with no handler | Ignored, as Linux does for PID 1. `signal(9)` is seen by `monitor()` as exit 137 within about 1 s. |
| 9 | Every container's own address | `10.0.0.1/24` on `cfeth0`. Pod IPs must be virtual. |
| 10 | Idle containers | Stopped after a few minutes without traffic; set `setInactivityTimeout`. |

## Consequences for the design

- Pod images must be copied into the account registry before the Pod
  starts (an image-import step at admission or scheduling, resolving the
  tag to a digest).
- The copy step is also where a tiny static init can be added as a layer
  and made the entrypoint: it gives the workload real pipes for
  stdout/stderr (fixing 2c and providing `kubectl logs`), forwards
  SIGTERM (fixing 8), and could host several containers of one Pod in
  one microVM.
- Service ClusterIPs and Pod IPs are virtual addresses handled by
  outbound interception. Only HTTP interception works today; raw TCP
  (databases, Redis) waits on `interceptOutboundTcp`. Whether gRPC over
  HTTP/2 passes through the HTTP interceptor is not tested.
- Resource requests map to named instance types; custom sizes start at
  1 vCPU / 3 GiB.

## Round 2: open questions from plans/pod-on-containers.md

Same account and tooling, redeployed 2026-09-30 ~14:45 UTC.

| # | Check | Result |
|---|---|---|
| 11 | Custom shapes | `vcpu` 1.5, 1.3 and 1.01 all accepted (1.01 vCPU / 3103 MiB booted). Rejections: `memoryMib` < 3072 per vCPU, `diskMb` < 2000, memory > 12288 (`Container exceeds account limits`). Validation errors arrive through `monitor()`, not from `start()`, so they must be checked before starting. |
| 12 | `setInactivityTimeout` | Maximum 6 h (`The maximum amount of time that a container can stay disconnected from a Durable Object is 6 hours`). A long-lived Pod needs the DO to reconnect within 6 h (an alarm). |
| 13 | Raw TCP into a container port | `getTcpPort(7000).connect()` round trip 5 ms. Enough for `port-forward` and TCP Services on the receiving side. |
| 14 | `interceptOutboundTcp` | Still `undefined`. Egress raw TCP is not possible. |
| 15 | DNS | `/etc/resolv.conf` is `nameserver 1.1.1.1`, `/etc/hosts` is read-only. With `interceptAllOutboundHttp` active, any name (`my-svc.default.svc.cluster.local`) resolves to `fd00::119:1` and the HTTP request reaches the interceptor with the URL and Host intact. Cluster DNS is not needed for HTTP. |
| 16 | `interceptAllOutboundHttp` pass-through | Plain HTTP to the internet reaches the interceptor and can be re-fetched (200). HTTPS to the internet is not intercepted and goes out directly (200). |
| 17 | `snapshotDirectory` | `is not a function` in production. |
| 18 | `directorySnapshots: [{mountPoint}]` (empty mount) | The container never starts (`The container has not been started`). ConfigMap / Secret / emptyDir volumes have no platform mechanism yet. |
| 19 | Container stdout/stderr | Not in `wrangler tail`. The observability query API returned 401 for both the wrangler and cf OAuth tokens, so where the lines land is not established. |
| 20 | `interceptOutboundHttps("kubernetes.default.svc:443")` | The platform terminates TLS with a certificate from its own CA, which it places in every container at `/etc/cloudflare/certs/cloudflare-containers-ca.crt` (the image has no `/etc/ssl` of its own). Without trusting it: `SELF_SIGNED_CERT_IN_CHAIN`. With `NODE_EXTRA_CA_CERTS` pointing at it: 200 at the interceptor, URL and Host intact. By IP (`10.43.0.1:443`) the connection is reset. |
| 21 | In-cluster API access | Reachable by name through the HTTPS interceptor, and the interceptor knows which Pod is calling, so it can attach that Pod's ServiceAccount identity itself. What is missing is the token and CA *files* that `rest.InClusterConfig` reads, since nothing can be placed in the filesystem before the entrypoint (17, 18). |
| 22 | Dashboard / `wrangler containers list` | Show `LIVE INSTANCES 0` while a `durable_object` container is running and serving (uptime 687 s). The platform's own counts cannot be used to find running or leaked Pods. |
| 23 | Platform-injected files | None besides the interception CA (present only once HTTPS interception is set up). No helper binary exists in the container, so writing files needs a program in the Pod's own image. |

## Round 3: placing files with a container snapshot

Flow on `cloudflare/debian-trixie`: start with `sh -c 'exec sleep infinity'`,
write files with `exec`, `snapshotContainer`, `destroy`, start from the
snapshot with the Pod's own entrypoint.

| Step | Time |
|---|---|
| Boot to first successful `exec` | 1.95 s |
| Write two files | 0.28 s |
| `snapshotContainer` | 6.5–6.7 s, 2,680 bytes (the snapshot holds only the changes to the image) |
| Restore to first successful `exec` | 1.34 s |

- Files on the image's root filesystem (`/etc/k8s/key`) survive the
  snapshot with their contents.
- Files under tmpfs mounts do not: `/run` (and so `/var/run`, which links
  to it), `/dev`, `/dev/shm`. `/var/run/secrets/kubernetes.io/serviceaccount/token`
  is gone after restore. The first restore attempt "failed" with exit 1
  only because the test program read that missing file; this is not a
  platform error.
- Restoring does not bring back the image's ENTRYPOINT/CMD (7d), so the
  Pod's command must be known independently of the image. For a Pod
  without `command`, that means the image's own config.

Decision (2026-10-01): not adopted. Ten seconds per start, tmpfs paths
lost, and a hard dependency on `/bin/sh` in the image outweigh the
benefit. ConfigMap and Secret values go through environment variables,
and API access through the HTTPS interceptor, which attaches the Pod's
identity (plans/pod-on-containers.md).

## Round 4: idle survival with the 6 h inactivity timeout

`app` was started at 23:48 JST with `setInactivityTimeout(21600000)`
(accepted), then touched last at 23:59 (running, uptime 687 s). The Worker
was redeployed once in between (23:55) and the container survived it. At
00:24, after 25 minutes with no request to its DO, it was stopped
(`running: false`). Containers without the timeout stopped after about
4 minutes. So the timeout extends idle life but does not keep a container
for 6 h when its DO receives nothing; whether the DO's eviction or the
redeploy reset the setting is not isolated. The PodKubelet DO must keep
itself active with an alarm at a short interval (minutes) rather than
relying on the 6 h figure, and the interval should be measured before
settling on it.
