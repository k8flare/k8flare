import { apiserverFetch } from "../loader.ts";
import { clusterName } from "../clusterid.ts";

type R2Env = Env & {
  CLOUDFLARE_API_TOKEN?: string;
  R2_ACCOUNT_ID?: string;
  R2_ACCESS_KEY_ID?: string;
  R2_SECRET_ACCESS_KEY?: string;
  R2_BUCKET?: string;
};

export type TempCreds = { accessKeyId: string; secretAccessKey: string; sessionToken: string };

export async function r2PodEnv(env: Env, namespace: string, name: string): Promise<Record<string, string>> {
  const e = env as R2Env;
  if (!e.R2_ACCOUNT_ID || !e.R2_BUCKET) return {};
  const prefix = `clusters/${clusterName(env)}/pods/${namespace}/${name}/`;
  const endpoint = `https://${e.R2_ACCOUNT_ID}.r2.cloudflarestorage.com`;
  const minted = (await mintLocal(e, prefix)) ?? (await mintTemp(e, prefix));
  if (!minted) return {};
  return {
    AWS_ACCESS_KEY_ID: minted.accessKeyId,
    AWS_SECRET_ACCESS_KEY: minted.secretAccessKey,
    AWS_SESSION_TOKEN: minted.sessionToken,
    AWS_REGION: "auto",
    AWS_ENDPOINT_URL: endpoint,
    R2_BUCKET: e.R2_BUCKET,
    R2_PREFIX: prefix,
  };
}

export async function mintLocal(env: R2Env, prefix: string): Promise<TempCreds | null> {
  if (!env.R2_ACCESS_KEY_ID || !env.R2_SECRET_ACCESS_KEY || !env.R2_ACCOUNT_ID || !env.R2_BUCKET) return null;
  const resp = await apiserverFetch(env, new Request("https://apiserver.internal/internal/r2/mint", {
    method: "POST",
    headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      accessKeyId: env.R2_ACCESS_KEY_ID,
      secretAccessKey: env.R2_SECRET_ACCESS_KEY,
      accountId: env.R2_ACCOUNT_ID,
      bucket: env.R2_BUCKET,
      prefix,
    }),
  }));
  if (!resp.ok) return null;
  return resp.json() as Promise<TempCreds>;
}

async function mintTemp(env: R2Env, prefix: string): Promise<TempCreds | null> {
  if (!env.CLOUDFLARE_API_TOKEN || !env.R2_ACCESS_KEY_ID) return null;
  const resp = await fetch(`https://api.cloudflare.com/client/v4/accounts/${env.R2_ACCOUNT_ID}/r2/temp-access-credentials`, {
    method: "POST",
    headers: { Authorization: `Bearer ${env.CLOUDFLARE_API_TOKEN}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      bucket: env.R2_BUCKET,
      parentAccessKeyId: env.R2_ACCESS_KEY_ID,
      permission: "object-read-write",
      ttlSeconds: 3600,
      prefixes: [prefix],
    }),
  });
  if (!resp.ok) return null;
  const body = (await resp.json()) as { success?: boolean; result?: TempCreds };
  if (!body.success || !body.result?.accessKeyId || !body.result.secretAccessKey || !body.result.sessionToken) {
    return null;
  }
  return body.result;
}
