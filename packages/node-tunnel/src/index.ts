import { DurableObject } from "cloudflare:workers";
import "../../control-plane-worker/assets/wasm/wasm_exec.js";
import nodeTunnelWasm from "../assets/node-tunnel.wasm";

declare const Go: new () => {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance, context: unknown): Promise<void>;
};

interface RequestOut {
  status: number;
  headers: [string, string][];
  body: Uint8Array | null;
}

interface Binding {
  handleRequest(req: unknown, env: unknown): Promise<RequestOut>;
  attach(nodeName: string, send: (bytes: Uint8Array) => void): void;
  message(bytes: Uint8Array): void;
  closed(): void;
  tick(): void;
}

function stringVar(env: Env, key: string): string {
  return (env as unknown as Record<string, string | undefined>)[key] ?? "";
}

function instantiate(env: Record<string, unknown>): Promise<Binding> {
  return new Promise((resolve, reject) => {
    const go = new Go();
    const binding: Partial<Binding> = {};
    const context = { env, ctx: null, binding, ready: () => resolve(binding as Binding) };
    go.run(new WebAssembly.Instance(nodeTunnelWasm as WebAssembly.Module, go.importObject), context).then(
      () => reject(new Error("node-tunnel: go program exited before ready")),
      (err: unknown) => reject(err),
    );
  });
}
export class NodeTunnel extends DurableObject<Env> {
  private goInstance: Promise<Binding> | null = null;
  private attached = false;

  private go(): Promise<Binding> {
    if (!this.goInstance) {
      this.goInstance = instantiate({
        ADMIN_TOKEN: this.env.ADMIN_TOKEN,
        KUBELET_CLIENT_CERT: stringVar(this.env, "KUBELET_CLIENT_CERT"),
        KUBELET_CLIENT_KEY: stringVar(this.env, "KUBELET_CLIENT_KEY"),
        KUBELET_CA: stringVar(this.env, "KUBELET_CA"),
      });
    }
    return this.goInstance;
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/v1-k3s/connect") return this.acceptTunnel(request);
    const binding = await this.go();
    const raw = await request.arrayBuffer();
    const out = await binding.handleRequest(
      { method: request.method, url: request.url, headers: [...request.headers], body: raw.byteLength === 0 ? null : new Uint8Array(raw) },
      {},
    );
    return new Response(out.body, { status: out.status, headers: out.headers });
  }

  private async acceptTunnel(request: Request): Promise<Response> {
    if (request.headers.get("Upgrade") !== "websocket") {
      return new Response("websocket upgrade required", { status: 426 });
    }
    const nodeName = request.headers.get("X-K8flare-Node") ?? "";
    if (!nodeName) return new Response("missing node name", { status: 400 });
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    this.ctx.acceptWebSocket(server);
    const binding = await this.go();
    binding.attach(nodeName, (bytes) => {
      try {
        server.send(bytes);
      } catch {
        server.close(1011, "send failed");
      }
    });
    this.attached = true;
    return new Response(null, { status: 101, webSocket: client });
  }

  async webSocketMessage(ws: WebSocket, message: string | ArrayBuffer): Promise<void> {
    if (!this.attached) {
      ws.close(1012, "no tunnel session, reconnect");
      return;
    }
    const bytes = typeof message === "string" ? new TextEncoder().encode(message) : new Uint8Array(message);
    const binding = await this.go();
    binding.message(bytes);
  }

  async webSocketClose(): Promise<void> {
    if (this.attached) (await this.go()).closed();
    this.attached = false;
  }

  async webSocketError(): Promise<void> {
    if (this.attached) (await this.go()).closed();
    this.attached = false;
  }
}
