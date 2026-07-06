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
  // lifecycle is owned by the scheduler DO (destroy on pod termination),
  // never by inactivity.
  sleepAfter = "10m";
  override async onActivityExpired(): Promise<void> {}

  override onError(error: unknown): unknown {
    console.error(`NodeVM error: ${String(error)}`);
    return error;
  }

  /** Boots the node VM (idempotent) with its join parameters. */
  async up(nodeName: string): Promise<void> {
    const state = await this.getState();
    if (state.status === "running" || state.status === "healthy") return;
    const serverURL = this.env.GATEWAY_URL;
    if (!serverURL) throw new Error("GATEWAY_URL is not configured on workers/nodes");
    await this.startAndWaitForPorts({
      ports: this.defaultPort,
      startOptions: {
        envVars: {
          SERVER_URL: serverURL,
          NODE_NAME: nodeName,
          K3S_TOKEN: this.env.K3S_TOKEN ?? "k8flare-dev-token",
        },
      },
    });
  }

  async currentState() {
    return this.getState();
  }

  /** Pod terminated/deleted -- tear the whole node VM down. */
  async destroyVM(): Promise<void> {
    await this.destroy();
  }

  // Kubelet bridge: the nodes Worker forwards gateway requests here as
  // /{port}/{kubelet path}. Only 10256 is reachable -- cmd/agent's
  // plain-HTTP proxy in front of the authenticated kubelet API
  // (/containerLogs, /stats/summary, /metrics/resource; k3s pins the
  // read-only 10255 off via CLI flag). Streams straight through, so
  // `kubectl logs -f` follows for as long as the client stays connected
  // (CPU-time billing only -- idle stream time is I/O wait).
  override async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const match = url.pathname.match(/^\/(10256)(\/.*)$/);
    if (!match) {
      return new Response("NodeVM: expected /10256/...", { status: 400 });
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
