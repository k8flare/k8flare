import type { Env } from "../env.ts";
import { invalidateResolveCache } from "./resolve.ts";
import { registryStub } from "./registry.ts";
import { invalidateTokenCache } from "./tokens.ts";

// Teardown, idempotent (a retry resumes): mark deleting (new traffic
// 404s at resolution) -> Scheduler destroy FIRST (live Containers are
// wall-clock-billed) -> Controllers -> WatchHub -> Cluster (facets +
// deleteAll) -> registry record. Each DO exposes /admin/destroy and
// deletes its own storage -- DO storage cannot be enumerated externally.
//
// Extracted from clusters/api.ts so the cluster operator's internal API
// (internalapi.ts) and the legacy admin API share one implementation
// rather than two that can drift. The cascade is AWAITED here: the
// operator only drops the Cluster object's finalizer once this resolves,
// so a failure must surface as a failure rather than disappearing into a
// waitUntil while the object vanishes. The admin API keeps its old
// fire-and-forget shape by not awaiting the returned promise.
export async function teardownCluster(env: Env, id: string, doName: string): Promise<void> {
  await registryStub(env).fetch(`http://registry.internal/clusters/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ state: "deleting" }),
  });
  invalidateResolveCache(id);
  invalidateTokenCache(doName);

  const kill = async (ns: DurableObjectNamespace, label: string) => {
    try {
      await ns.get(ns.idFromName(doName)).fetch("http://do.internal/admin/destroy", {
        method: "POST",
      });
    } catch (err) {
      console.log(`teardown ${id}: ${label} destroy failed (retry to resume): ${err}`);
      throw err;
    }
  };
  await kill(env.SCHEDULER as unknown as DurableObjectNamespace, "scheduler");
  await kill(env.CONTROLLERS, "controllers");
  await kill(env.WATCHHUB, "watchhub");
  await kill(env.CLUSTER, "cluster");
  await registryStub(env).fetch(`http://registry.internal/clusters/${id}`, {
    method: "DELETE",
  });
  console.log(`teardown ${id}: complete`);
}
