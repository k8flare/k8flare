// S15 round 2 throwaway spike: boots ONE node-in-a-Container microVM on
// Cloudflare Containers, joining the production k8flare control plane.
// The bearer token from the bootstrap request is forwarded into the VM as
// K3S_TOKEN (no secret stored on this worker; a wrong token just fails
// the join at the gateway). DELETE THIS WORKER after the spike.
import { Container } from "@cloudflare/containers";

export class FargateNodeVM extends Container<unknown> {
  defaultPort = 10250; // kubelet's port -- open means the agent came up
  sleepAfter = "10m";
  override async onActivityExpired(): Promise<void> {} // spike: manual /down only
  async up(serverURL: string, nodeName: string, token: string) {
    await this.startAndWaitForPorts({
      ports: this.defaultPort,
      startOptions: {
        envVars: { SERVER_URL: serverURL, NODE_NAME: nodeName, K3S_TOKEN: token },
      },
    });
    return this.getState();
  }
  async state() {
    return this.getState();
  }
  async down() {
    await this.destroy();
  }
}

interface Env {
  VM: DurableObjectNamespace<FargateNodeVM>;
}

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const url = new URL(req.url);
    const auth = req.headers.get("Authorization") ?? "";
    const token = auth.replace(/^Bearer /, "");
    if (!token) return new Response("token required", { status: 401 });
    const vm = env.VM.get(env.VM.idFromName("solo"));
    try {
      if (url.pathname === "/up") {
        const state = await vm.up(
          "https://k8flare-gateway.kooffice.workers.dev",
          "s15-cf-microvm",
          token,
        );
        return Response.json({ ok: true, state });
      }
      if (url.pathname === "/state") return Response.json(await vm.state());
      if (url.pathname === "/down") {
        await vm.down();
        return Response.json({ ok: true, down: true });
      }
    } catch (err) {
      return Response.json({ ok: false, error: String(err) }, { status: 500 });
    }
    return new Response("s15 spike: /up /state /down", { status: 404 });
  },
};
