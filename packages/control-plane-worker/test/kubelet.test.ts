import assert from "node:assert/strict";
import { test } from "node:test";
import { KEEPALIVE_INTERVAL_MS, PodKubelet } from "../src/podkubelet/kubelet.ts";
import type { Pod } from "../src/podkubelet/spec.ts";

interface Deferred {
  resolve(): void;
  reject(err: unknown): void;
}

async function readText(stream: ReadableStream<Uint8Array>): Promise<string> {
  const reader = stream.getReader();
  let out = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) return out;
    out += new TextDecoder().decode(value);
  }
}

class FakeProcess {
  readonly cmd: string[];
  readonly options: ContainerExecOptions;
  readonly stdin: WritableStream | null = null;
  readonly stdout: ReadableStream<Uint8Array> | null;
  readonly stderr: ReadableStream<Uint8Array> | null = null;
  readonly pid = 42;
  readonly isPty: boolean;
  readonly exitCode: Promise<number>;
  readonly stdinText: Promise<string> | null;
  resizes: Array<[number, number]> = [];
  killed: number[] = [];
  private exit!: (code: number) => void;
  private closeStdout = () => {};

  constructor(cmd: string[], options: ContainerExecOptions, defaultExit: number) {
    this.cmd = cmd;
    this.options = options;
    this.isPty = Boolean(options.pty);
    this.exitCode = new Promise<number>((resolve) => {
      this.exit = resolve;
    });
    this.stdinText = options.stdin instanceof ReadableStream ? readText(options.stdin) : null;
    this.stdout =
      options.stdout === "pipe"
        ? new ReadableStream<Uint8Array>({
            start: (c) => {
              c.enqueue(new TextEncoder().encode(cmd.join(" ")));
              this.closeStdout = () => c.close();
            },
          })
        : null;
    this.defaultExit = defaultExit;
  }

  private readonly defaultExit: number;

  finish(code: number): void {
    this.closeStdout();
    this.exit(code);
  }

  async output() {
    this.finish(this.defaultExit);
    return { exitCode: await this.exitCode, stdout: new TextEncoder().encode(this.cmd.join(" ")).buffer as ArrayBuffer, stderr: new ArrayBuffer(0) };
  }

  kill(signal = 15): void {
    this.killed.push(signal);
    this.finish(128 + signal);
  }

  resize(cols: number, rows: number): void {
    this.resizes.push([cols, rows]);
  }
}

class FakeContainer {
  running = false;
  starts: ContainerStartupOptions[] = [];
  signals: number[] = [];
  destroyed = 0;
  intercepts: string[] = [];
  inactivity: number[] = [];
  exits: Deferred[] = [];
  images: Record<string, string> = { httpd: "registry.cloudflare.com/acct/httpd@sha256:deadbeef" };
  probeStatus = 200;
  execExit = 0;
  served: Array<{ port: number; url: string; host: string | null }> = [];
  sockets: Array<{ port: number; received: Promise<string>; close: () => void }> = [];
  processes: FakeProcess[] = [];

  start(options: ContainerStartupOptions): void {
    this.starts.push(options);
    this.running = true;
  }

  monitor(): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      this.exits.push({
        resolve: () => {
          this.running = false;
          resolve();
        },
        reject: (err) => {
          this.running = false;
          reject(err);
        },
      });
    });
  }

  signal(signo: number): void {
    this.signals.push(signo);
  }

  async destroy(): Promise<void> {
    this.destroyed++;
    this.running = false;
  }

  getTcpPort(port: number) {
    return {
      fetch: async (input: string | Request) => {
        const request = typeof input === "string" ? new Request(input) : input;
        this.served.push({ port, url: request.url, host: request.headers.get("Host") });
        return new Response(`hi from ${port}`, { status: this.probeStatus });
      },
      connect: () => {
        const input = new TransformStream<Uint8Array, Uint8Array>();
        const received = readText(input.readable);
        let closeOutput = () => {};
        const readable = new ReadableStream<Uint8Array>({
          start: (c) => {
            c.enqueue(new TextEncoder().encode(`echo ${port}`));
            closeOutput = () => c.close();
          },
        });
        this.sockets.push({ port, received, close: () => closeOutput() });
        return { opened: Promise.resolve({}), readable, writable: input.writable, close: async () => closeOutput() };
      },
      port,
    };
  }

  async exec(cmd: string[], options: ContainerExecOptions = {}) {
    const proc = new FakeProcess(cmd, options, this.execExit);
    this.processes.push(proc);
    return proc;
  }

  async setInactivityTimeout(ms: number): Promise<void> {
    this.inactivity.push(ms);
  }

  async interceptAllOutboundHttp(): Promise<void> {
    this.intercepts.push("all-http");
  }

  async interceptOutboundHttps(addr: string): Promise<void> {
    this.intercepts.push(`https:${addr}`);
  }
}

class FakeStorage {
  data = new Map<string, unknown>();
  alarm: number | null = null;

  async get<T>(key: string): Promise<T | undefined> {
    return structuredClone(this.data.get(key)) as T | undefined;
  }

  async put(key: string, value: unknown): Promise<void> {
    this.data.set(key, structuredClone(value));
  }

  async deleteAll(): Promise<void> {
    this.data.clear();
  }

  async setAlarm(at: number): Promise<void> {
    this.alarm = at;
  }

  async deleteAlarm(): Promise<void> {
    this.alarm = null;
  }
}

class FakeLedger {
  released: string[] = [];
  running: Array<[string, boolean]> = [];

  async claim(uid: string, namespace: string, name: string, cidr: string) {
    return { uid, namespace, name, podIP: cidr.startsWith("10.42.255.") ? "10.42.255.2" : "10.42.0.2", running: false, updatedAt: 0 };
  }

  async setRunning(uid: string, running: boolean) {
    this.running.push([uid, running]);
  }

  async release(uid: string) {
    this.released.push(uid);
  }

  async byIP(podIP: string) {
    return podIP === "10.42.255.9" ? { uid: "uid-9", namespace: "default", name: "peer", podIP, running: true, updatedAt: 0 } : null;
  }
}

class FakeAPIServer {
  pod: Pod | null;
  node: Record<string, unknown> | null = null;
  configMaps: Record<string, Record<string, string>> = {};
  events: Array<{ type: string; reason: string; message: string }> = [];
  statuses: Pod["status"][] = [];
  deletes: Array<{ name: string; body: Record<string, unknown> }> = [];
  tokens = 0;
  requests: Array<{ method: string; path: string; auth: string | null }> = [];
  services: Record<string, Record<string, unknown>> = {};
  slices: Record<string, unknown>[] = [];

  constructor(pod: Pod) {
    this.pod = pod;
  }

  async handle(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname;
    this.requests.push({ method: request.method, path, auth: request.headers.get("Authorization") });
    if (request.method === "GET" && /\/pods\/[^/]+$/.test(path)) return this.pod ? Response.json(this.pod) : new Response("nf", { status: 404 });
    if (request.method === "GET" && path.startsWith("/api/v1/nodes/")) return this.node ? Response.json(this.node) : new Response("nf", { status: 404 });
    if (request.method === "GET" && path.includes("/configmaps/")) {
      const cm = this.configMaps[path.split("/").pop()!];
      return cm ? Response.json({ data: cm }) : new Response("nf", { status: 404 });
    }
    if (request.method === "GET" && path.includes("/secrets/")) return new Response("nf", { status: 404 });
    if (request.method === "GET" && /\/services\/[^/]+$/.test(path)) {
      const svc = this.services[path.split("/").pop()!];
      return svc ? Response.json(svc) : new Response("nf", { status: 404 });
    }
    if (request.method === "GET" && path === "/api/v1/services") {
      const ip = url.searchParams.get("fieldSelector")?.replace("spec.clusterIP=", "");
      return Response.json({ kind: "ServiceList", items: Object.values(this.services).filter((s) => (s.spec as { clusterIP?: string }).clusterIP === ip) });
    }
    if (request.method === "GET" && path.endsWith("/endpointslices")) {
      const name = url.searchParams.get("labelSelector")?.replace("kubernetes.io/service-name=", "");
      return Response.json({ kind: "EndpointSliceList", items: this.slices.filter((s) => (s.metadata as { labels: Record<string, string> }).labels["kubernetes.io/service-name"] === name) });
    }
    if (request.method === "PUT" && path.endsWith("/status")) {
      if (!this.pod) return new Response("nf", { status: 404 });
      const body = (await request.json()) as Pod;
      this.pod = { ...this.pod, status: body.status };
      this.statuses.push(body.status);
      return Response.json(this.pod);
    }
    if (request.method === "DELETE") {
      this.deletes.push({ name: path.split("/").pop()!, body: (await request.json()) as Record<string, unknown> });
      this.pod = null;
      return Response.json({ kind: "Status", status: "Success" });
    }
    if (request.method === "POST" && path.endsWith("/events")) {
      const ev = (await request.json()) as { type: string; reason: string; message: string };
      this.events.push({ type: ev.type, reason: ev.reason, message: ev.message });
      return Response.json(ev, { status: 201 });
    }
    if (request.method === "POST" && path.endsWith("/token")) {
      this.tokens++;
      return Response.json({ status: { token: `sa-token-${this.tokens}`, expirationTimestamp: new Date(Date.now() + 3600_000).toISOString() } });
    }
    if (path === "/api/v1/namespaces/default/pods" || path.startsWith("/api/")) return Response.json({ kind: "PodList", items: [] });
    return new Response("unexpected", { status: 500 });
  }
}

function makePod(overrides: Partial<Pod["spec"]> = {}, metadata: Partial<Pod["metadata"]> = {}): Pod {
  return {
    apiVersion: "v1",
    kind: "Pod",
    metadata: { name: "web", namespace: "default", uid: "uid-1", annotations: { "containers.k8flare.com/image": "images/httpd", "containers.k8flare.com/instance": "lite" }, ...metadata },
    spec: { nodeName: "cloudflare", serviceAccountName: "web-sa", containers: [{ name: "app", image: "images/httpd", env: [{ name: "GREETING", valueFrom: { configMapKeyRef: { name: "cm", key: "hello" } } }] }], ...overrides },
  };
}

async function harness(pod: Pod) {
  const container = new FakeContainer();
  const storage = new FakeStorage();
  const ledger = new FakeLedger();
  const api = new FakeAPIServer(pod);
  api.configMaps.cm = { hello: "world" };
  const waits: Promise<unknown>[] = [];
  const peers: Array<{ uid: string; port: number; url: string; host: string | null }> = [];
  const ctx = {
    id: { name: pod.metadata.uid },
    storage,
    container,
    blockConcurrencyWhile: (fn: () => Promise<void>) => fn(),
    waitUntil: (p: Promise<unknown>) => waits.push(p),
  };
  const env = {
    ADMIN_TOKEN: "admin",
    CLUSTER_UID: "",
    POD_KUBELET: {
      idFromName: (n: string) => n,
      get: (id: unknown) => ({
        fetch: async () => new Response("self"),
        ingress: async (port: number, request: Request) => {
          peers.push({ uid: String(id), port, url: request.url, host: request.headers.get("Host") });
          return new Response(`peer ${id}:${port}`);
        },
      }),
    },
    POD_LEDGER: { idFromName: (n: string) => n, get: () => ledger },
    __apiserver: (request: Request) => api.handle(request),
  };
  const kubelet = new PodKubelet(ctx as unknown as DurableObjectState, env as unknown as Env);
  await new Promise((r) => setImmediate(r));
  const settle = async () => {
    while (waits.length > 0) await waits.splice(0).reduce((p, w) => p.then(() => w), Promise.resolve() as Promise<unknown>);
  };
  const exit = async (err?: unknown) => {
    const d = container.exits.shift()!;
    if (err === undefined) d.resolve();
    else d.reject(err);
    await new Promise((r) => setTimeout(r, 0));
    await settle();
  };
  return { container, storage, ledger, api, kubelet, settle, exit, peers };
}

function endpointSlice(service: string, endpoints: Array<Record<string, unknown>>, port = 8080, name = "http"): Record<string, unknown> {
  return { metadata: { name: `${service}-x`, namespace: "default", labels: { "kubernetes.io/service-name": service } }, addressType: "IPv4", endpoints, ports: [{ name, port }] };
}

const latest = <T>(items: T[]): T => items[items.length - 1];

test("a bound Pod starts its declared image with the resolved env and reports Running/Ready", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  assert.equal(h.container.starts.length, 1);
  const start = h.container.starts[0];
  assert.equal(start.image, "registry.cloudflare.com/acct/httpd@sha256:deadbeef");
  assert.deepEqual(start.entrypoint, ["httpd", "-f", "-p", "8080", "-h", "/www"]);
  assert.equal(start.instance, "lite");
  assert.equal(start.enableInternet, true);
  assert.equal(start.env?.GREETING, "world");
  assert.equal(start.env?.KUBERNETES_SERVICE_HOST, "kubernetes.default.svc");
  assert.equal(start.env?.SSL_CERT_FILE, "/etc/cloudflare/certs/cloudflare-containers-ca.crt");
  assert.deepEqual(h.container.intercepts, ["all-http", "https:kubernetes.default.svc:443"]);
  assert.deepEqual(h.container.inactivity, [6 * 3600_000]);
  const status = latest(h.api.statuses)!;
  assert.equal(status.phase, "Running");
  assert.equal(status.podIP, "10.42.255.2");
  assert.equal(status.hostIP, "10.42.255.1");
  assert.equal(status.containerStatuses?.[0].ready, true);
  assert.equal(status.containerStatuses?.[0].state.running !== undefined, true);
  assert.equal(status.conditions?.find((c) => c.type === "Ready")?.status, "True");
  assert.deepEqual(h.api.events.map((e) => e.reason), ["Started"]);
  assert.deepEqual(h.ledger.running, [["uid-1", true]]);
  assert.ok(h.storage.alarm !== null && h.storage.alarm <= Date.now() + KEEPALIVE_INTERVAL_MS);
  assert.equal(h.api.requests[0].auth?.startsWith("Bearer component:podkubelet:"), true);
});

test("crashes restart under CrashLoopBackOff and the alarm restarts the container", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  await h.exit(new Error("container exited with code 1"));
  assert.equal(h.container.starts.length, 2);
  let status = latest(h.api.statuses)!;
  assert.equal(status.containerStatuses?.[0].restartCount, 1);
  assert.equal(status.containerStatuses?.[0].lastState.terminated?.exitCode, 1);
  assert.equal(status.containerStatuses?.[0].lastState.terminated?.reason, "Error");
  await h.exit(new Error("container exited with code 1"));
  assert.equal(h.container.starts.length, 2);
  status = latest(h.api.statuses)!;
  assert.equal(status.phase, "Running");
  assert.equal(status.containerStatuses?.[0].state.waiting?.reason, "CrashLoopBackOff");
  assert.match(status.containerStatuses?.[0].state.waiting?.message ?? "", /back-off 10s restarting failed container=app/);
  assert.equal(status.containerStatuses?.[0].ready, false);
  assert.equal(latest(h.api.events).reason, "BackOff");
  assert.ok(h.storage.alarm !== null && h.storage.alarm > Date.now() + 9_000);
  const realNow = Date.now;
  Date.now = () => realNow() + 11_000;
  try {
    await h.kubelet.alarm();
  } finally {
    Date.now = realNow;
  }
  assert.equal(h.container.starts.length, 3);
  assert.equal(latest(h.api.statuses)!.containerStatuses?.[0].restartCount, 2);
});

test("restartPolicy Never ends the Pod on exit and frees the container", async () => {
  const h = await harness(makePod({ restartPolicy: "Never" }));
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  await h.exit();
  assert.equal(h.container.destroyed, 1);
  const status = latest(h.api.statuses)!;
  assert.equal(status.phase, "Succeeded");
  assert.equal(status.containerStatuses?.[0].state.terminated?.exitCode, 0);
  assert.equal(status.containerStatuses?.[0].state.terminated?.reason, "Completed");
  assert.equal(status.conditions?.find((c) => c.type === "Ready")?.reason, "PodCompleted");
  assert.equal(h.storage.alarm, null);
});

test("a platform validation failure is terminal: waiting reason, Warning Event, no retry alarm", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  await h.exit(new Error("Invalid custom instance: memoryMib must be at least 3072 per vcpu"));
  assert.equal(h.container.starts.length, 1);
  const status = latest(h.api.statuses)!;
  assert.equal(status.phase, "Pending");
  assert.equal(status.containerStatuses?.[0].state.waiting?.reason, "CreateContainerError");
  assert.equal(latest(h.api.events).type, "Warning");
  assert.equal(h.storage.alarm, null);
});

test("a capacity failure retries on an alarm with backoff after a Warning Event", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  await h.exit(new Error("There is no container instance that can be provided at this time"));
  assert.equal(latest(h.api.events).reason, "FailedCreatePodSandBox");
  assert.equal(latest(h.api.statuses)!.containerStatuses?.[0].state.waiting?.reason, "ContainerCreating");
  assert.ok(h.storage.alarm !== null && h.storage.alarm > Date.now() + 4_000);
  const realNow = Date.now;
  Date.now = () => realNow() + 6_000;
  try {
    await h.kubelet.alarm();
  } finally {
    Date.now = realNow;
  }
  assert.equal(h.container.starts.length, 2);
});

test("missing env sources fail the container with CreateContainerConfigError", async () => {
  const h = await harness(makePod());
  delete h.api.configMaps.cm;
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  assert.equal(h.container.starts.length, 0);
  const status = latest(h.api.statuses)!;
  assert.equal(status.containerStatuses?.[0].state.waiting?.reason, "CreateContainerConfigError");
  assert.match(status.containerStatuses?.[0].state.waiting?.message ?? "", /configmap "cm" not found/);
});

test("deletion sends SIGTERM, waits the grace period for SIGKILL, then removes the Pod with grace 0", async () => {
  const h = await harness(makePod({ terminationGracePeriodSeconds: 5 }));
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  h.api.pod = { ...h.api.pod!, metadata: { ...h.api.pod!.metadata, deletionTimestamp: new Date().toISOString(), deletionGracePeriodSeconds: 5 } };
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  assert.deepEqual(h.container.signals, [15]);
  assert.equal(latest(h.api.events).reason, "Killing");
  assert.ok(h.storage.alarm !== null && h.storage.alarm >= Date.now() + 4_000);
  const realNow = Date.now;
  Date.now = () => realNow() + 6_000;
  try {
    await h.kubelet.alarm();
  } finally {
    Date.now = realNow;
  }
  assert.deepEqual(h.container.signals, [15, 9]);
  await h.exit(new Error("container exited with code 137"));
  assert.equal(h.container.destroyed, 1);
  assert.equal(latest(h.api.statuses)!.phase, "Failed");
  assert.deepEqual(h.api.deletes, [{ name: "web", body: { apiVersion: "v1", kind: "DeleteOptions", gracePeriodSeconds: 0, preconditions: { uid: "uid-1" } } }]);
  assert.deepEqual(h.ledger.released, ["uid-1"]);
  assert.equal(h.storage.data.size, 0);
});

test("a Pod that vanished destroys its container and releases its ledger entry", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  h.api.pod = null;
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  assert.equal(h.container.destroyed, 1);
  assert.deepEqual(h.ledger.released, ["uid-1"]);
  assert.equal(h.storage.data.size, 0);
});

test("probes gate readiness and a failed liveness probe restarts the container", async () => {
  const pod = makePod();
  pod.spec.containers[0].readinessProbe = { httpGet: { path: "/healthz", port: 8080 }, periodSeconds: 1, failureThreshold: 1 };
  pod.spec.containers[0].livenessProbe = { exec: { command: ["true"] }, periodSeconds: 1, failureThreshold: 2 };
  const h = await harness(pod);
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  assert.equal(latest(h.api.statuses)!.containerStatuses?.[0].ready, false);
  await h.kubelet.alarm();
  assert.equal(latest(h.api.statuses)!.containerStatuses?.[0].ready, true);
  h.container.probeStatus = 500;
  h.container.execExit = 1;
  const realNow = Date.now;
  let offset = 0;
  Date.now = () => realNow() + offset;
  try {
    offset = 1_100;
    await h.kubelet.alarm();
    assert.equal(latest(h.api.statuses)!.containerStatuses?.[0].ready, false);
    assert.equal(h.api.events.some((e) => e.reason === "Unhealthy" && /Readiness probe failed: HTTP probe failed with statuscode: 500/.test(e.message)), true);
    assert.deepEqual(h.container.signals, []);
    offset = 2_200;
    await h.kubelet.alarm();
  } finally {
    Date.now = realNow;
  }
  assert.deepEqual(h.container.signals, [15]);
  assert.equal(h.api.events.some((e) => e.reason === "Killing" && /failed liveness probe/.test(e.message)), true);
  await h.exit(new Error("container exited with code 143"));
  assert.equal(h.container.starts.length, 2);
  assert.equal(latest(h.api.statuses)!.containerStatuses?.[0].lastState.terminated?.message, "liveness probe failed");
});

test("outbound requests to the API server carry a bound ServiceAccount token", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  const before = h.api.requests.length;
  const res = await h.kubelet.fetch(new Request("https://kubernetes.default.svc/api/v1/namespaces/default/pods"));
  assert.equal(res.status, 200);
  const tokenRequest = h.api.requests.slice(before).find((r) => r.path.endsWith("/serviceaccounts/web-sa/token"));
  assert.ok(tokenRequest);
  const apiCall = latest(h.api.requests);
  assert.equal(apiCall.path, "/api/v1/namespaces/default/pods");
  assert.equal(apiCall.auth, "Bearer sa-token-1");
  await h.kubelet.fetch(new Request("https://kubernetes.default.svc/api/v1/nodes"));
  assert.equal(h.api.tokens, 1);
});

test("egress to a Service name or ClusterIP reaches the endpoint Pod's Durable Object on the target port", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  h.api.services.api = { metadata: { name: "api", namespace: "default" }, spec: { clusterIP: "10.43.0.20", ports: [{ name: "http", port: 80, targetPort: 8080 }] } };
  h.api.slices.push(endpointSlice("api", [{ addresses: ["10.42.255.9"], conditions: { ready: true }, nodeName: "cloudflare", targetRef: { kind: "Pod", namespace: "default", name: "peer", uid: "uid-9" } }]));
  const byName = await h.kubelet.fetch(new Request("http://api.default.svc.cluster.local/v1/items", { headers: { Host: "api.default.svc.cluster.local" } }));
  assert.equal(await byName.text(), "peer uid-9:8080");
  assert.deepEqual(h.peers, [{ uid: "uid-9", port: 8080, url: "http://api.default.svc.cluster.local/v1/items", host: "api.default.svc.cluster.local" }]);
  const byIP = await h.kubelet.fetch(new Request("http://10.43.0.20/v1/items"));
  assert.equal(await byIP.text(), "peer uid-9:8080");
  const byPodIP = await h.kubelet.fetch(new Request("http://10.42.255.9:8080/direct"));
  assert.equal(await byPodIP.text(), "peer uid-9:8080");
  assert.equal(h.peers.length, 3);
});

test("egress to a Service whose endpoints are on a real node is a 502 that names the node; a Service to itself is served locally", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  h.api.services.db = { metadata: { name: "db", namespace: "default" }, spec: { clusterIP: "10.43.0.30", ports: [{ name: "pg", port: 5432 }] } };
  h.api.slices.push(endpointSlice("db", [{ addresses: ["10.42.0.4"], conditions: { ready: true }, nodeName: "node-a", targetRef: { kind: "Pod", namespace: "default", name: "db-0", uid: "uid-db" } }], 5432, "pg"));
  const res = await h.kubelet.fetch(new Request("http://db.default.svc:5432/"));
  assert.equal(res.status, 502);
  assert.match(await res.text(), /endpoints of service default\/db are on node-a/);
  h.api.services.self = { metadata: { name: "self", namespace: "default" }, spec: { clusterIP: "10.43.0.40", ports: [{ name: "http", port: 80 }] } };
  h.api.slices.push(endpointSlice("self", [{ addresses: ["10.42.255.2"], conditions: { ready: true }, nodeName: "cloudflare", targetRef: { kind: "Pod", namespace: "default", name: "web", uid: "uid-1" } }]));
  const own = await h.kubelet.fetch(new Request("http://self/healthz"));
  assert.equal(await own.text(), "hi from 8080");
  assert.deepEqual(h.container.served, [{ port: 8080, url: "http://self/healthz", host: null }]);
  assert.equal(h.peers.length, 0);
});

test("ingress forwards to the container port while running and is 503 otherwise", async () => {
  const h = await harness(makePod());
  const early = await h.kubelet.ingress(8080, new Request("http://10.42.255.2:8080/"));
  assert.equal(early.status, 503);
  assert.match(await early.text(), /has no running container on cloudflare/);
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  const res = await h.kubelet.ingress(8080, new Request("http://10.42.255.2:8080/index.html", { headers: { Host: "web.example.com" } }));
  assert.equal(await res.text(), "hi from 8080");
  assert.deepEqual(h.container.served, [{ port: 8080, url: "http://10.42.255.2:8080/index.html", host: "web.example.com" }]);
});

test("exec runs the command with piped stdio, relays resizes from the control stream and ends with the exit Status", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  const stdin = new TransformStream<Uint8Array, Uint8Array>();
  const control = new TransformStream<Uint8Array, Uint8Array>();
  const streams = await h.kubelet.exec(["sh"], { stdin: stdin.readable, stdout: true, stderr: false, tty: true, cols: 80, rows: 24, control: control.readable });
  const proc = h.container.processes[0];
  assert.deepEqual(proc.cmd, ["sh"]);
  assert.deepEqual(proc.options.pty, { cols: 80, rows: 24 });
  assert.equal(proc.options.stdout, "pipe");
  assert.equal(proc.options.stderr, "ignore");
  const controlWriter = control.writable.getWriter();
  await controlWriter.write(new TextEncoder().encode('{"Width":120,"Height":40}'));
  const stdinWriter = stdin.writable.getWriter();
  await stdinWriter.write(new TextEncoder().encode("exit 3\n"));
  await stdinWriter.close();
  await new Promise((r) => setTimeout(r, 5));
  assert.deepEqual(proc.resizes, [[120, 40]]);
  assert.equal(await proc.stdinText, "exit 3\n");
  proc.finish(3);
  assert.equal(await readText(streams.stdout!), "sh");
  const status = JSON.parse(await readText(streams.status));
  assert.equal(status.status, "Failure");
  assert.deepEqual(status.details, { causes: [{ reason: "ExitCode", message: "3" }] });
  await controlWriter.close();
  await h.settle();
  assert.deepEqual(proc.killed, []);
});

test("a client that goes away before the command exits gets it SIGTERMed; exec without a running container is refused", async () => {
  const h = await harness(makePod());
  await assert.rejects(h.kubelet.exec(["ls"], { stdin: null, stdout: true, stderr: true, tty: false, control: new ReadableStream() }), /has no running container on cloudflare/);
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  const control = new TransformStream<Uint8Array, Uint8Array>();
  await h.kubelet.exec(["sleep", "100"], { stdin: null, stdout: true, stderr: true, tty: false, control: control.readable });
  await control.writable.close();
  await h.settle();
  assert.deepEqual(h.container.processes[0].killed, [15]);
});

test("connectPort pipes the client bytes into the container port and returns what the port writes back", async () => {
  const h = await harness(makePod());
  await h.kubelet.reconcile({ namespace: "default", name: "web" });
  const input = new TransformStream<Uint8Array, Uint8Array>();
  const readable = await h.kubelet.connectPort(8080, input.readable);
  const writer = input.writable.getWriter();
  await writer.write(new TextEncoder().encode("GET / HTTP/1.0\r\n\r\n"));
  await writer.close();
  const socket = h.container.sockets[0];
  assert.equal(socket.port, 8080);
  assert.equal(await socket.received, "GET / HTTP/1.0\r\n\r\n");
  socket.close();
  assert.equal(await readText(readable), "echo 8080");
});
