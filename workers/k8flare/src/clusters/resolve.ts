import type { Env } from "../env.ts";
import { type ClusterRecord, clusterDOName, registryStub } from "./registry.ts";

// Cluster resolution: the /c/<id> path prefix names the cluster
// (kubeconfig server URL = https://<host>/c/<id>); no prefix = the
// zero-config "default" cluster (Registry never consulted -- go test /
// e2e / single-cluster OSS deployments keep working unchanged).
// Resolution order is deliberate: registry lookup BEFORE any token
// check or DO touch, so an unknown id 404s without initializing DO
// storage for garbage names.

export const CLUSTER_ID_RE = /^[a-z0-9][a-z0-9-]{0,38}$/;
const RESOLVE_CACHE_TTL_MS = 60_000;

export interface ResolvedCluster {
  id: string;
  doName: string; // "<id>@<uid>", or literal "default"
  basePath: string; // "" for default, "/c/<id>" otherwise
}

const resolveCache = new Map<string, { rec: ClusterRecord | null; expires: number }>();

async function lookupCluster(env: Env, id: string): Promise<ClusterRecord | null> {
  const hit = resolveCache.get(id);
  if (hit && hit.expires > Date.now()) return hit.rec;
  const resp = await registryStub(env).fetch(`http://registry.internal/clusters/${id}`);
  const rec = resp.ok ? ((await resp.json()) as ClusterRecord) : null;
  resolveCache.set(id, { rec, expires: Date.now() + RESOLVE_CACHE_TTL_MS });
  return rec;
}

export function invalidateResolveCache(id: string): void {
  resolveCache.delete(id);
}

/**
 * Parses (and strips) the /c/<id> prefix. Returns the resolved cluster
 * plus the request/URL rewritten to the un-prefixed path the rest of
 * the routing understands, or an error Response (404 unknown/deleting
 * cluster, 400 malformed id).
 */
export async function resolveCluster(
  req: Request,
  env: Env,
  url: URL,
): Promise<{ cluster: ResolvedCluster; req: Request; url: URL } | Response> {
  const m = url.pathname.match(/^\/c\/([^/]+)(\/.*)?$/);
  if (!m) {
    return { cluster: { id: "default", doName: "default", basePath: "" }, req, url };
  }
  const [, id, rest] = m;
  if (!CLUSTER_ID_RE.test(id)) {
    return new Response("invalid cluster id", { status: 400 });
  }
  let doName: string;
  if (id === "default") {
    doName = "default"; // /c/default is the same cluster as the bare paths
  } else {
    const rec = await lookupCluster(env, id);
    if (!rec || rec.state !== "active") {
      return new Response("cluster not found", { status: 404 });
    }
    doName = clusterDOName(rec);
  }
  const stripped = new URL(req.url);
  stripped.pathname = rest || "/";
  return {
    cluster: { id, doName, basePath: `/c/${id}` },
    req: new Request(stripped.toString(), req),
    url: stripped,
  };
}
