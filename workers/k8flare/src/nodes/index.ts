export { CFContainersScheduler } from "./scheduler.ts";
export { NodeVMLarge, NodeVMMedium, NodeVMSmall } from "./nodevm.ts";

import type { Env } from "./env.ts";
import { vmBinding } from "./scheduler.ts";

// workers/nodes hosts the cf-containers-scheduler (per-Pod microVM node
// binder, scheduler.ts) and the NodeVM container classes it manages. The
// only inbound traffic is pokes: storage's pingNodes write-hook on pod
// writes, and an operator's bootstrap/health request. Token-gated: a
// poke wakes reconciliation (a recurring cost lever), so strangers must
// not be able to pull it.
// The former nodes Worker's fetch handler, called directly from the
// consolidated public routing (gateway/index.ts). Still token-gated on
// its own: a poke wakes reconciliation (a recurring cost lever), so the
// caller's auth is deliberately re-checked here.
export async function handleNodes(request: Request, env: Env): Promise<Response> {
  const token = env.K3S_TOKEN || "k8flare-dev-token";
  const auth = request.headers.get("Authorization") || "";
  if (auth !== `Bearer ${token}`) {
    return new Response("unauthorized", { status: 401 });
  }
  const url = new URL(request.url);
  // Kubelet bridge for the gateway's logs/metrics proxy:
  //   /kubelet/{podUID|nodeName}/10256/{kubelet path}
  // The scheduler DO resolves which NodeVM (and size tier) backs the
  // pod/node; the NodeVM DO containerFetches the in-VM kubelet port.
  const kubeletMatch = url.pathname.match(/^\/kubelet\/([^/]+)(\/10256\/.*)$/);
  if (kubeletMatch) {
    const [, key, portAndPath] = kubeletMatch;
    const scheduler = env.SCHEDULER.get(env.SCHEDULER.idFromName("default"));
    const vm = await scheduler.lookupVM(key);
    if (!vm) {
      return new Response(`no live node VM for ${key}`, { status: 404 });
    }
    const ns = vmBinding(env, vm.tier);
    const target = new URL(request.url);
    target.pathname = portAndPath;
    return ns.get(ns.idFromName(vm.podUID)).fetch(new Request(target.toString(), request));
  }
  if (url.pathname.startsWith("/podproxy/")) {
    // The old VirtualNode-backed pod proxy died with that backend.
    // Reinstating HTTP ingress against per-Pod NodeVMs (via the real
    // kubelet API) is tracked as follow-up work.
    return new Response("pods/proxy is temporarily unavailable on the per-Pod node backend", {
      status: 501,
    });
  }
  const stub = env.SCHEDULER.get(env.SCHEDULER.idFromName("default"));
  return stub.fetch(request);
}
