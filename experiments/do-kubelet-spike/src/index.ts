import { DurableObject, WorkerEntrypoint } from "cloudflare:workers";

type Env = {
  POD: DurableObjectNamespace<PodSandbox>;
  INTERCEPT: Fetcher;
};

type StartRequest = {
  image?: string;
  imageName?: string;
  instance?: ContainerStartupOptions["instance"];
  entrypoint?: string[];
  env?: Record<string, string>;
  port: number;
  path?: string;
  snapshotId?: string;
  readyTimeoutMs?: number;
  directorySnapshots?: ContainerDirectorySnapshotRestoreParams[];
  inactivityMs?: number;
  skipReady?: boolean;
};

type Exit = { at: number; error?: string };

function errorText(err: unknown): string {
  return err instanceof Error ? `${err.name}: ${err.message}` : String(err);
}

async function waitForPort(container: Container, port: number, path: string, deadline: number) {
  let attempts = 0;
  let lastError = "";
  while (Date.now() < deadline) {
    attempts++;
    try {
      const res = await container.getTcpPort(port).fetch(`http://pod${path}`);
      return { ok: true, status: res.status, body: (await res.text()).slice(0, 200), attempts };
    } catch (err) {
      lastError = errorText(err);
      await new Promise((r) => setTimeout(r, 50));
    }
  }
  return { ok: false, attempts, lastError };
}

export class PodSandbox extends DurableObject<Env> {
  exit: Exit | undefined;

  images() {
    return this.ctx.container!.images;
  }

  status() {
    return { running: this.ctx.container!.running, exit: this.exit };
  }

  async inspect() {
    return await this.ctx.container!.inspect();
  }

  async start(req: StartRequest) {
    const container = this.ctx.container!;
    const image = req.imageName ? container.images[req.imageName] : req.image;
    const t0 = Date.now();
    this.exit = undefined;
    try {
      const base = JSON.parse(JSON.stringify({ enableInternet: true, instance: req.instance, entrypoint: req.entrypoint, env: req.env, directorySnapshots: req.directorySnapshots }));
      container.start(req.snapshotId ? { ...base, containerSnapshot: { id: req.snapshotId } } : { ...base, image: image! });
    } catch (err) {
      return { phase: "start", image, error: errorText(err), ms: Date.now() - t0 };
    }
    const startReturnedMs = Date.now() - t0;
    container.monitor().then(
      () => (this.exit = { at: Date.now() }),
      (err) => (this.exit = { at: Date.now(), error: errorText(err) }),
    );
    let inactivity: unknown;
    if (req.inactivityMs !== undefined) {
      try {
        await container.setInactivityTimeout(req.inactivityMs);
        inactivity = { ok: req.inactivityMs };
      } catch (err) {
        inactivity = { error: errorText(err) };
      }
    }
    if (req.skipReady) return { image, startReturnedMs, inactivity, running: container.running };
    const ready = await waitForPort(container, req.port, req.path ?? "/", t0 + (req.readyTimeoutMs ?? 60_000));
    return { image, startReturnedMs, readyMs: Date.now() - t0, ready, inactivity, exit: this.exit, running: container.running };
  }

  async fetchPort(port: number, path: string) {
    const res = await this.ctx.container!.getTcpPort(port).fetch(`http://pod${path}`);
    return { status: res.status, body: (await res.text()).slice(0, 500) };
  }

  async exec(cmd: string[], pty: boolean) {
    const t0 = Date.now();
    const proc = await this.ctx.container!.exec(cmd, pty ? { pty: true } : { stdout: "pipe", stderr: "pipe" });
    const out = await proc.output();
    const text = (b: ArrayBuffer) => new TextDecoder().decode(b);
    return { ms: Date.now() - t0, pid: proc.pid, isPty: proc.isPty, exitCode: out.exitCode, stdout: text(out.stdout), stderr: text(out.stderr) };
  }

  async intercept(kind: "http" | "https" | "tcp", addr: string) {
    const container = this.ctx.container!;
    if (kind === "tcp") await container.interceptOutboundTcp(addr, this.env.INTERCEPT);
    else if (kind === "https") await container.interceptOutboundHttps(addr, this.env.INTERCEPT);
    else await container.interceptOutboundHttp(addr, this.env.INTERCEPT);
    return { intercepted: addr, kind };
  }

  async setInactivity(ms: number) {
    await this.ctx.container!.setInactivityTimeout(ms);
    return { ok: ms };
  }

  async interceptAll() {
    await this.ctx.container!.interceptAllOutboundHttp(this.env.INTERCEPT);
    return { interceptedAll: true };
  }

  async tcpEcho(port: number, payload: string) {
    const t0 = Date.now();
    const socket = this.ctx.container!.getTcpPort(port).connect(`pod:${port}`);
    const writer = socket.writable.getWriter();
    await writer.write(new TextEncoder().encode(payload));
    const reader = socket.readable.getReader();
    const { value } = await reader.read();
    await socket.close();
    return { ms: Date.now() - t0, echoed: new TextDecoder().decode(value) };
  }

  async dirSnapshot(dir: string, name: string) {
    const t0 = Date.now();
    const snap = await this.ctx.container!.snapshotDirectory({ dir, name });
    return { ms: Date.now() - t0, snap };
  }

  hasTcpIntercept() {
    return { interceptOutboundTcp: typeof (this.ctx.container as any).interceptOutboundTcp, interceptOutboundHttps: typeof this.ctx.container!.interceptOutboundHttps };
  }

  async snapshot(name: string) {
    const t0 = Date.now();
    const snap = await this.ctx.container!.snapshotContainer({ name });
    return { ms: Date.now() - t0, snap };
  }

  signal(signo: number) {
    this.ctx.container!.signal(signo);
    return { signalled: signo };
  }

  async destroy() {
    await this.ctx.container!.destroy();
    return { destroyed: true };
  }
}

export class Intercept extends WorkerEntrypoint<Env> {
  async fetch(request: Request) {
    const url = new URL(request.url);
    const target = url.searchParams.get("pod") ?? request.headers.get("x-target-pod");
    if (target) {
      const res = await this.env.POD.getByName(target).fetchPort(80, url.pathname);
      return Response.json({ via: "intercept->pod", target, res });
    }
    if (url.searchParams.has("passthrough")) {
      const res = await fetch(request);
      return new Response(`passthrough ${res.status} ${(await res.text()).slice(0, 80)}`);
    }
    return Response.json({ via: "intercept", method: request.method, url: request.url, headers: Object.fromEntries(request.headers) });
  }
}

async function dispatch(env: Env, request: Request): Promise<unknown> {
  const url = new URL(request.url);
  const [, pods, id, action] = url.pathname.split("/");
  if (pods !== "pods" || !id) return { usage: "/pods/:id/{start,status,inspect,images,fetch,exec,intercept,snapshot,signal,destroy}" };
  const pod = env.POD.getByName(id);
  const body = request.method === "POST" ? await request.json<any>() : {};
  switch (action) {
    case "images":
      return await pod.images();
    case "status":
      return await pod.status();
    case "inspect":
      return await pod.inspect();
    case "start":
      return await pod.start(body);
    case "fetch":
      return await pod.fetchPort(Number(url.searchParams.get("port") ?? 80), url.searchParams.get("path") ?? "/");
    case "exec":
      return await pod.exec(body.cmd, Boolean(body.pty));
    case "intercept":
      return await pod.intercept(body.kind, body.addr);
    case "inactivity":
      return await pod.setInactivity(body.ms);
    case "interceptall":
      return await pod.interceptAll();
    case "tcpecho":
      return await pod.tcpEcho(body.port, body.payload ?? "ping");
    case "dirsnap":
      return await pod.dirSnapshot(body.dir, body.name ?? id);
    case "caps":
      return await pod.hasTcpIntercept();
    case "snapshot":
      return await pod.snapshot(body.name ?? id);
    case "signal":
      return await pod.signal(body.signo ?? 15);
    case "destroy":
      return await pod.destroy();
  }
  return { error: `unknown action ${action}` };
}

export default {
  async fetch(request: Request, env: Env) {
    if (request.headers.get("authorization") !== `Bearer ${(env as any).SPIKE_TOKEN}`) return new Response("unauthorized", { status: 401 });
    const t0 = Date.now();
    try {
      return Response.json({ totalMs: Date.now() - t0, result: await dispatch(env, request) });
    } catch (err) {
      return Response.json({ error: errorText(err), totalMs: Date.now() - t0 }, { status: 500 });
    }
  },
} satisfies ExportedHandler<Env>;
