import assert from "node:assert/strict";
import { test } from "node:test";
import { KEEPALIVE_INTERVAL_MS, PodKubelet } from "../src/podkubelet/kubelet.ts";
import type { Pod } from "../src/podkubelet/spec.ts";

interface Deferred {
  resolve(): void;
  reject(err: unknown): void;
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
      fetch: async () => new Response("hi", { status: this.probeStatus }),
      connect: () => ({ opened: Promise.resolve(), close: async () => {} }),
      port,
    };
  }

  async exec(cmd: string[]) {
    return { output: async () => ({ exitCode: this.execExit, stdout: new TextEncoder().encode(cmd.join(" ")).buffer, stderr: new ArrayBuffer(0) }), kill() {} };
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
    POD_KUBELET: { idFromName: (n: string) => n, get: () => ({ fetch: async () => new Response("self") }) },
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
  return { container, storage, ledger, api, kubelet, settle, exit };
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
