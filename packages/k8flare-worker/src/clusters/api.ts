import type { Env } from "../env.ts";
import { authorizeAdmin } from "./adminauth.ts";
import { buildKubeconfig } from "./kubeconfig.ts";
import { CLUSTER_ID_RE, invalidateResolveCache } from "./resolve.ts";
import { type ClusterRecord, clusterDOName, registryStub } from "./registry.ts";
import { teardownCluster } from "./teardown.ts";
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
  if (!CLUSTER_ID_RE.test(id) && id !== "default") {
    return Response.json({ error: "invalid cluster id" }, { status: 400 });
  }
  // "default" is the zero-config cluster: it has no registry record and
  // cannot be created or deleted, but its TOKEN routes work exactly like
  // a provisioned cluster's -- the K3S_TOKEN Worker secret is abolished
  // and every cluster (default included) authenticates against its own
  // vault, minted/rotated through this API. A fresh deployment has an
  // empty default vault (dev posture: the dev fallback token applies
  // until the first real token is minted here).
  let rec: ClusterRecord | null = null;
  let doName: string;
  if (id === "default") {
    if (parts.length === 2 && req.method === "DELETE") {
      return Response.json({ error: '"default" cannot be deleted' }, { status: 400 });
    }
    doName = "default";
  } else {
    const recResp = await registryStub(env).fetch(`http://registry.internal/clusters/${id}`);
    if (!recResp.ok) return new Response("cluster not found", { status: 404 });
    rec = (await recResp.json()) as ClusterRecord;
    doName = clusterDOName(rec);
  }

  if (parts.length === 2 && req.method === "GET") {
    return Response.json(rec ?? { id: "default", doName: "default" });
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
    const token = mintToken();
    // revision 0 creates the vault -- the default cluster starts without
    // one (provisioned clusters get theirs at POST /clusters time).
    await writeClusterTokens(env, doName, [...(vault?.tokens ?? []), token], vault?.revision ?? 0);
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

  if (parts.length === 2 && req.method === "DELETE" && rec !== null) {
    // Run past the response; failures leave state=deleting and a re-DELETE
    // resumes from the top (every step is idempotent).
    // teardownCluster rejects on a failed step (the operator path needs
    // that to surface); this legacy path has already answered 202, so all
    // it can do is log -- the re-DELETE is the retry.
    ctx.waitUntil(
      teardownCluster(env, rec.id, doName).catch((err) =>
        console.log(`teardown ${rec.id}: ${err}`),
      ),
    );
    return Response.json({ id: rec.id, state: "deleting" }, { status: 202 });
  }

  return new Response("not found", { status: 404 });
}
