# Pod on Containers v2: containerd inside the PodKubelet container

Revision of `plans/pod-on-containers.md`. The `PodKubelet` Durable Object
stays the kubelet, the virtual Node `cloudflare`, the placement pass, the
`PodLedger`, egress routing and the kubectl stream front stay as they are.
What changes is what runs inside the one container a Pod gets: instead of
the Pod's own image, it is a fixed containerd image, and the Pod's
containers run inside it through CRI.

## Why

v1 can only start images declared in the deployment config or digest-pinned in
`registry.cloudflare.com`, because the `durable_object` scheduling policy
pulls nothing else (FINDINGS.md #1b, image-management docs). It also
rejects every Pod with more than one container, init containers or
volumes, and cannot serve `kubectl logs`. Running containerd in the
container moves image pulls and container lifecycle inside it, where
Docker Hub, tags, sidecars and logs all work.

## Configuration: `cloudflare.config.ts`

The project moves from `wrangler.jsonc` to `cloudflare.config.ts` (cf CLI,
`@cloudflare/config`) before this work, and every config below is written
in that form. `cf migrate --dry-run` (cf 1.0.0-beta.12, 2026-10-14) does
not carry the parts this plan depends on and lists them as required
manual follow-ups: the `containers` entry, all eight Durable Object
bindings, and the Durable Object `migrations` (replaced by `exports`
lifecycle declarations).

The container is declared with `defineContainer` and attached to the
`PodKubelet` export:

```ts
const podRuntime = defineContainer({
  name: "k8flare-pod-runtime",
  schedulingPolicy: "durable-object",
  images: { "pod-runtime": { dockerfile: "./images/pod-runtime/Dockerfile" } },
});

exports: {
  PodKubelet: exports.durableObject({ storage: "sqlite", container: podRuntime }),
}
```

Spelling differs from Wrangler: `schedulingPolicy: "durable-object"`
(hyphen), not `scheduling_policy: "durable_object"`. cf rejects preview
deployments for `durable-object` containers, so every platform check uses
a full deployment (`cf build`, `cf deploy --dry-run`, then `cf deploy`
after approval).

## Decisions

- **One Pod, one container, one containerd.** The `PodKubelet` DO starts
  one `durable_object` container from a single declared image
  (`images/pod-runtime`: containerd with its CRI plugin, runc and
  `crictl`, nothing Pod-specific), built at deploy time. containerd
  rather than `dockerd`: it is what the kubelet itself drives, CRI
  already has Pod sandboxes, image pulls with auth and kubelet-format
  container logs, and it carries no second daemon. The Pod's image is no longer the container's image.
- **The Pod sandbox uses the node network namespace.** Cloudflare
  Containers do not allow iptables (Sandbox Docker-in-Docker guide), so
  there is no CNI: the sandbox is created with
  `NamespaceOption.network: NODE`, which needs no CNI plugin. All containers of
  the Pod share the outer container's network stack, which is exactly a
  Pod's shared network namespace: sidecars reach each other on
  `localhost`.
- **Ports inside a Pod must not collide.** The placement pass rejects
  (Pending + Warning Event) a Pod in which two containers declare the
  same `containerPort` and protocol. Upstream only warns; this compute
  class rejects, because the collision is certain. An undeclared
  collision fails at bind time inside the container and surfaces as
  that container's crash and CrashLoopBackOff, as on a real node.
- **The DO drives CRI through `crictl` over `exec`.** CRI is gRPC on a
  unix socket, which `getTcpPort().fetch` (HTTP/1.1) cannot speak, so the
  DO runs `crictl ... -o json` with `ctx.container.exec` (16–43 ms per
  call, FINDINGS #4) and parses the output. No port is opened. Streaming
  calls (`exec`, `attach`, `logs -f`) run `crictl` under `exec` with
  `pty` or piped stdio. If `exec` round trips turn out too slow, a small
  HTTP shim inside the image is the fallback, decided in step 0.
- **Images are pulled by containerd.** Any reference containerd accepts
  (Docker Hub, tags, other registries) works. `imagePullSecrets` become
  `crictl pull --creds` / `--auth`. `imagePullPolicy` follows the kubelet
  rules. Pulled images live on the container's disk and are lost when
  the container stops; there is no cache across Pods in this version.
- **Container ordering follows the kubelet.**
  1. Init containers run one at a time; each must exit 0 before the next.
     A failure restarts the sequence under the Pod's `restartPolicy`.
  2. Native sidecars (`initContainers` with `restartPolicy: Always`)
     start in declaration order; the next starts once the sidecar is
     running and its `startupProbe`, if any, has passed. They keep
     running and restart on exit.
  3. App containers start together.
  4. On deletion: app containers get SIGTERM, then sidecars in reverse
     order, all within `terminationGracePeriodSeconds`, then `destroy()`.
- **Volumes become directories inside the container.** `emptyDir` is a
  directory under `/var/lib/k8flare/pods/<uid>/volumes/` bind-mounted into
  every container that mounts it. ConfigMap, Secret, downwardAPI and
  projected volumes (including the ServiceAccount token) are written there
  by the DO through `exec` before the containers start
  and rewritten when they change. This lifts v1's token-file limitation,
  so `rest.InClusterConfig` works. `hostPath` and persistent volumes stay
  rejected.
- **Logs and attach come from containerd.** CRI writes each container's
  log in the kubelet format under the sandbox's log directory, and keeps
  the previous instance's file, so `kubectl logs` (`-c`, `--previous`,
  `--since`, `-f`) reads those files. `attach` is `crictl attach`. This
  removes FINDINGS #19 and the stdout-symlink failure (#2c), because the
  inner containers' stdio belongs to containerd, not the platform.
- **Instance shape includes the runtime's overhead.** The Pod's summed
  requests (max of request and limit, as in v1) plus a fixed
  containerd allowance become the instance. The allowance is measured in
  step 0, not guessed. If it does not fit in `lite`, `lite` is no longer
  offered.
- **Networking is unchanged and stays in the DO.** Egress interception,
  ClusterIP / Pod IP routing between Pod DOs, ingress through
  `getTcpPort(port)` and `port-forward` keep working as in v1, on the
  assumption that host-network inner containers are seen by the platform
  as the outer container. That assumption is checked in step 0.

## Rejected after this change (Pending + Event)

`hostNetwork`, `hostPID`, `hostIPC`, `privileged`, `hostPath`,
persistent volumes, duplicate declared ports within a Pod, a shape
outside the instance limits, egress to Services over anything but
HTTP/HTTPS (unchanged from v1).

## Removed from v1

The image allowlist (`CONTAINERS_IMAGES`, `images.generated.ts`,
`scripts/genimages.mjs`, the `containers.k8flare.com/image` annotation),
the recorded ENTRYPOINT/CMD metadata, the single-container and no-volume
rejections, the `logs` and `attach` refusals.

## Steps and how each is checked

Platform checks need a Cloudflare deployment; each one is asked for
before it runs and deleted afterwards.

| # | Step | Check |
|---|---|---|
| M | Migrate the repo to `cloudflare.config.ts` (separate change, first) | `cf migrate`, resolve every `[required]` item, then `cf build`, `cf deploy --dry-run`, `make gen`, Unit/E2E/Conformance green; `wrangler dev` in `scripts/ci/e2e.sh` becomes `cf dev`; `wrangler.jsonc` deleted |
| 0 | Spike: containerd in a `durable_object` container | Extend `experiments/do-kubelet-spike`, moved to `cloudflare.config.ts`. Locally first (`cf dev` + Docker), then one approved deployment. Record in FINDINGS.md: (a) containerd with runc starts (cgroups, overlayfs or the native snapshotter); (b) `crictl runp` with a NODE network sandbox succeeds without CNI; (c) `crictl pull nginx:1.27` from Docker Hub and the start-to-ready time; (d) two containers in the sandbox talk over `localhost`; (e) the DO reaches an inner container's port via `getTcpPort`; (f) `interceptAllOutboundHttp` still sees inner containers' egress; (g) containerd's idle memory; (h) latency of `crictl` through `exec`, to decide on the shim |
| 1 | Pure rules: ordering, port collision, shape with allowance, volume plan | Go/TS unit tables: Pod spec → start plan or rejection with reason |
| 2 | Placement pass update | host test: duplicate-port Pod stays Pending with the Event; a multi-container Pod with an `emptyDir` and Docker Hub image is bound |
| 3 | `PodKubelet` drives CRI: pull, init, sidecars, apps, status, restart, delete | unit tests with `crictl` output faked; then local `cf dev` if step 0 shows it runs `durable_object` containers |
| 4 | Volumes: emptyDir, ConfigMap, Secret, projected token | unit tests; on the deployment, a Pod using `rest.InClusterConfig` lists Pods |
| 5 | Logs and attach through CRI | `kubectl logs -c`, `--previous`, `-f` and `kubectl attach` against the deployment |
| 6 | e2e: nginx + a sidecar that `wget`s `localhost`, reached through a Service from another `cloudflare` Pod | on the deployment, with approval |
| 7 | Remove the v1 image allowlist and metadata; update the v1 plan and README | `make gen`, unit tests, full CI green |

## Open questions

- Whether `cf dev` runs `durable-object` containers locally (v1 left
  this unverified for `wrangler dev`; cf's local runtime contains
  handling for the policy, not yet exercised). If not, steps 3 to 6 are checked only on the
  deployment, which raises the number of deployments to ask for.
- Docker Hub's pull rate limits apply per pull with no cache. A shared
  pull-through cache is out of scope for this version.
- Start latency: container start (about 0.6 s measured) plus containerd
  start plus the image pull. Step 0 measures it.
