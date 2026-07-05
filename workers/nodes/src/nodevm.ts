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
}

export class NodeVMSmall extends NodeVMBase {}
export class NodeVMMedium extends NodeVMBase {}
export class NodeVMLarge extends NodeVMBase {}
