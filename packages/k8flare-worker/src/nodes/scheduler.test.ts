// The cf-containers-scheduler's boot policy. A poke runs reconcile()
// detached (fetch() returns before it finishes), so the stub.up() call
// that starts a Pod's NodeVM can be abandoned with its caller's request
// -- in production that surfaces as a NodeVM entrypoint whose requests
// end `outcome=canceled` with no exception and no log, and an abandoned
// promise neither resolves nor rejects (S31 E2). The safety-net alarm is
// the un-cancellable context, so it must be able to re-issue a boot that
// the poke never landed; before this test it could not, and the Pod sat
// Unschedulable until NODE_READY_TIMEOUT_MS (5 minutes) reaped the
// claim.
import { describe, expect, it, vi } from "vite-plus/test";

vi.mock("cloudflare:workers", () => ({
  DurableObject: class {
    ctx: unknown;
    env: unknown;
    constructor(ctx: unknown, env: unknown) {
      this.ctx = ctx;
      this.env = env;
    }
  },
}));

const api = vi.hoisted(() => ({ pods: [] as Record<string, unknown>[] }));

vi.mock("../clusters/tokens.ts", () => ({
  clusterSecrets: () => Promise.resolve(["cluster-secret"]),
}));

vi.mock("../loader/apiserver.ts", () => ({
  apiserverFetch: (_env: unknown, request: Request) => {
    const url = new URL(request.url);
    if (url.pathname === "/api/v1/pods") return Promise.resolve(Response.json({ items: api.pods }));
    if (url.pathname.startsWith("/api/v1/nodes/")) {
      return Promise.resolve(new Response("no such node", { status: 404 }));
    }
    const pod = api.pods[0];
    if (url.pathname.startsWith("/api/v1/namespaces/") && pod) {
      return Promise.resolve(Response.json(pod));
    }
    return Promise.resolve(new Response(`unexpected ${url.pathname}`, { status: 500 }));
  },
}));

import { CFContainersScheduler } from "./scheduler.ts";

interface SchedulerInternals {
  fetch(request: Request): Promise<Response>;
  alarm(): Promise<void>;
}

class FakeStorage {
  kv = new Map<string, unknown>();
  alarm: number | null = null;

  setAlarm(ms: number): Promise<void> {
    this.alarm = ms;
    return Promise.resolve();
  }
  getAlarm(): Promise<number | null> {
    return Promise.resolve(this.alarm);
  }
  deleteAlarm(): Promise<void> {
    this.alarm = null;
    return Promise.resolve();
  }
  get<T>(key: string): Promise<T | undefined> {
    return Promise.resolve(this.kv.get(key) as T | undefined);
  }
  put(key: string, value: unknown): Promise<void> {
    this.kv.set(key, JSON.parse(JSON.stringify(value)) as unknown);
    return Promise.resolve();
  }
}

/** A NodeVM namespace whose up() can be abandoned exactly once. */
function fakeVMNamespace(abandonFirstBoot: boolean) {
  const boots: string[] = [];
  return {
    boots,
    binding: {
      idFromName: (name: string) => ({ name }),
      get: () => ({
        up: (nodeName: string) => {
          boots.push(nodeName);
          if (abandonFirstBoot && boots.length === 1) return new Promise<void>(() => {});
          return Promise.resolve();
        },
        destroyVM: () => Promise.resolve(),
      }),
    },
  };
}

function containersPod() {
  return {
    apiVersion: "v1",
    kind: "Pod",
    metadata: {
      name: "probe",
      namespace: "default",
      uid: "uid-1",
      annotations: { "k8flare.com/nodevm-tier": "small" },
    },
    spec: {
      nodeSelector: { "k8flare.com/backend": "containers", "kubernetes.io/hostname": "cf-probe1" },
      containers: [{ name: "c", image: "busybox" }],
    },
    status: { phase: "Pending" },
  };
}

function newScheduler(abandonFirstBoot: boolean) {
  const storage = new FakeStorage();
  const vm = fakeVMNamespace(abandonFirstBoot);
  const ctx = { id: { name: "default" }, storage };
  const env = { NODE_VM_SMALL: vm.binding, K3S_TOKEN: "cluster-secret" };
  const scheduler = new CFContainersScheduler(
    ctx as unknown as DurableObjectState,
    env as unknown as never,
  ) as unknown as SchedulerInternals;
  return { scheduler, storage, vm };
}

async function waitFor(what: string, done: () => boolean): Promise<void> {
  for (let i = 0; i < 500; i++) {
    if (done()) return;
    await new Promise((resolve) => setTimeout(resolve, 1));
  }
  throw new Error(`timed out waiting for ${what}`);
}

describe("NodeVM boot", () => {
  it("re-issues a boot the poke abandoned, on the next safety-net alarm", async () => {
    api.pods = [containersPod()];
    const { scheduler, vm } = newScheduler(true);

    await scheduler.fetch(new Request("http://nodes.internal/"));
    await waitFor("the poke's boot attempt", () => vm.boots.length === 1);

    await scheduler.alarm();
    expect(vm.boots).toEqual(["cf-probe1", "cf-probe1"]);
  });

  it("does not re-issue a boot that landed", async () => {
    api.pods = [containersPod()];
    const { scheduler, vm } = newScheduler(false);

    await scheduler.fetch(new Request("http://nodes.internal/"));
    await waitFor("the poke's boot attempt", () => vm.boots.length === 1);

    await scheduler.alarm();
    await scheduler.alarm();
    expect(vm.boots).toEqual(["cf-probe1"]);
  });
});
