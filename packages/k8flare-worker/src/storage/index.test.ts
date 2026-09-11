// The Cluster DO's write-side policy: which keys poke which reconciler,
// and when the safety-net alarm may stay armed. Both are cost invariants
// (#1/#3 -- nothing runs on an idle cluster, and an alarm is event-armed
// or parked), and both were previously observable only through a
// five-minute end-to-end lane.
//
// `cloudflare:workers` is stubbed because storage/watch.ts reaches it
// through facets.ts; nothing under test here touches DurableObject.
import { beforeEach, describe, expect, it, vi } from "vite-plus/test";

vi.mock("cloudflare:workers", () => ({ DurableObject: class {} }));

import { Cluster } from "./index.ts";

const SAFETY_NET_INTERVAL_MS = 60_000;
const DEBOUNCE_MS = 1_000;

interface ClusterInternals {
  afterWrite(key: string): Promise<void>;
  alarm(): Promise<void>;
}

class FakeStorage {
  kv = new Map<string, unknown>();
  alarm: number | null = null;
  alarmSets: number[] = [];
  readonly sql: { exec(query: string, ...args: unknown[]): { toArray(): unknown[] } };

  constructor(hasLiveNode: () => boolean) {
    this.sql = {
      exec: (_query: string, ...args: unknown[]) => {
        // hasLiveKeyUnderPrefix passes the prefix as the first bound
        // parameter; the schema statements pass none.
        const prefix = args[0];
        const rows =
          prefix === "/registry/nodes/" && hasLiveNode() ? [{ name: "/registry/nodes/n1" }] : [];
        return { toArray: () => rows };
      },
    };
  }

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
  get(key: string): Promise<unknown> {
    return Promise.resolve(this.kv.get(key));
  }
  put(key: string, value: unknown): Promise<void> {
    this.kv.set(key, value);
    return Promise.resolve();
  }
  delete(key: string): Promise<boolean> {
    return Promise.resolve(this.kv.delete(key));
  }
}

/** A DO namespace whose stub records every poke, and can fail them all. */
function fakeNamespace(mode: "ok" | "fail" = "ok") {
  const pokes: string[] = [];
  return {
    pokes,
    binding: {
      idFromName: (name: string) => ({ name }),
      get: () => ({
        fetch: (url: string) => {
          pokes.push(url);
          if (mode === "fail") return Promise.reject(new Error("Network connection lost."));
          return Promise.resolve(new Response("{}"));
        },
      }),
    },
  };
}

function newCluster(
  opts: {
    hasLiveNode?: boolean;
    controllers?: "ok" | "fail" | "unbound";
    scheduler?: "ok" | "fail";
    kcmDisabled?: boolean;
  } = {},
) {
  let hasLiveNode = opts.hasLiveNode ?? false;
  const storage = new FakeStorage(() => hasLiveNode);
  const controllers = fakeNamespace(opts.controllers === "fail" ? "fail" : "ok");
  const scheduler = fakeNamespace(opts.scheduler ?? "ok");
  const ctx = { id: { name: "default" }, facets: { get: () => null, delete: () => {} }, storage };
  const env: Record<string, unknown> = {
    CONTROLLERS: opts.controllers === "unbound" ? undefined : controllers.binding,
    SCHEDULER: scheduler.binding,
  };
  if (opts.kcmDisabled) env.KCM_DISABLED = "1";
  const cluster = new Cluster(ctx, env) as unknown as ClusterInternals;
  return {
    cluster,
    storage,
    controllers,
    scheduler,
    setLiveNode: (v: boolean) => {
      hasLiveNode = v;
    },
  };
}

describe("afterWrite", () => {
  let now = 0;
  beforeEach(() => {
    now = Date.now();
  });

  it("pokes both reconcilers for a Pod write and clears the flags once delivered", async () => {
    const { cluster, storage, controllers, scheduler } = newCluster();
    await cluster.afterWrite("/registry/pods/default/nginx");

    expect(controllers.pokes).toEqual(["http://controllers.internal/"]);
    expect(scheduler.pokes).toEqual(["http://nodes.internal/"]);
    // The persisted flags exist so this DO's alarm can redeliver a poke
    // whose detached promise died with its request; a delivered poke must
    // clear its own, or the alarm re-arms forever.
    expect(storage.kv.has("pendingPing:controllers")).toBe(false);
    expect(storage.kv.has("pendingPing:nodes")).toBe(false);
  });

  it("keeps the pending flag when a poke fails, so the alarm redelivers it", async () => {
    const { cluster, storage, controllers } = newCluster({ controllers: "fail" });
    await cluster.afterWrite("/registry/deployments/default/web");

    expect(controllers.pokes).toEqual(["http://controllers.internal/"]);
    expect(storage.kv.get("pendingPing:controllers")).toBe(true);
    expect(storage.alarm).not.toBeNull();
  });

  it("does not poke anything for a Secret write", async () => {
    const { cluster, storage, controllers, scheduler } = newCluster();
    // Deliberately absent from CONTROLLER_RELEVANT_PREFIXES: the cluster
    // operator writes Secrets itself, and poking on them re-drives the
    // reconcile that wrote them.
    await cluster.afterWrite("/registry/secrets/default/cluster-token");

    expect(controllers.pokes).toEqual([]);
    expect(scheduler.pokes).toEqual([]);
    expect(storage.alarmSets).toEqual([]);
    expect(storage.kv.size).toBe(0);
  });

  it("pokes the controllers but not the node scheduler for a Cluster write", async () => {
    const { cluster, controllers, scheduler } = newCluster();
    await cluster.afterWrite("/registry/clusters/tenant-a");

    expect(controllers.pokes).toEqual(["http://controllers.internal/"]);
    expect(scheduler.pokes).toEqual([]);
  });

  it("pulls the safety net in for a Node write", async () => {
    const { cluster, storage } = newCluster();
    await cluster.afterWrite("/registry/nodes/node-1");

    expect(storage.alarm).not.toBeNull();
    expect(storage.alarm!).toBeLessThanOrEqual(now + DEBOUNCE_MS + 500);
  });

  it("never pushes an alarm that is already due sooner further out", async () => {
    const { cluster, storage } = newCluster();
    storage.alarm = now + 200;
    storage.alarmSets = [];
    await cluster.afterWrite("/registry/nodes/node-1");

    expect(storage.alarmSets).toEqual([]);
    expect(storage.alarm).toBe(now + 200);
  });

  it("clears the controllers flag rather than leaving it armed when KCM is disabled", async () => {
    const { cluster, storage, controllers } = newCluster({ kcmDisabled: true });
    await cluster.afterWrite("/registry/pods/default/nginx");

    expect(controllers.pokes).toEqual([]);
    expect(storage.kv.has("pendingPing:controllers")).toBe(false);
  });
});

describe("the safety-net alarm", () => {
  it("gives the node-lifecycle controller a pump window while a Node is live, and stays armed", async () => {
    const { cluster, storage, controllers } = newCluster({ hasLiveNode: true });
    await cluster.alarm();

    // Lease staleness is detected by the ABSENCE of a write, so this is
    // the one input with no event to arm a poke from.
    expect(controllers.pokes).toEqual(["http://controllers.internal/safety-net/node-lifecycle"]);
    expect(storage.alarm).toBeGreaterThan(Date.now() + SAFETY_NET_INTERVAL_MS - 5_000);
  });

  it("parks once no Node is left and nothing is undelivered", async () => {
    const { cluster, storage, controllers } = newCluster({ hasLiveNode: false });
    await cluster.alarm();

    expect(controllers.pokes).toEqual([]);
    expect(storage.alarmSets).toEqual([]);
    expect(storage.alarm).toBeNull();
  });

  it("stays armed for an undelivered poke and parks once it lands", async () => {
    const failing = newCluster({ hasLiveNode: false, controllers: "fail" });
    failing.storage.kv.set("pendingPing:controllers", true);
    await failing.cluster.alarm();
    expect(failing.storage.alarm).not.toBeNull();

    const delivering = newCluster({ hasLiveNode: false });
    delivering.storage.kv.set("pendingPing:controllers", true);
    await delivering.cluster.alarm();
    expect(delivering.controllers.pokes).toEqual(["http://controllers.internal/"]);
    expect(delivering.storage.alarmSets).toEqual([]);
  });

  it("does not poke the node-lifecycle safety net when there is no Node to monitor", async () => {
    const { cluster, controllers, setLiveNode } = newCluster({ hasLiveNode: true });
    setLiveNode(false);
    await cluster.alarm();

    expect(controllers.pokes).toEqual([]);
  });
});
