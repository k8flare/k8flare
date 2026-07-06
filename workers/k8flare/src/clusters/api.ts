import type { Env } from "../env.ts";
import { authorizeAdmin } from "./adminauth.ts";
import { buildKubeconfig } from "./kubeconfig.ts";
import { CLUSTER_ID_RE, invalidateResolveCache } from "./resolve.ts";
import { type ClusterRecord, clusterDOName, registryStub } from "./registry.ts";
import {
  invalidateTokenCache,
  mintToken,
  readClusterTokens,
  writeClusterTokens,
} from "./tokens.ts";

// Cluster management API, mounted at /clusters (never under /c/<id>).
// Admin-authenticated (adminauth.ts); "default" is reserved (the
// zero-config cluster needs no provisioning).
//
//   POST   /clusters {"id": "..."}          -> 201 {id, token, kubeconfig}
//   GET    /clusters                        -> {items: [...]}
//   GET    /clusters/<id>                   -> record
//   GET    /clusters/<id>/kubeconfig        -> kubeconfig YAML (first token)
//   POST   /clusters/<id>/tokens            -> 201 {tokenId, token} (rotation)
//   DELETE /clusters/<id>/tokens/<tokenId>  -> 204 (refuses the last one)
//   DELETE /clusters/<id>                   -> 202 teardown (idempotent)

export async function handleClustersAPI(
  req: Request,
  env: Env,
  ctx: ExecutionContext,
): Promise<Response> {
  if (!(await authorizeAdmin(req, env))) {
    return Response.json(
      { error: "admin authentication required (ADMIN_TOKENS or Cloudflare Access)" },
      { status: 403 },
    );
  }
  const url = new URL(req.url);
  const parts = url.pathname.split("/").filter(Boolean); // ["clusters", id?, "tokens"?, tokenId?]

  if (parts.length === 1) {
    if (req.method === "GET") {
      const resp = await registryStub(env).fetch("http://registry.internal/clusters");
      return new Response(resp.body, resp);
    }
    if (req.method === "POST") {
      const body = (await req.json().catch(() => ({}))) as { id?: string };
      const id = body.id ?? "";
      if (!CLUSTER_ID_RE.test(id) || id === "default") {
        return Response.json(
          { error: `invalid cluster id (must match ${CLUSTER_ID_RE}, "default" is reserved)` },
          { status: 400 },
        );
      }
      const created = await registryStub(env).fetch(`http://registry.internal/clusters/${id}`, {
        method: "POST",
      });
      if (!created.ok) return new Response(created.body, created);
      const rec = (await created.json()) as ClusterRecord;
      const doName = clusterDOName(rec);
      const token = mintToken();
      try {
        await writeClusterTokens(env, doName, [token], 0);
      } catch (err) {
        // Roll the record back so a failed provision is retryable.
        await registryStub(env).fetch(`http://registry.internal/clusters/${id}`, {
          method: "DELETE",
        });
        throw err;
      }
      invalidateResolveCache(id);
      return Response.json(
        {
          id,
          token: token.secret,
          tokenId: token.tokenId,
          kubeconfig: buildKubeconfig(url.origin, id, token.secret),
        },
        { status: 201 },
      );
    }
    return new Response("method not allowed", { status: 405 });
  }

  const id = parts[1];
  if (!CLUSTER_ID_RE.test(id) || id === "default") {
    return Response.json({ error: "invalid cluster id" }, { status: 400 });
  }
  const recResp = await registryStub(env).fetch(`http://registry.internal/clusters/${id}`);
  if (!recResp.ok) return new Response("cluster not found", { status: 404 });
  const rec = (await recResp.json()) as ClusterRecord;
  const doName = clusterDOName(rec);

  if (parts.length === 2 && req.method === "GET") {
    return Response.json(rec);
  }

  if (parts.length === 3 && parts[2] === "kubeconfig" && req.method === "GET") {
    const vault = await readClusterTokens(env, doName);
    if (!vault || vault.tokens.length === 0) {
      return Response.json({ error: "cluster has no tokens" }, { status: 409 });
    }
    return new Response(buildKubeconfig(url.origin, id, vault.tokens[0].secret), {
      headers: { "Content-Type": "application/yaml" },
    });
  }

  if (parts.length === 3 && parts[2] === "tokens" && req.method === "POST") {
    const vault = await readClusterTokens(env, doName);
    if (!vault) return Response.json({ error: "cluster has no token vault" }, { status: 409 });
    const token = mintToken();
    await writeClusterTokens(env, doName, [...vault.tokens, token], vault.revision);
    invalidateTokenCache(doName);
    return Response.json({ tokenId: token.tokenId, token: token.secret }, { status: 201 });
  }

  if (parts.length === 4 && parts[2] === "tokens" && req.method === "DELETE") {
    const vault = await readClusterTokens(env, doName);
    if (!vault) return Response.json({ error: "cluster has no token vault" }, { status: 409 });
    const remaining = vault.tokens.filter((t) => t.tokenId !== parts[3]);
    if (remaining.length === vault.tokens.length) return new Response("not found", { status: 404 });
    if (remaining.length === 0) {
      return Response.json(
        { error: "refusing to delete the last token (delete the cluster instead)" },
        { status: 409 },
      );
    }
    await writeClusterTokens(env, doName, remaining, vault.revision);
    invalidateTokenCache(doName);
    return new Response(null, { status: 204 });
  }

  if (parts.length === 2 && req.method === "DELETE") {
    return teardownCluster(env, ctx, rec);
  }

  return new Response("not found", { status: 404 });
}

// Teardown, idempotent (a re-DELETE resumes): mark deleting (new traffic
// 404s at resolution) -> Scheduler destroy FIRST (live Containers are
// wall-clock-billed) -> Controllers -> WatchHub -> Cluster (facets +
// deleteAll) -> registry record. Each DO exposes /admin/destroy and
// deletes its own storage -- DO storage cannot be enumerated externally.
// NOT covered in v1: R2 objects under clusters/<doName>/ (S3-API
// deletion needs SigV4 signing this Worker doesn't carry yet; R2
// deletes are free, so a later cleanup pass loses nothing -- recorded
// in docs/cost-model.md).
async function teardownCluster(
  env: Env,
  ctx: ExecutionContext,
  rec: ClusterRecord,
): Promise<Response> {
  const doName = clusterDOName(rec);
  await registryStub(env).fetch(`http://registry.internal/clusters/${rec.id}`, {
    method: "PATCH",
    body: JSON.stringify({ state: "deleting" }),
  });
  invalidateResolveCache(rec.id);
  invalidateTokenCache(doName);

  const destroy = async () => {
    const kill = async (ns: DurableObjectNamespace, label: string) => {
      try {
        await ns.get(ns.idFromName(doName)).fetch("http://do.internal/admin/destroy", {
          method: "POST",
        });
      } catch (err) {
        console.log(`teardown ${rec.id}: ${label} destroy failed (re-DELETE to retry): ${err}`);
        throw err;
      }
    };
    await kill(env.SCHEDULER as unknown as DurableObjectNamespace, "scheduler");
    await kill(env.CONTROLLERS, "controllers");
    await kill(env.WATCHHUB, "watchhub");
    await kill(env.CLUSTER, "cluster");
    await registryStub(env).fetch(`http://registry.internal/clusters/${rec.id}`, {
      method: "DELETE",
    });
    console.log(`teardown ${rec.id}: complete`);
  };
  // Run past the response; failures leave state=deleting and a re-DELETE
  // resumes from the top (every step is idempotent).
  ctx.waitUntil(destroy());
  return Response.json({ id: rec.id, state: "deleting" }, { status: 202 });
}
