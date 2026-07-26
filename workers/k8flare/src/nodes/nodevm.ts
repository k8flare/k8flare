// NodeVM: Cloudflare Containers-backed Durable Object classes hosting ONE
// per-Pod microVM node each (the cf-containers-scheduler model that
// replaces the old VirtualNode/PodContainer backend): the container image
// is this repo's node image -- an unmodified k3s agent embed (kubelet +
// containerd) plus the official k3s binary, see images/node/ -- and the
// Pod's actual (arbitrary) image is pulled by containerd INSIDE the
// microVM. One class per size tier because Cloudflare Containers fix
// instance_type per class at deploy time (same platform constraint the
// old backend documented; S15 spike proved the whole shape end-to-end,
// spikes/s15-containers-node/FINDINGS.md).
import { Container } from "@cloudflare/containers";
import type { Env } from "./env.ts";

export abstract class NodeVMBase extends Container<Env> {
  // kubelet's port: reachable means the agent booted. Also the target for
  // the future logs/exec bridge.
  defaultPort = 10250;
  // Required by @cloudflare/containers to arm its monitoring alarm at all;
  // lifecycle is owned by the scheduler DO (destroy on pod termination) --
  // but onActivityExpired below is the ask-before-sleep safety net cost
  // invariant #2 requires, NOT a no-op: if the scheduler's teardown ever
  // leaks a VM (the 2026-07-16 incident left 21 provisioned instances
  // billing wall-clock for days), the VM asks the scheduler whether it is
  // still tracked and destroys itself when it isn't.
  sleepAfter = "10m";

  override async onActivityExpired(): Promise<void> {
    // This DO's name is the owning Pod's UID (scheduler.ts vmStub()).
    const uid = this.ctx.id?.name;
    if (!uid) return;
    try {
      const sched = this.env.SCHEDULER.get(this.env.SCHEDULER.idFromName("default"));
      const resp = await sched.fetch(
        `http://scheduler.internal/internal/vm-tracked?uid=${encodeURIComponent(uid)}`,
      );
      const body = (await resp.json()) as { tracked?: boolean };
      if (resp.ok && body.tracked === false) {
        console.log(`NodeVM ${uid}: untracked at activity expiry -- destroying (leak guard)`);
        await this.destroy();
      }
      // Tracked (a Pod still owns this VM) or the check failed: stay up;
      // the next expiry re-asks. Conservative on error so a transient
      // scheduler hiccup can't kill a live workload.
    } catch (err) {
      console.log(`NodeVM ${uid}: leak-guard check failed (staying up): ${err}`);
    }
  }

  override onError(error: unknown): unknown {
    console.error(`NodeVM error: ${String(error)}`);
    return error;
  }

  /**
   * Boots the node VM (idempotent) with its join parameters.
   * meshConnectorToken (spikes/s17-mesh-nodevm/FINDINGS.md's per-Pod-Mesh
   * entry, minted per-Pod by scheduler.ts via nodes/meshconnector.ts) is
   * passed straight through as MESH_CONNECTOR_TOKEN -- cmd/agent already
   * reads that env var as its --mesh-connector-token default, so no
   * entrypoint.sh change was needed to wire it through, only to start
   * warp-svc and pass --mesh-ip-as-node-ip.
   */
  async up(nodeName: string, meshConnectorToken?: string): Promise<void> {
    const state = await this.getState();
    if (state.status === "running" || state.status === "healthy") return;
    const serverURL = this.env.GATEWAY_URL;
    if (!serverURL) throw new Error("GATEWAY_URL is not configured on workers/nodes");
    const envVars: Record<string, string> = {
      SERVER_URL: serverURL,
      NODE_NAME: nodeName,
      K3S_TOKEN: this.env.K3S_TOKEN ?? "k8flare-dev-token",
    };
    if (meshConnectorToken) envVars.MESH_CONNECTOR_TOKEN = meshConnectorToken;
    // CI/local-dev only (smoke-nodes.yml): base64 PEM of the harness's
    // self-signed gateway CA, appended to the VM's system bundle by
    // entrypoint.sh so the embedded k3s agent can join a wrangler-dev
    // gateway. Never set in production (GATEWAY_URL is publicly trusted).
    if (this.env.GATEWAY_CA_B64) envVars.K8FLARE_EXTRA_CA_B64 = this.env.GATEWAY_CA_B64;
    await this.startAndWaitForPorts({
      ports: this.defaultPort,
      startOptions: { envVars },
    });
  }

  async currentState() {
    return this.getState();
  }

  /** Pod terminated/deleted -- tear the whole node VM down. */
  async destroyVM(): Promise<void> {
    await this.destroy();
  }

  // Generic port bridge: the nodes Worker forwards gateway requests here
  // as /{port}/{path}. Two callers, same shape: the kubelet bridge (port
  // 10999 -- cmd/agent's plain-HTTP proxy in front of the authenticated
  // kubelet API, /containerLogs, /stats/summary, /metrics/resource; k3s
  // pins the read-only 10255 off via CLI flag) and, since `hostNetwork:
  // true` (computeclass.go) makes every container port directly
  // reachable on the VM's own network stack, task #13's pods/proxy and
  // virtual-kube-proxy bridges (arbitrary Pod container ports -- verified
  // directly reachable this way over SSH, docs/platform-verification.md
  // S16: `curl 127.0.0.1:80` returned the Pod's own nginx page). No
  // allowlist beyond "numeric port": containerFetch only ever reaches
  // this one VM's own loopback-equivalent network, so there is no
  // cross-Pod traffic to gate here -- callers (nodes/index.ts) are the
  // ones responsible for authorizing the request before it gets here.
  // Streams straight through, so `kubectl logs -f` follows for as long as
  // the client stays connected (CPU-time billing only -- idle stream time
  // is I/O wait).
  override async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const match = url.pathname.match(/^\/(\d+)(\/.*)$/);
    if (!match) {
      return new Response("NodeVM: expected /{port}/...", { status: 400 });
    }
    const [, port, rest] = match;
    url.pathname = rest;
    // The Authorization header (cluster bearer token) is forwarded
    // as-is: the kubelet runs stock webhook auth and TokenReviews the
    // token against this cluster's own apiserver, so the bridged port
    // is useless without a valid cluster token.
    return this.containerFetch(new Request(url.toString(), request), Number(port));
  }
}

export class NodeVMSmall extends NodeVMBase {}
export class NodeVMMedium extends NodeVMBase {}
export class NodeVMLarge extends NodeVMBase {}
