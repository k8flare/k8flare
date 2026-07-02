# S3 — Cloudflare Containers spike findings

Verified 2026-07-02. Environment: Docker 29.4.0 (OrbStack backend,
linux/aarch64 host), wrangler 4.106.0, `@cloudflare/containers` 0.3.7
(installed spike-locally with `--ignore-workspace`; root lockfiles untouched).
Scope was narrowed mid-spike by the coordinating session: controllers-fallback
concerns dropped (user decision 2026-07-02: controllers are WASM-only), Pod
backend (Phase 7) concerns kept. Docker images/containers were removed after
verification. Spike code: `images/` (echo, scheduler, controller-manager
Dockerfiles), `hoststream/`, `worker/`. The agent that ran this spike could
not write report files, so this document was transcribed by the coordinating
session from its full report.

## 1. Real-binary images (measured)

`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"`:

| Binary | FROM scratch | distroless/static-debian12:nonroot |
|---|---|---|
| cmd/scheduler | 73.5 MB | 75.6 MB |
| cmd/controller-manager | 95.2 MB | 97.2 MB |
| minimal Go echo server | 6.33 MB | — |

**Key empirical finding: `FROM scratch` has no CA bundle.** A test hitting
`https://cloudflare.com` from the scratch image failed TLS verification with
`x509: certificate signed by unknown authority`. The comment in cmd/scheduler
("Workers present publicly trusted certs, no custom CA needed") is about the
server side; the client-side CA bundle is a separate concern. Distroless adds
~2 MB and includes the bundle. **Production Dockerfiles (including any k8flare
base images for Pods) should be distroless-family, not scratch.**

SIGTERM: `docker stop` completes in 0.2 s (`signal.NotifyContext` graceful
shutdown works).

Note: these images are no longer needed for Phase 5 (controllers are
WASM-only by user decision), but remain useful for BYO VM Docker distribution.

## 2. wrangler dev + Containers + DO developer experience (measured)

- `wrangler dev` performs a real local `docker build` (same build path as
  deploy).
- **The Dockerfile MUST contain `EXPOSE <port>`** — setting `defaultPort` on
  the DO class alone fails at startup. Undocumented; found empirically.
- Startup pulls `docker.io/cloudflare/proxy-everything` (egress-control
  sidecar; see item 5).
- DO-side `fetch()` proxies to `defaultPort` fine. First access (demand
  start) ≈ 0.86 s locally — NOT representative of production cold start
  (official figures: typically 1–3 s, up to 3–15 s in practice).
- Logs appear in wrangler dev stdout; `docker logs <generated-name>` also
  works.

## 3. onActivityExpired override (measured, two DO variants compared)

With `sleepAfter="20s"`, compared via `getState()` + `docker ps` + logs:

- Default (no override): after 20 s the state becomes `stopped_with_code`
  (exitCode=2), the container leaves `docker ps`, `onStop` fires.
- Overridden without calling `stop()`/`destroy()`: container stays `healthy`
  indefinitely, remains in `docker ps`, and **`onActivityExpired` fires
  repeatedly (~every 20 s; count=2 observed)** — it is not a one-shot hook.

Official Warning confirmed accurate.

**Bonus (answers an open S3 question):** reading the shipped
`@cloudflare/containers@0.3.7` code (`dist/lib/container.js`, fetched from
npm): the Container class self-monitors via the DO `alarm()`, re-arming at
most every 3 minutes (or the remaining `sleepAfter`, whichever is shorter)
while the container runs, and calling `ctx.storage.deleteAlarm()` once the
container is stopped with no pending schedules — i.e. it parks itself when
idle (source comment: "container DOs ALWAYS need an alarm right now"). This
matches the repo's event-armed alarm rule (cost invariant #3). Quantitative
consequence for Phase 7 cost modeling: **each running Pod container implies a
DO alarm firing at least every 3 minutes.**

## 4. Arbitrary images at runtime — NOT possible (desk research, confirmed)

Per Image Management docs and the full Containers changelog read from
2025-09-25 through 2026-07-01 (including the 2026-07-01 Google Artifact
Registry entry): `containers[].image` is deploy-time-fixed (Dockerfile path or
fully qualified registry reference). No API lets a Worker/DO choose an
arbitrary image at runtime; all recent changes extend deploy-time
configuration only.

→ **Phase 7 v1 is the "wrangler-defined image allowlist" model.** `kubectl
run --image=<anything>` cannot work; Pods can only run images k8flare has
pre-registered. Must be stated honestly in the README.

## 5. Outbound UDP — officially impossible; local dev CANNOT verify egress policy

Official docs are explicit: outbound ports other than 80/443 bypass no
handler — they are unsupported; DNS is Cloudflare's resolvers only; the port
restriction applies regardless of `enableInternet`. UDP is unsupported
inbound and outbound. → VXLAN-over-UDP and CoreDNS UDP:53 cannot run on
Containers as-is.

**However, local `wrangler dev` does not enforce any of this** (found
empirically): a default-configured container reached
`host.docker.internal:8846` (non-80/443) successfully, and so did a third
test container with explicit `enableInternet = false` and no `allowedHosts`.
Two candidate explanations (not separable locally): `usingInterception` in
`container.js` only activates when `outboundByHost`/`allowedHosts` are set;
and `host.docker.internal` is Docker's host-loopback special name, possibly
outside the proxy-everything sidecar's scope.

→ Treat "no UDP / no non-80/443 egress" as fact (official docs), but **never
conclude egress policy behavior from local dev** — production Containers must
be re-tested before relying on `enableInternet`/`allowedHosts` for Pod
network isolation.

## 6. Container → host long-lived stream (measured, works)

A host-side chunked HTTP stream server (1 s interval × 25 ticks) was relayed
from inside the container to the caller with immediate flushing: intervals
preserved in real time, full 25-tick stream completed. (Also succeeded with
`enableInternet=false` — see item 5 on local non-enforcement.) Source IP
observed as 127.0.0.1 on the host (possibly OrbStack-specific). This proves
only Docker-level stream mechanics; the production "container → k8flare
apiserver watch" path (reconnects, resourceVersion continuation) is a
Phase 5/7 implementation-time concern.

## Production-only remaining items

1. Project-specific cold-start measurement (was S7's charter; S7 closed as
   moot for controllers, still relevant to Phase 7 Pod startup latency).
2. Whether `enableInternet=false`/`allowedHosts`/`deniedHosts` are actually
   enforced in production (locally they are not).
3. Empirical confirmation that UDP is fully blocked.
4. How the egress-control sidecar is billed.
5. Cold-start penalty of pulls from non-Cloudflare registries.

## Implications for Phase 7 (Pod-on-Containers)

- Image allowlist model is settled (item 4); README must say so.
- No UDP hits Cluster DNS directly (item 5): CoreDNS running as a
  Containers-hosted Pod cannot serve UDP:53. Either place CoreDNS on BYO VM
  nodes or rethink the DNS path (S4 already ruled out Mesh as the answer).
- Per-running-Pod DO alarm every ≤3 min (item 3) must enter the Phase 7 cost
  model (Pod workload cost, not control-plane cost, per cost invariant #6).
- k8flare-provided base images should be distroless-family (CA bundle,
  item 1).

## Transcriber's note

The spike report inferred that S8 "succeeded" from the task list title
("WASM 一本化"). That inference is incorrect as to cause: the WASM-only
direction is a user decision (2026-07-02) made independently of S8's outcome;
S8 was still running when this spike completed. Recorded here so the wrong
causal claim does not propagate.
