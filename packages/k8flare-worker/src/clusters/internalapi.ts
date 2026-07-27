import type { Env } from "../env.ts";
import { buildKubeconfig } from "./kubeconfig.ts";
import { type ClusterRecord, clusterDOName, registryStub } from "./registry.ts";
import { CLUSTER_ID_RE, invalidateResolveCache } from "./resolve.ts";
import { teardownCluster } from "./teardown.ts";
import {
  type ClusterToken,
  invalidateTokenCache,
  mintToken,
  readClusterTokens,
  writeClusterTokens,
} from "./tokens.ts";

// The cluster operator's platform-operations bridge, mounted under
// /internal/clusters/*. It exists because a Loader-loaded dynamic worker
// cannot be handed a Durable Object namespace (docs/platform-verification.md
// S2), so the Go operator (pkg/controllers/clusterop) can do Kubernetes
// object writes but nothing that touches a DO directly: token vaults, the
// /c/<id> resolution cache, and the teardown cascade all live here.
//
//   POST   /internal/clusters/vault/<doName>               -> {tokenId, token, kubeconfig, endpoint}
//   POST   /internal/clusters/vault/<doName>/tokens        -> same + {superseded: [tokenId]}
//   DELETE /internal/clusters/vault/<doName>/tokens/<id>   -> 204
//   PUT    /internal/clusters/registry/<id>  {uid}         -> 200 (409 on a uid conflict)
//   DELETE /internal/clusters/registry/<id>                -> 204
//   POST   /internal/clusters/teardown/<id>  {doName}      -> 200 once the cascade completes
//
// The caller is authorized by gateway/index.ts before reaching here, and
// only for the management ("default") cluster -- a tenant's own token must
// never be able to mint into another tenant's vault.

// DO instance names are "<id>@<uid>", or the literal "default".
const DO_NAME_RE = /^(?:default|[a-z0-9][a-z0-9-]{0,38}@[0-9a-f-]{36})$/;

function clusterIdOf(doName: string): string {
  return doName === "default" ? "default" : doName.slice(0, doName.lastIndexOf("@"));
}

// The public origin credentials are minted against. GATEWAY_URL is the
// deployment's real external URL (it already exists for in-VM k3s agents,
// which dial out over the internet); the request's own origin is the dev
// fallback -- but note that a request arriving over the GATEWAY service
// binding carries the binding's synthetic host, so a deployment that
// serves real kubeconfigs must set GATEWAY_URL.
function publicOrigin(env: Env, req: Request): string {
  return env.GATEWAY_URL || new URL(req.url).origin;
}

function credentials(env: Env, req: Request, doName: string, token: ClusterToken) {
  const id = clusterIdOf(doName);
  const origin = publicOrigin(env, req);
  return {
    tokenId: token.tokenId,
    token: token.secret,
    kubeconfig: buildKubeconfig(origin, id, token.secret),
    endpoint: id === "default" ? origin : `${origin}/c/${id}`,
  };
}

export async function handleClustersInternalAPI(req: Request, env: Env): Promise<Response> {
  // ["internal", "clusters", <verb>, ...]
  const parts = new URL(req.url).pathname.split("/").filter(Boolean);
  const [, , verb, ...rest] = parts;

  if (verb === "vault") return handleVault(req, env, rest);
  if (verb === "registry") return handleRegistry(req, env, rest);
  if (verb === "teardown") return handleTeardown(req, env, rest);
  return new Response("not found", { status: 404 });
}

async function handleVault(req: Request, env: Env, rest: string[]): Promise<Response> {
  const doName = rest[0] ?? "";
  if (!DO_NAME_RE.test(doName)) {
    return Response.json({ error: "invalid doName" }, { status: 400 });
  }

  // POST /vault/<doName>: ensure the vault exists and return a usable
  // token. Idempotent on purpose -- an operator that crashed after minting
  // but before publishing the Secret must resume with the SAME token, not
  // strand it and mint another.
  if (rest.length === 1 && req.method === "POST") {
    const vault = await readClusterTokens(env, doName);
    if (vault && vault.tokens.length > 0) {
      return Response.json(credentials(env, req, doName, vault.tokens[0]));
    }
    const token = mintToken();
    await writeClusterTokens(env, doName, [token], vault?.revision ?? 0);
    invalidateTokenCache(doName);
    return Response.json(credentials(env, req, doName, token));
  }

  // POST /vault/<doName>/tokens: rotation. Adds a token WITHOUT revoking
  // the old ones (they stay valid until the caller has distributed the
  // replacement) and reports what it supersedes so the caller can revoke
  // them afterwards.
  if (rest.length === 2 && rest[1] === "tokens" && req.method === "POST") {
    const vault = await readClusterTokens(env, doName);
    const token = mintToken();
    const previous = vault?.tokens ?? [];
    await writeClusterTokens(env, doName, [...previous, token], vault?.revision ?? 0);
    invalidateTokenCache(doName);
    return Response.json({
      ...credentials(env, req, doName, token),
      superseded: previous.map((t) => t.tokenId),
    });
  }

  if (rest.length === 3 && rest[1] === "tokens" && req.method === "DELETE") {
    const vault = await readClusterTokens(env, doName);
    if (!vault) return new Response(null, { status: 204 }); // nothing to revoke
    const remaining = vault.tokens.filter((t) => t.tokenId !== rest[2]);
    if (remaining.length === vault.tokens.length) return new Response(null, { status: 204 });
    if (remaining.length === 0) {
      return Response.json(
        { error: "refusing to revoke the last token (delete the Cluster instead)" },
        { status: 409 },
      );
    }
    await writeClusterTokens(env, doName, remaining, vault.revision);
    invalidateTokenCache(doName);
    return new Response(null, { status: 204 });
  }

  return new Response("not found", { status: 404 });
}

async function handleRegistry(req: Request, env: Env, rest: string[]): Promise<Response> {
  const id = rest[0] ?? "";
  if (rest.length !== 1 || !CLUSTER_ID_RE.test(id)) {
    return Response.json({ error: "invalid cluster id" }, { status: 400 });
  }

  if (req.method === "PUT") {
    const body = (await req.json().catch(() => ({}))) as { uid?: string };
    const uid = body.uid ?? "";
    if (!uid) return Response.json({ error: "uid required" }, { status: 400 });
    const resp = await registryStub(env).fetch(`http://registry.internal/clusters/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ uid }),
    });
    // A uid conflict is a real 409 from the registry: two different DO
    // trees claiming one public id would silently repoint live traffic.
    if (resp.ok) invalidateResolveCache(id);
    return new Response(resp.body, resp);
  }

  if (req.method === "DELETE") {
    const resp = await registryStub(env).fetch(`http://registry.internal/clusters/${id}`, {
      method: "DELETE",
    });
    invalidateResolveCache(id);
    return new Response(resp.body, resp);
  }

  return new Response("method not allowed", { status: 405 });
}

async function handleTeardown(req: Request, env: Env, rest: string[]): Promise<Response> {
  const id = rest[0] ?? "";
  if (rest.length !== 1 || !CLUSTER_ID_RE.test(id) || req.method !== "POST") {
    return new Response("not found", { status: 404 });
  }
  if (id === "default") {
    return Response.json({ error: '"default" cannot be torn down' }, { status: 400 });
  }
  const body = (await req.json().catch(() => ({}))) as { doName?: string };
  // Prefer the registry's own record; fall back to the caller's doName so
  // a teardown that already removed the record still finishes the cascade.
  const recResp = await registryStub(env).fetch(`http://registry.internal/clusters/${id}`);
  const doName = recResp.ok
    ? clusterDOName((await recResp.json()) as ClusterRecord)
    : (body.doName ?? "");
  if (!DO_NAME_RE.test(doName)) {
    return Response.json({ error: "no doName for this cluster" }, { status: 400 });
  }
  // Awaited, not waitUntil: the operator drops the Cluster's finalizer on
  // this response, so a failure must be visible as a failure.
  await teardownCluster(env, id, doName);
  return Response.json({ id, torndown: true });
}
