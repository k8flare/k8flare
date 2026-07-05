export { CFContainersScheduler } from "./scheduler.ts";
export { NodeVMLarge, NodeVMMedium, NodeVMSmall } from "./nodevm.ts";

import type { Env } from "./env.ts";

// workers/nodes hosts the cf-containers-scheduler (per-Pod microVM node
// binder, scheduler.ts) and the NodeVM container classes it manages. The
// only inbound traffic is pokes: storage's pingNodes write-hook on pod
// writes, and an operator's bootstrap/health request. Token-gated: a
// poke wakes reconciliation (a recurring cost lever), so strangers must
// not be able to pull it.
export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const token = env.K3S_TOKEN || "k8flare-dev-token";
    const auth = request.headers.get("Authorization") || "";
    if (auth !== `Bearer ${token}`) {
      return new Response("unauthorized", { status: 401 });
    }
    const url = new URL(request.url);
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
  },
};
