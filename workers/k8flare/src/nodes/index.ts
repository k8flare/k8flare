export { CFContainersScheduler } from "./scheduler.ts";
export { NodeVMLarge, NodeVMMedium, NodeVMSmall } from "./nodevm.ts";

import type { Env } from "./env.ts";
import { vmBinding } from "./scheduler.ts";
import { handlePodProxy, handleVKubeProxy } from "./podproxy.ts";

// workers/nodes hosts the cf-containers-scheduler (per-Pod microVM
// NodeVM lifecycle manager, scheduler.ts -- despite the name it no
// longer schedules/binds, see that file's doc comment) and the NodeVM
// container classes it manages. The only inbound traffic is pokes:
// storage's pingNodes write-hook on pod writes, and an operator's
// bootstrap/health request. Token-gated: a poke wakes reconciliation (a
// recurring cost lever), so strangers must not be able to pull it.
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
  //   /kubelet/{podUID|nodeName}/10999/{kubelet path}
  // The scheduler DO resolves which NodeVM (and size tier) backs the
  // pod/node; the NodeVM DO containerFetches the in-VM kubelet port.
  const kubeletMatch = url.pathname.match(/^\/kubelet\/([^/]+)(\/10999\/.*)$/);
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
  // pods/proxy subresource (task #13): HTTP ingress against per-Pod
  // NodeVMs, reusing hostNetwork's direct container-port reachability
  // (podproxy.ts). Replaces the old VirtualNode-backed pod proxy that
  // died with that backend.
  if (url.pathname.startsWith("/podproxy/")) {
    return handlePodProxy(request, env, url.pathname);
  }
  // Virtual kube-proxy backend (task #13): pkg/vkubeproxy (node-side tun
  // + userspace TCP forwarder) re-issues a Pod's ClusterIP-bound
  // connection here as a plain HTTP request, resolved to a backing Pod
  // via Service+EndpointSlice (podproxy.ts).
  const vkubeMatch = url.pathname.match(/^\/vkubeproxy(\/.*)?$/);
  if (vkubeMatch) {
    return handleVKubeProxy(request, env, vkubeMatch[1] || "/");
  }
  const stub = env.SCHEDULER.get(env.SCHEDULER.idFromName("default"));
  return stub.fetch(request);
}
