// The Controllers DO's poke/park policy. Every branch here is a cost
// invariant with a measured regression behind it: an alarm that re-armed
// itself on an empty cluster (94 firings / 25 idle minutes, 2026-07-26), a
// node-less cluster that parked with a Deployment still unreconciled, a
// CronJob whose wall-clock fire was dropped because nothing wakes for it
// (S27), and a foreground cascade that parked mid-delete (S36).
import { describe, expect, it, vi } from "vite-plus/test";

vi.mock("cloudflare:workers", () => ({ DurableObject: class {} }));

import { Controllers } from "./index.ts";

const SAFETY_NET_INTERVAL_MS = 60_000;

interface ControllersInternals {
  hasUnconvergedWork(): Promise<boolean>;
  alarm(): Promise<void>;
  fetch(request: Request): Promise<Response>;
  loadComponent(name: string): Promise<unknown>;
}

/** What the gateway answers for each path apiGet asks about. */
interface ApiRoutes {
  deployments?: unknown[];
  replicasets?: unknown[];
  jobs?: unknown[];
  replicationcontrollers?: unknown[];
  clusters?: unknown[];
  pendingDeletions?: number;
  nextCronSchedule?: { nextScheduleTime?: string; overdue?: boolean };
  failing?: boolean;
}

class FakeStorage {
  kv = new Map<string, unknown>();
  alarm: number | null = null;
  alarmSets: number[] = [];

  setAlarm(ms: number): void {
    this.alarm = ms;
    this.alarmSets.push(ms);
  }
  getAlarm(): Promise<number | null> {
    return Promise.resolve(this.alarm);
  }
  deleteAlarm(): Promise<void> {
    this.alarm = null;
    return Promise.resolve();
  }
  deleteAll(): Promise<void> {
    this.kv.clear();
    return Promise.resolve();
  }
  get<T>(key: string): Promise<T | undefined> {
    return Promise.resolve(this.kv.get(key) as T | undefined);
  }
  put(key: string, value: unknown): Promise<void> {
    this.kv.set(key, value);
    return Promise.resolve();
  }
}

function newControllers(
  routes: ApiRoutes = {},
  env: Record<string, unknown> = {},
  clusterName = "default",
) {
  const storage = new FakeStorage();
  const apiCalls: string[] = [];
  const list = (items: unknown[] | undefined) => ({ items: items ?? [] });
  const self = {
    fetch: (url: string) => {
      const path = new URL(url).pathname;
      apiCalls.push(path);
      if (routes.failing) return Promise.resolve(new Response("boom", { status: 500 }));
      if (path.endsWith("/deployments"))
        return Promise.resolve(Response.json(list(routes.deployments)));
      if (path.endsWith("/replicasets"))
        return Promise.resolve(Response.json(list(routes.replicasets)));
      if (path.endsWith("/jobs")) return Promise.resolve(Response.json(list(routes.jobs)));
      if (path.endsWith("/replicationcontrollers"))
        return Promise.resolve(Response.json(list(routes.replicationcontrollers)));
      if (path.endsWith("/clusters")) return Promise.resolve(Response.json(list(routes.clusters)));
      if (path.endsWith("/internal/pending-deletions"))
        return Promise.resolve(Response.json({ pending: routes.pendingDeletions ?? 0 }));
      if (path.endsWith("/internal/next-cron-schedule"))
        return Promise.resolve(Response.json(routes.nextCronSchedule ?? {}));
      return Promise.resolve(new Response("not found", { status: 404 }));
    },
  };
  // No manifest in ASSETS = component not shipped, which every load path
  // already treats as absent. That keeps these tests off the Loader.
  const assets = { fetch: () => Promise.resolve(new Response("nope", { status: 404 })) };
  const cluster = {
    idFromName: (name: string) => ({ name }),
    get: () => ({ fetch: () => Promise.resolve(Response.json({ revision: 1, kv: null })) }),
  };
  const state = { id: { name: clusterName }, storage };
  const fullEnv = {
    SELF: self,
    ASSETS: assets,
    CLUSTER: cluster,
    K3S_TOKEN: "k8flare-dev-token",
    CLUSTEROP_DISABLED: "1",
    ...env,
  };
  const controllers = new Controllers(
    state as unknown as DurableObjectState,
    fullEnv as never,
  ) as unknown as ControllersInternals;
  return { controllers, storage, apiCalls };
}

// The dynamic workers' trace relay is a cost-sensitive binding (invariant
// #8): attaching `tails` makes every dynamic-worker invocation also invoke
// this script's tail handler, which is billed. It must appear only when an
// operator asked to measure.
function loaderProbe() {
  const codes: Promise<Record<string, unknown>>[] = [];
  const manifest = { size: 0, sha256: "0".repeat(64), parts: [] as string[] };
  const assets = {
    fetch: (url: string) =>
      Promise.resolve(
        new URL(url).pathname.endsWith(".manifest.json")
          ? Response.json(manifest)
          : new Response("// wasm_exec"),
      ),
  };
  // The real WorkerLoader.get is synchronous and runs the factory lazily,
  // so this must not be async or the caller gets a Promise where it
  // expects a stub.
  const loader = {
    get: (_id: string, factory: () => Promise<Record<string, unknown>>) => {
      codes.push(factory());
      return { getEntrypoint: () => ({ fetch: () => Promise.resolve(new Response("{}")) }) };
    },
  };
  return { codes, assets, loader };
}

async function loadedWorkerCode(env: Record<string, unknown>) {
  const { codes, assets, loader } = loaderProbe();
  const { controllers } = newControllers({}, { ASSETS: assets, LOADER: loader, ...env });
  await controllers.loadComponent("kcm");
  return await codes[0];
}

describe("the dynamic workers' trace relay", () => {
  it("attaches no tail consumer unless tracing was asked for", async () => {
    const code = await loadedWorkerCode({});
    expect(code.tails).toBeUndefined();
  });

  it("attaches this script as the tail consumer when tracing is on", async () => {
    const code = await loadedWorkerCode({ PUMP_TRACE: "1" });
    expect(code.tails).toHaveLength(1);
  });

  it("forwards the knob into the dynamic worker, or the Go side stays silent", async () => {
    expect((await loadedWorkerCode({})).env).not.toHaveProperty("PUMP_TRACE");
    expect((await loadedWorkerCode({ PUMP_TRACE: "1" })).env).toHaveProperty("PUMP_TRACE", "1");
  });
});

describe("hasUnconvergedWork", () => {
  it("is false on a converged cluster", async () => {
    const { controllers } = newControllers();
    expect(await controllers.hasUnconvergedWork()).toBe(false);
  });

  it("is true while a Deployment's status lags its spec", async () => {
    const { controllers } = newControllers({
      deployments: [
        {
          metadata: { generation: 1 },
          spec: { replicas: 2 },
          status: { observedGeneration: 1, replicas: 1, availableReplicas: 1 },
        },
      ],
    });
    expect(await controllers.hasUnconvergedWork()).toBe(true);
  });

  it("is true while a ReplicaSet's replica count lags", async () => {
    const { controllers } = newControllers({
      replicasets: [{ spec: { replicas: 3 }, status: { replicas: 1 } }],
    });
    expect(await controllers.hasUnconvergedWork()).toBe(true);
  });

  it("treats a failed probe as work, because parking through an apiserver hiccup is not cheap", async () => {
    const { controllers } = newControllers({ failing: true });
    expect(await controllers.hasUnconvergedWork()).toBe(true);
  });

  // A foreground-deleted owner still matches its own spec, so the
  // workload probes above cannot see it: without this the alarm parks
  // mid-cascade and `kubectl delete --cascade=foreground` never finishes
  // on a cluster with no other write traffic.
  it("is true while a graceful deletion is still in flight", async () => {
    const { controllers } = newControllers({ pendingDeletions: 4 });
    expect(await controllers.hasUnconvergedWork()).toBe(true);
  });

  // Waking exactly at the schedule was measured NOT to be enough: a cold
  // KCM needs more than one pass to load and sync, so an overdue CronJob
  // is routed through the machinery that holds a window open until work
  // converges.
  it("is true while a CronJob is past its slot", async () => {
    const { controllers } = newControllers({ nextCronSchedule: { overdue: true } });
    expect(await controllers.hasUnconvergedWork()).toBe(true);
  });

  it("is true while a Job has not reached its completions", async () => {
    const { controllers } = newControllers({
      jobs: [{ spec: { completions: 2 }, status: { succeeded: 1 } }],
    });
    expect(await controllers.hasUnconvergedWork()).toBe(true);
  });

  it("is false for a Job that already succeeded", async () => {
    const { controllers } = newControllers({
      jobs: [
        {
          spec: { completions: 1 },
          status: { succeeded: 1, completionTime: "2026-09-11T00:00:00Z" },
        },
      ],
    });
    expect(await controllers.hasUnconvergedWork()).toBe(false);
  });

  it("counts cluster provisioning on the management cluster", async () => {
    const { controllers, apiCalls } = newControllers(
      {
        clusters: [
          { metadata: { generation: 1 }, status: { observedGeneration: 1, phase: "Provisioning" } },
        ],
      },
      { CLUSTEROP_DISABLED: undefined },
    );
    expect(await controllers.hasUnconvergedWork()).toBe(true);
    expect(apiCalls).toContain("/apis/k8flare.com/v1alpha1/clusters");
  });

  it("does not list Cluster objects on a tenant cluster", async () => {
    const { controllers, apiCalls } = newControllers(
      {},
      { CLUSTEROP_DISABLED: undefined },
      "t1@uid",
    );
    expect(await controllers.hasUnconvergedWork()).toBe(false);
    expect(apiCalls.some((p) => p.endsWith("/clusters"))).toBe(false);
  });
});

describe("the alarm", () => {
  it("parks on an idle cluster without loading a single component", async () => {
    const { controllers, storage } = newControllers();
    await controllers.alarm();

    // The park check runs BEFORE any dynamic worker is touched: an alarm
    // that loaded four ~40MB components just to decide to park re-armed
    // its own warmup window and chained forever.
    expect(storage.alarmSets).toEqual([]);
    expect(storage.kv.get("unconvergedTicks")).toBe(0);
  });

  it("arms one alarm for a CronJob's next fire instead of parking", async () => {
    const at = new Date(Date.now() + 120_000).toISOString();
    const { controllers, storage } = newControllers({ nextCronSchedule: { nextScheduleTime: at } });
    await controllers.alarm();

    expect(storage.alarmSets).toHaveLength(1);
    expect(storage.alarmSets[0]).toBe(Date.parse(at));
  });

  it("clamps a CronJob fire the cluster slept through to now-ish rather than dropping it", async () => {
    const at = new Date(Date.now() - 600_000).toISOString();
    const { controllers, storage } = newControllers({ nextCronSchedule: { nextScheduleTime: at } });
    await controllers.alarm();

    expect(storage.alarmSets).toHaveLength(1);
    expect(storage.alarmSets[0]).toBeGreaterThan(Date.now());
  });

  it("keeps a short cadence while a freshly loaded component's warmup window is open", async () => {
    const { controllers, storage } = newControllers();
    storage.kv.set("warmupUntil", Date.now() + 60_000);
    await controllers.alarm();

    expect(storage.alarmSets).toHaveLength(1);
    expect(storage.alarmSets[0]).toBeLessThanOrEqual(Date.now() + 15_000);
    expect(storage.kv.get("unconvergedTicks")).toBe(0);
  });

  it("backs off exponentially on work that will not converge, up to a ceiling", async () => {
    const { controllers, storage } = newControllers({
      deployments: [
        { metadata: { generation: 1 }, spec: { replicas: 1 }, status: { observedGeneration: 1 } },
      ],
    });
    const intervals: { atLeast: number; atMost: number }[] = [];
    for (let tick = 0; tick < 10; tick++) {
      storage.alarmSets = [];
      const before = Date.now();
      await controllers.alarm();
      const after = Date.now();
      expect(storage.alarmSets).toHaveLength(1);
      intervals.push({
        atLeast: storage.alarmSets[0] - after,
        atMost: storage.alarmSets[0] - before,
      });
    }
    // 15s flat for the first few ticks, then doubling, then capped.
    expect(intervals[0].atMost).toBeGreaterThanOrEqual(15_000);
    expect(intervals[0].atLeast).toBeLessThan(20_000);
    expect(intervals[5].atLeast).toBeGreaterThan(intervals[3].atMost);
    expect(Math.max(...intervals.map((i) => i.atLeast))).toBeLessThanOrEqual(600_000);
    expect(intervals[9].atLeast).toBeGreaterThan(intervals[6].atMost);
  });

  it("does nothing and re-arms nothing when the test kill switch is set", async () => {
    const { controllers, storage, apiCalls } = newControllers({}, { KCM_DISABLED: "1" });
    await controllers.alarm();

    expect(storage.alarmSets).toEqual([]);
    expect(apiCalls).toEqual([]);
  });
});

describe("pokes", () => {
  // Was "and resets the backoff" until 2026-09-11. Resetting here is what kept
  // S26b's ~65,000 alarms a month alive: on a cluster that cannot converge the
  // controllers never stop writing, so the counter never survived long enough
  // for the backoff to grow. Pulling the alarm in is the half that was right.
  it("arms the safety net on a write-origin poke, without discarding the backoff", async () => {
    const { controllers, storage } = newControllers();
    storage.kv.set("unconvergedTicks", 7);
    await controllers.fetch(new Request("http://controllers.internal/"));

    expect(storage.kv.get("unconvergedTicks")).toBe(7);
    expect(storage.alarmSets).toHaveLength(1);
    expect(storage.alarmSets[0]).toBeLessThanOrEqual(Date.now() + SAFETY_NET_INTERVAL_MS);
  });

  it("leaves an alarm that is already due sooner alone", async () => {
    const { controllers, storage } = newControllers();
    storage.alarm = Date.now() + 5_000;
    await controllers.fetch(new Request("http://controllers.internal/"));

    expect(storage.alarmSets).toEqual([]);
  });

  // The Cluster DO's node-lifecycle poke is alarm-origin: all it owes the
  // nodelifecycle controller is one pump window. Arming this DO's own
  // alarm or a warmup window here is what turned one idle BYO node into a
  // permanent 60s chain.
  it("does not arm an alarm or a warmup window for an alarm-origin node-lifecycle poke", async () => {
    const { controllers, storage } = newControllers();
    const resp = await controllers.fetch(
      new Request("http://controllers.internal/safety-net/node-lifecycle"),
    );

    expect(resp.status).toBe(200);
    expect(storage.alarmSets).toEqual([]);
    expect(storage.kv.has("warmupUntil")).toBe(false);
    expect(storage.kv.has("unconvergedTicks")).toBe(false);
  });

  it("drops all state and parks the alarm on teardown", async () => {
    const { controllers, storage } = newControllers();
    storage.kv.set("warmupUntil", Date.now());
    storage.alarm = Date.now() + 1_000;
    const resp = await controllers.fetch(
      new Request("http://controllers.internal/admin/destroy", { method: "POST" }),
    );

    expect(await resp.json()).toEqual({ destroyed: true });
    expect(storage.kv.size).toBe(0);
    expect(storage.alarm).toBeNull();
  });
});

// docs/platform-verification.md S26b measured ~65,000 alarms a month on a
// cluster whose work can never converge: the exponential backoff is supposed
// to reach its 600s ceiling, but the observed average interval was ~40s, and
// the cause was recorded as unidentified. That is a standing violation of cost
// invariant #3. These tests pin what the backoff must do; if the ceiling is
// never reached the first one fails with the interval it actually chose.
describe("alarm backoff on a cluster that never converges", () => {
  const unconverged = {
    deployments: [
      { metadata: { generation: 2 }, spec: { replicas: 1 }, status: { observedGeneration: 1 } },
    ],
  };

  it("reaches the 600s ceiling instead of re-arming every few seconds", async () => {
    const { controllers, storage } = newControllers(unconverged, { KCM_DISABLED: "0" });
    for (let i = 0; i < 20; i++) {
      await controllers.alarm();
    }
    const intervals = storage.alarmSets.map((at) => at - Date.now());
    const last = intervals[intervals.length - 1];
    expect(last).toBeGreaterThanOrEqual(600_000 - 5_000);
  });

  it("does not let a write-path poke reset the backoff it has already earned", async () => {
    const { controllers, storage } = newControllers(unconverged, { KCM_DISABLED: "0" });
    for (let i = 0; i < 20; i++) {
      await controllers.alarm();
    }
    const settled = storage.alarmSets[storage.alarmSets.length - 1] - Date.now();

    // A controller writing status pokes this DO through storage's afterWrite.
    // On a cluster that cannot converge those writes never stop, so if a poke
    // resets the backoff the alarm chain never slows down.
    await controllers.fetch(new Request("http://controllers.internal/"));
    await controllers.alarm();

    const after = storage.alarmSets[storage.alarmSets.length - 1] - Date.now();
    expect(after).toBeGreaterThanOrEqual(settled - 5_000);
  });
});

// S41 measured the probe behind /internal/pending-deletions at one LIST per
// namespaced resource across every namespace, and hasUnconvergedWork ran it on
// every alarm tick. A deletion can only arrive through a write, and every write
// advances the cluster revision, so an unmoved revision is proof that a
// previous empty answer still holds.
describe("pending-deletions probe", () => {
  it("is skipped while the cluster revision has not moved", async () => {
    const { controllers, storage, apiCalls } = newControllers({ pendingDeletions: 0 });

    await controllers.alarm();
    const first = apiCalls.filter((p) => p.endsWith("/internal/pending-deletions")).length;
    expect(first).toBe(1);
    expect(storage.kv.get("noPendingDeletionsAt")).toBe(1);

    await controllers.alarm();
    const second = apiCalls.filter((p) => p.endsWith("/internal/pending-deletions")).length;
    expect(second).toBe(1);
  });

  it("runs again once a write has advanced the revision", async () => {
    const { controllers, storage, apiCalls } = newControllers({ pendingDeletions: 0 });
    await controllers.alarm();
    expect(apiCalls.filter((p) => p.endsWith("/internal/pending-deletions")).length).toBe(1);

    storage.kv.set("noPendingDeletionsAt", 0); // as if a write had moved it on
    await controllers.alarm();
    expect(apiCalls.filter((p) => p.endsWith("/internal/pending-deletions")).length).toBe(2);
  });

  it("still reports work when a deletion is in flight", async () => {
    const { controllers, storage } = newControllers({ pendingDeletions: 1 });
    await controllers.alarm();
    expect(storage.kv.get("noPendingDeletionsAt")).toBeUndefined();
  });
});
