import { WorkerEntrypoint } from "cloudflare:workers";
import type { Env } from "./env.ts";

// Header stamped by the caller (loader/apiserver.ts today; the /c/<id>
// cluster resolver in the multi-cluster phase) naming which Cluster DO
// instance a storage request targets. Only the kine transport layer
// speaks this header -- everything else propagates the cluster in the
// URL path.
export const CLUSTER_HEADER = "X-K8flare-Cluster";

// ClusterLoopback is the named-entrypoint self service binding
// (wrangler.jsonc's STORAGE) that gives Loader-loaded code -- which can
// receive Fetchers but never DO namespaces (S2 item 3a) -- a route to
// the Cluster DO. Verified end-to-end by spikes/s19-single-worker (G2).
export class ClusterLoopback extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    const name = request.headers.get(CLUSTER_HEADER) ?? "default";
    const ns = this.env.CLUSTER;
    return ns.get(ns.idFromName(name)).fetch(request);
  }
}
