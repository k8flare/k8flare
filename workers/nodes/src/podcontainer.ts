// PodContainer* Durable Object classes: one Cloudflare Containers-backed DO
// class per size tier (images.ts's SIZE_TIERS), each instance backing
// exactly one Pod (keyed by `<namespace>/<name>`, see virtualnode.ts). Split
// into three classes (not one parametrized class) because Cloudflare
// Containers fixes both `image` and `instance_type` per wrangler.jsonc
// `containers[]` entry / DO class at deploy time -- there is no runtime way
// to pick a size for a single class (spikes/s3-containers/FINDINGS.md's
// instance-sizing addendum), so the size tier has to be baked in via which
// class a Pod's stub gets constructed from (see virtualnode.ts's
// containerBindingForTier).
//
// Deliberately thin: this class only starts/stops/reports the container.
// restartPolicy handling and Pod-status reconciliation live in
// virtualnode.ts's reconcile loop, not here -- keeping this class a dumb
// wrapper around @cloudflare/containers' Container means the interesting
// logic lives in one place and is testable without Docker.
import { Container } from "@cloudflare/containers";
import type { Env } from "./env.ts";

export abstract class PodContainerBase extends Container<Env> {
  defaultPort = 8080; // matches images/demo's hardcoded PORT default

  // sleepAfter is irrelevant in practice (onActivityExpired below never
  // calls stop()/destroy()), but @cloudflare/containers requires some value
  // to arm its self-monitoring alarm in the first place (see FINDINGS.md
  // item 3) -- set generously long so that alarm (re-armed at most every 3
  // minutes while running, per FINDINGS.md) fires as rarely as the library
  // allows.
  sleepAfter = "10m";

  // Deliberately does NOT call this.stop()/this.destroy(): a Pod's backing
  // container must stay resident until the owning Pod is actually
  // deleted/completes (cost invariant #6 -- Pod workload cost is the user's
  // cost, not subject to control-plane idle-stop), not merely because
  // nothing has hit its HTTP port for a while. FINDINGS.md's S3 measurement
  // confirmed this hook fires repeatedly (not one-shot) for as long as the
  // container keeps running with no stop() call -- this override relies on
  // exactly that documented behavior.
  override async onActivityExpired(): Promise<void> {}

  override onError(error: unknown): unknown {
    console.error(`PodContainer error: ${String(error)}`);
    return error;
  }

  /**
   * Starts the container if it isn't already up, waiting for defaultPort to
   * be reachable. Idempotent. envVars (Phase 8: R2 PV/PVC backend) are
   * passed straight through to @cloudflare/containers' startOptions --
   * used by virtualnode.ts to inject AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/
   * AWS_SESSION_TOKEN (and this project's own R2_ENDPOINT/R2_BUCKET/
   * R2_PREFIX) for a Pod whose spec.volumes references a PersistentVolumeClaim.
   * Note: env vars are fixed at container start and never updated in place
   * for an already-running container -- see virtualnode.ts's doc comment on
   * the credential-refresh design for how (and how far) this project works
   * around that for long-running Pods.
   */
  async ensureRunning(envVars?: Record<string, string>): Promise<void> {
    const state = await this.getState();
    if (state.status === "running" || state.status === "healthy") return;
    await this.startAndWaitForPorts({ ports: this.defaultPort, startOptions: { envVars } });
  }

  /** RPC-visible wrapper so virtualnode.ts can read container health without an extra fetch(). */
  async currentState() {
    return this.getState();
  }

  /** Pod deleted or terminal (Succeeded/Failed, restartPolicy != Always) -- tear the container down. */
  async stopContainer(): Promise<void> {
    await this.destroy();
  }
}

export class PodContainerSmall extends PodContainerBase {}
export class PodContainerMedium extends PodContainerBase {}
export class PodContainerLarge extends PodContainerBase {}
