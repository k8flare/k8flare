// Worker-side Cloudflare Mesh (warp_connector) API client, for per-Pod
// Mesh membership on the Containers backend
// (spikes/s17-mesh-nodevm/FINDINGS.md's per-Pod-Mesh entry). Automates
// the exact two calls gate 2 already proved by hand for the BYO-VM path
// (pkg/meshconnector's Go equivalent, reused unmodified by cmd/agent) --
// `POST /accounts/{id}/warp_connector`, `GET .../token` -- but mints a
// FRESH connector per Pod at schedule time instead of a human
// pre-provisioning one long-lived connector per BYO VM. This runs
// Worker-side (not inside the microVM) because only the Worker holds
// the Cloudflare API credentials; the microVM only ever sees the
// resulting per-Pod token (passed via envVars, see nodevm.ts's up()).
//
// Requires CLOUDFLARE_API_TOKEN (a Zero Trust/Tunnel-scoped token) and
// CLOUDFLARE_ACCOUNT_ID (env.ts) to be configured -- neither is set by
// this repo's own dev config or CI. Absent either, createMeshConnector
// returns undefined and the Pod's NodeVM boots WITHOUT Mesh membership:
// the same "log and keep going without the feature" posture
// pkg/vkubeproxy/pkg/dnsshim already use for their own optional
// enhancements, not a hard failure of Pod scheduling.
//
// Cap to keep in mind (FINDINGS.md): every warp_connector created here
// consumes one of the account's 50 free Mesh nodes until deleted. Per-Pod
// minting means this cap now bounds concurrent PODS on this backend, not
// concurrent NodeVMs -- deleteMeshConnector below MUST run on every Pod
// teardown path (scheduler.ts's teardown() and its /admin/destroy
// cluster-teardown loop) so the cap doesn't leak.
import type { Env } from "./env.ts";

const API_BASE = "https://api.cloudflare.com/client/v4";

export interface MeshConnector {
  id: string;
  token: string;
}

function configured(env: Env): { token: string; accountId: string } | undefined {
  if (!env.CLOUDFLARE_API_TOKEN || !env.CLOUDFLARE_ACCOUNT_ID) return undefined;
  return { token: env.CLOUDFLARE_API_TOKEN, accountId: env.CLOUDFLARE_ACCOUNT_ID };
}

/**
 * Mints a fresh warp_connector for one Pod's NodeVM, named after the
 * Node so it's identifiable in the dashboard/API listing. Returns
 * undefined (not an error/throw) when CLOUDFLARE_API_TOKEN/ACCOUNT_ID
 * aren't configured, or on any API failure -- callers boot the NodeVM
 * without Mesh membership rather than failing Pod scheduling over an
 * optional networking enhancement.
 */
export async function createMeshConnector(
  env: Env,
  name: string,
): Promise<MeshConnector | undefined> {
  const cfg = configured(env);
  if (!cfg) return undefined;
  try {
    const createResp = await fetch(`${API_BASE}/accounts/${cfg.accountId}/warp_connector`, {
      method: "POST",
      headers: { Authorization: `Bearer ${cfg.token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    });
    if (!createResp.ok) {
      console.log(
        `createMeshConnector ${name}: create ${createResp.status} ${await createResp.text()}`,
      );
      return undefined;
    }
    const created = (await createResp.json()) as { result?: { id?: string } };
    const id = created.result?.id;
    if (!id) {
      console.log(`createMeshConnector ${name}: create response had no id`);
      return undefined;
    }
    const tokenResp = await fetch(
      `${API_BASE}/accounts/${cfg.accountId}/warp_connector/${id}/token`,
      {
        headers: { Authorization: `Bearer ${cfg.token}` },
      },
    );
    if (!tokenResp.ok) {
      console.log(
        `createMeshConnector ${name}: token fetch ${tokenResp.status} ${await tokenResp.text()}`,
      );
      await deleteMeshConnector(env, id); // don't leak the half-created connector against the 50-node cap
      return undefined;
    }
    const tokenBody = (await tokenResp.json()) as { result?: string };
    if (!tokenBody.result) {
      console.log(`createMeshConnector ${name}: token response had no result`);
      await deleteMeshConnector(env, id);
      return undefined;
    }
    return { id, token: tokenBody.result };
  } catch (err) {
    console.log(`createMeshConnector ${name}: ${err}`);
    return undefined;
  }
}

/**
 * Deletes a warp_connector (Pod deleted, scheduling failed after
 * create, or cluster torn down). Best-effort: logs, never throws --
 * mirrors CFContainersScheduler.teardown's posture for the other
 * per-Pod resources it reaps, so one failed cleanup call doesn't stop
 * the rest of a Pod's teardown from proceeding.
 */
export async function deleteMeshConnector(env: Env, id: string): Promise<void> {
  const cfg = configured(env);
  if (!cfg) return;
  try {
    const resp = await fetch(`${API_BASE}/accounts/${cfg.accountId}/warp_connector/${id}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${cfg.token}` },
    });
    if (!resp.ok && resp.status !== 404) {
      console.log(`deleteMeshConnector ${id}: ${resp.status} ${await resp.text()}`);
    }
  } catch (err) {
    console.log(`deleteMeshConnector ${id}: ${err}`);
  }
}
