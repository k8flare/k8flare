import { apiserverFetch } from "./loader.ts";
import { isolateId } from "@k8flare/loader-kit";
import type { QueueMessage } from "@k8flare/cluster-store";
import { consume } from "./queues.ts";
import { withClientCert } from "./clientcert.ts";
import { refuseUpgradeAfterAdmission } from "./upgrade.ts";
import { clusterStub, tunnelName } from "./clusterid.ts";
import { Metrics } from "./metrics.ts";
import { bytesOf, sendBinary, sendLog } from "./podstream.ts";
import type { NodeTunnel } from "@k8flare/node-tunnel";
import { servePodStream } from "./podkubelet/streams.ts";
import { VIRTUAL_NODE } from "./podkubelet/spec.ts";

export { Cluster } from "@k8flare/cluster-store";
export { NodeTunnel } from "@k8flare/node-tunnel";
export { Printers } from "./printers.ts";
export { OpenAPI } from "./openapi.ts";
export { CustomResources } from "./customresources.ts";
export { APIGroups } from "./apigroups.ts";
export { Scheduler } from "./scheduler.ts";
export { Workloads } from "./workloads.ts";
export { GarbageCollector } from "./gc.ts";
export { HPAAPIServer, AttachDetachAPIServer } from "./apiserver.ts";
export { Storage } from "./storage.ts";
export { NodeTunnels } from "./nodetunnel.ts";
export { Admission } from "./admission.ts";
export { AttachDetach } from "./attachdetach.ts";
export { HorizontalPodAutoscaler } from "./hpactrl.ts";
export { Hooks } from "./hooks.ts";
export { Outbound } from "./outbound.ts";
export { Metrics };
export { CFContainersScheduler } from "./nodes/scheduler.ts";
export { NodeVMSmall, NodeVMMedium, NodeVMLarge } from "./nodes/nodevm.ts";
export { PodKubelet } from "./podkubelet/kubelet.ts";
export { PodLedger } from "./podkubelet/ledger.ts";

async function digest(value: string): Promise<ArrayBuffer> {
  return crypto.subtle.digest("SHA-256", new TextEncoder().encode(value));
}

async function isAdmin(request: Request, env: Env): Promise<boolean> {
  if (!env.ADMIN_TOKEN) return false;
  const [given, want] = await Promise.all([digest(request.headers.get("Authorization") ?? ""), digest(`Bearer ${env.ADMIN_TOKEN}`)]);
  return crypto.subtle.timingSafeEqual(given, want);
}

function withAuthorization(request: Request, extra?: Headers): Headers {
  const headers = new Headers(request.headers);
  const auth = request.headers.get("Authorization");
  if (auth) headers.set("Authorization", auth);
  extra?.forEach((value, key) => headers.set(key, value));
  return headers;
}

async function acceptTunnel(request: Request, env: Env): Promise<Response> {
  if (request.headers.get("Upgrade") !== "websocket") {
    return new Response("websocket upgrade required", { status: 426 });
  }
  const authHeaders = withAuthorization(request);
  const bearer = authHeaders.get("Authorization") ?? "";
  if (bearer.startsWith("Bearer ")) authHeaders.set("X-K8flare-Node-Token", bearer.slice("Bearer ".length));
  const authed = await apiserverFetch(env, new Request("https://apiserver.internal/v1-k3s/node-tunnel", {
    method: "POST",
    headers: authHeaders,
    body: "1",
    redirect: "manual",
  }));
  if (!authed.ok) return new Response("not authorized", { status: 401 });
  const body = (await authed.json()) as { node?: string };
  if (!body.node) return new Response("not authorized", { status: 401 });
  const stub = env.NODE_TUNNEL.get(env.NODE_TUNNEL.idFromName(tunnelName(env, body.node)));
  const extra = new Headers();
  extra.set("X-K8flare-Node", body.node);
  return stub.fetch(new Request(request, { headers: withAuthorization(request, extra) }));
}

function isStreamPath(path: string): boolean {
  return /\/pods\/[^/]+\/(exec|attach|portforward|log)(?:\/|$)/.test(path);
}

function streamUpgrade(client: WebSocket, protocol: string): Response {
  const headers = new Headers();
  if (protocol) headers.set("Sec-WebSocket-Protocol", protocol);
  return new Response(null, { status: 101, webSocket: client, headers });
}

function isNonWebSocketUpgrade(request: Request): boolean {
  const upgrade = (request.headers.get("Upgrade") || "").toLowerCase();
  return upgrade !== "" && upgrade !== "websocket" && isStreamPath(new URL(request.url).pathname);
}

function asAPIRequest(request: Request): Request {
  if ((request.headers.get("Upgrade") || "").toLowerCase() !== "websocket") {
    return request;
  }
  if (!isStreamPath(new URL(request.url).pathname)) {
    return request;
  }
  const headers = new Headers(request.headers);
  headers.delete("Upgrade");
  headers.delete("Connection");
  headers.delete("Sec-WebSocket-Key");
  headers.delete("Sec-WebSocket-Version");
  headers.delete("Sec-WebSocket-Protocol");
  headers.delete("Sec-WebSocket-Extensions");
  return new Request(request.url, { method: request.method, headers });
}

const socketPollIntervalMs = 200;
let socketsPolledAt = 0;

async function letSocketsBePolled(): Promise<void> {
  if (Date.now() - socketsPolledAt < socketPollIntervalMs) return;
  await new Promise((resolve) => setTimeout(resolve, 0));
  socketsPolledAt = Date.now();
}

export default {
  async fetch(incoming: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const request = withClientCert(incoming);
    const { pathname: path, hostname } = new URL(request.url);
    if (hostname === "k8flare.internal") await letSocketsBePolled();
    else socketsPolledAt = Date.now();
    console.log(`front iso=${isolateId()} ${request.method} ${path}`);
    if (path === "/v1-k3s/connect") return acceptTunnel(request, env);
    if (path === "/stats" && await isAdmin(request, env)) {
      return clusterStub(env).fetch("https://cluster.internal/stats");
    }
    if (path === "/vpc" && await isAdmin(request, env)) {
      const { vpcConnectAvailable } = await import("./vpc.ts");
      return Response.json({ vpc: await vpcConnectAvailable(env), tunnel: "nodetunnel" });
    }
    if (path === "/snapshot" && request.method === "POST" && await isAdmin(request, env)) {
      return clusterStub(env).fetch(new Request("https://cluster.internal/snapshot", { method: "POST" }));
    }
    if (path === "/restore" && request.method === "POST" && await isAdmin(request, env)) {
      const stub = clusterStub(env);
      const prepared = await stub.fetch(new Request(`https://cluster.internal/restore${new URL(request.url).search}`, { method: "POST" }));
      if (!prepared.ok) return prepared;
      try {
        await stub.fetch(new Request("https://cluster.internal/restore/apply", { method: "POST" }));
      } catch {}
      return prepared;
    }
    if (isNonWebSocketUpgrade(request)) {
      return refuseUpgradeAfterAdmission(() => {
        const headers = new Headers(request.headers);
        headers.delete("Upgrade");
        headers.delete("Connection");
        headers.set("X-K8flare-Stream-Locate", "1");
        return apiserverFetch(env, new Request(request.url, { method: request.method, headers }));
      });
    }
    if ((request.headers.get("Upgrade") || "").toLowerCase() === "websocket" && isStreamPath(path)) {
      const headers = new Headers(request.headers);
      headers.set("X-K8flare-Stream-Locate", "1");
      const located = await apiserverFetch(env, new Request(request, { headers }));
      if (!located.ok) return located;
      const loc = (await located.json()) as { node?: string; url?: string; protocol?: string; transport?: string };
      console.log(`stream path=${path}`);
      if (loc.node === VIRTUAL_NODE && loc.url) return servePodStream(env, ctx, { url: loc.url, protocol: loc.protocol });
      const pair = new WebSocketPair();
      const server = pair[1];
      server.accept();
      const early: Uint8Array[] = [];
      let sink: WebSocket | null = null;
      server.addEventListener("message", (ev) => {
        void bytesOf(ev.data).then((bytes) => {
          console.log(`stream-msg ${bytes.byteLength} sink=${sink ? 1 : 0} t=${typeof ev.data}`);
          if (bytes.byteLength === 0) return;
          if (sink) sendBinary(sink, bytes);
          else early.push(bytes);
        });
      });
      ctx.waitUntil((async () => {
        if (!loc.url || !loc.node) {
          try {
            server.close(1008, "pod is not scheduled");
          } catch {}
          return;
        }
        const dest = new URL(loc.url);
        const node = loc.node;
        if (!node) {
          try {
            server.close(1008, "pod is not scheduled");
          } catch {}
          return;
        }
        const headers = new Headers();
        if (loc.transport === "http") {
          // The kubelet serves logs as an ordinary read, so fetch the body and
          // relay it one way rather than dialling a socket that never opens.
          try {
            const stub = env.NODE_TUNNEL.get(env.NODE_TUNNEL.idFromName(tunnelName(env, node))) as DurableObjectStub<NodeTunnel>;
            const resp = await stub.fetch(dest.toString(), { headers });
            if (!resp.ok || !resp.body) {
              server.close(resp.ok ? 1011 : 1008, `logs ${resp.status}`);
              return;
            }
            const reader = resp.body.getReader();
            for (;;) {
              const { done, value } = await reader.read();
              if (done) break;
              if (value) sendLog(server, loc.protocol ?? "", value);
            }
            server.close(1000, "done");
          } catch (err) {
            try {
              server.close(1011, String(err).slice(0, 120));
            } catch {}
          }
          return;
        }
        headers.set("Upgrade", "websocket");
        headers.set("Connection", "Upgrade");
        if (loc.protocol) headers.set("Sec-WebSocket-Protocol", loc.protocol);
        if (dest.search) headers.set("X-Stream-Query", dest.search.slice(1));
        console.log(`stream in ${new URL(request.url).pathname}${new URL(request.url).search} tunnel ${dest.pathname} q=${dest.search.length}`);
        try {
          const stub = env.NODE_TUNNEL.get(env.NODE_TUNNEL.idFromName(tunnelName(env, node))) as DurableObjectStub<NodeTunnel>;
          const upstreamResp = await stub.fetch(dest.toString(), { headers });
          const upstream = upstreamResp.webSocket;
          if (!upstream) {
            try {
              server.close(1011, "no upstream socket");
            } catch {}
            return;
          }
          upstream.accept();
          sink = upstream;
          for (const bytes of early.splice(0)) sendBinary(upstream, bytes);
          upstream.addEventListener("message", (ev) => {
            void bytesOf(ev.data).then((bytes) => {
              if (bytes.byteLength === 0) return;
              sendBinary(server, bytes);
            });
          });
          await new Promise<void>((resolve) => {
            const finish = () => resolve();
            upstream.addEventListener("close", (ev) => {
              try {
                server.close(ev.code || 1000, ev.reason || "done");
              } catch {}
              finish();
            });
            upstream.addEventListener("error", () => {
              try {
                server.close(1011, "upstream error");
              } catch {}
              finish();
            });
            server.addEventListener("close", () => {
              try {
                upstream.close(1000, "done");
              } catch {}
              finish();
            });
            server.addEventListener("error", finish);
          });
        } catch (err) {
          try {
            server.close(1011, String(err).slice(0, 120));
          } catch {}
        }
      })());
      return streamUpgrade(pair[0], loc.protocol ?? "");
    }
    return apiserverFetch(env, asAPIRequest(request));
  },
  async queue(batch: MessageBatch<unknown>, env: Env): Promise<void> {
    await consume(batch as MessageBatch<QueueMessage>, env);
  },
  tail(events: TraceItem[]): void {
    for (const e of events) {
      if (e.event && "consumedEvents" in e.event) continue;
      console.log(`wasmcpu ${e.scriptName ?? "?"} ${e.entrypoint ?? "-"} ${e.outcome} ${e.cpuTime} ${e.wallTime}`);
      const where = e.event && "request" in e.event ? `${e.event.request.method} ${new URL(e.event.request.url).pathname}` : e.entrypoint ?? "?";
      for (const x of e.exceptions ?? []) console.log(`dynexc ${where} ${x.name}: ${x.message.slice(0, 200)}`);
      for (const l of e.logs ?? []) {
        for (const m of l.message ?? []) {
          if (typeof m !== "string") continue;
          if (m.includes('{"kind":"Event","apiVersion":"audit.k8s.io/v1"')) {
            for (const line of m.split("\n")) {
              if (line.startsWith('{"kind":"Event","apiVersion":"audit.k8s.io/v1"')) console.log(line);
            }
            continue;
          }
          const klog = /^[EW]\d{4} /.test(m);
          if (m.startsWith("bridge:") || m.startsWith("mem worker=") || m.startsWith("pods/status:") || m.startsWith("kine:") || m.startsWith("go program") || m.includes("panic") || m.startsWith("fatal error") || klog) {
            console.log(m.slice(0, 400));
          }
        }
      }
    }
  },
} satisfies ExportedHandler<Env>;
