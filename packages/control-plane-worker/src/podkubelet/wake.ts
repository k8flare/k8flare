import { clusterName } from "../clusterid.ts";
import { PodAPI } from "./api.ts";
import type { PodKubelet } from "./kubelet.ts";
import type { PodLedger } from "./ledger.ts";
import { VIRTUAL_NODE } from "./spec.ts";

const POD_KEY = /^\/registry\/pods\/([^/]+)\/([^/]+)$/;

export function podKubeletStub(env: Env, uid: string): DurableObjectStub<PodKubelet> {
  return env.POD_KUBELET.get(env.POD_KUBELET.idFromName(uid));
}

export function podLedgerStub(env: Env): DurableObjectStub<PodLedger> {
  return env.POD_LEDGER.get(env.POD_LEDGER.idFromName(clusterName(env)));
}

export async function wakePodKubelets(env: Env, keys: string[]): Promise<number> {
  const api = new PodAPI(env);
  let woken = 0;
  for (const key of keys) {
    const m = POD_KEY.exec(key);
    if (!m) continue;
    const [, namespace, name] = m;
    const uids = new Set<string>();
    try {
      const pod = await api.getPod(namespace, name);
      if (pod?.spec.nodeName === VIRTUAL_NODE && pod.metadata.uid) uids.add(pod.metadata.uid);
      for (const entry of await podLedgerStub(env).lookup(namespace, name)) uids.add(entry.uid);
    } catch (err) {
      console.log(`podkubelet: wake ${namespace}/${name}: ${String(err)}`);
      continue;
    }
    for (const uid of uids) {
      try {
        await podKubeletStub(env, uid).reconcile({ namespace, name });
        woken++;
      } catch (err) {
        console.log(`podkubelet: reconcile ${namespace}/${name} (${uid}): ${String(err)}`);
      }
    }
  }
  return woken;
}
