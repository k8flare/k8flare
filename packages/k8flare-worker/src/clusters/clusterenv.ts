import type { Env } from "../env.ts";
import type { ResolvedCluster } from "./resolve.ts";

// clusterEnv: the multi-cluster seam that keeps every downstream module
// (k8s/watch.ts, storage/, nodes/) single-cluster-shaped. Instead of
// threading a cluster parameter
// through every signature, the public routing derives a per-request env
// whose DO namespaces transparently retarget:
//
//  - CLUSTER/WATCHHUB/CONTROLLERS/SCHEDULER: idFromName("default") --
//    the literal every existing call site uses -- resolves to this
//    cluster's DO instance ("<id>@<uid>").
//  - NODE_VM_*: NodeVM DOs are keyed by podUID; the wrapper prefixes
//    "<doName>/" so two clusters' pods can never collide and teardown
//    can enumerate its own VMs.
//  - K3S_TOKEN: replaced with the token the caller actually presented
//    (already verified against the cluster's vault), so downstream
//    defense-in-depth checks (dwAuth in runtime handlers, handleNodes'
//    own gate) re-validate trivially without learning about vaults.
//
// Inside Durable Objects the wrapper doesn't exist -- DOs address
// sibling DOs of the same cluster via their OWN instance name
// (ctx.id.name), see storage/index.ts pingControllers/pingNodes.

function retargetNs(ns: DurableObjectNamespace, doName: string): DurableObjectNamespace {
  return new Proxy(ns, {
    get(target, prop) {
      if (prop === "idFromName") {
        return (name: string) => target.idFromName(name === "default" ? doName : name);
      }
      const v = Reflect.get(target, prop, target);
      return typeof v === "function" ? v.bind(target) : v;
    },
  });
}

function prefixNs<T extends Rpc.DurableObjectBranded | undefined>(
  ns: DurableObjectNamespace<T>,
  prefix: string,
): DurableObjectNamespace<T> {
  return new Proxy(ns, {
    get(target, prop) {
      if (prop === "idFromName") {
        return (name: string) => target.idFromName(`${prefix}/${name}`);
      }
      const v = Reflect.get(target, prop, target);
      return typeof v === "function" ? v.bind(target) : v;
    },
  });
}

export function clusterEnv(env: Env, cluster: ResolvedCluster, presentedToken?: string): Env {
  const derived: Env = {
    ...env,
    CLUSTER: retargetNs(env.CLUSTER, cluster.doName),
    WATCHHUB: retargetNs(env.WATCHHUB, cluster.doName),
    CONTROLLERS: retargetNs(env.CONTROLLERS, cluster.doName),
    SCHEDULER: retargetNs(
      env.SCHEDULER as unknown as DurableObjectNamespace,
      cluster.doName,
    ) as unknown as Env["SCHEDULER"],
    // NodeVM DOs are ALWAYS scheduler-scoped ("<doName>/<podUID>",
    // including "default/<podUID>") so the naming rule is uniform with
    // the scheduler DO's own internal addressing (nodes/scheduler.ts).
    NODE_VM_SMALL: prefixNs(env.NODE_VM_SMALL, cluster.doName),
    NODE_VM_MEDIUM: prefixNs(env.NODE_VM_MEDIUM, cluster.doName),
    NODE_VM_LARGE: prefixNs(env.NODE_VM_LARGE, cluster.doName),
    CLUSTER_DO_NAME: cluster.doName,
    CLUSTER_BASE_PATH: cluster.basePath,
    // Pristine Worker-secret token (see Env.ENV_K3S_TOKEN): kept separate
    // because K3S_TOKEN below is overwritten with the caller-presented
    // token, and the loader must only ever bake the stable secret.
    ENV_K3S_TOKEN: env.ENV_K3S_TOKEN ?? env.K3S_TOKEN,
  };
  if (presentedToken) derived.K3S_TOKEN = presentedToken;
  return derived;
}
