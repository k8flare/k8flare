type MeshEnv = Env & {
  CLOUDFLARE_API_TOKEN?: string;
  CLOUDFLARE_ACCOUNT_ID?: string;
};

const apiRoot = "https://api.cloudflare.com/client/v4";

function meshAPI(env: MeshEnv): { token: string; account: string } | null {
  const token = env.CLOUDFLARE_API_TOKEN ?? "";
  const account = env.CLOUDFLARE_ACCOUNT_ID ?? "";
  if (!token || !account) return null;
  return { token, account };
}

async function meshJSON<T>(env: MeshEnv, path: string, init?: RequestInit): Promise<T | null> {
  const creds = meshAPI(env);
  if (!creds) return null;
  const resp = await fetch(`${apiRoot}/accounts/${creds.account}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${creds.token}`,
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
  if (!resp.ok) {
    console.log(`mesh ${init?.method ?? "GET"} ${path}: ${resp.status}`);
    return null;
  }
  return (await resp.json()) as T;
}

export async function createMeshConnector(env: Env, nodeName: string): Promise<{ id: string } | undefined> {
  const body = await meshJSON<{ success?: boolean; result?: { id?: string } }>(env as MeshEnv, "/warp_connector", {
    method: "POST",
    body: JSON.stringify({ name: `k8flare-${nodeName}` }),
  });
  const id = body?.success ? body.result?.id : undefined;
  return id ? { id } : undefined;
}

export async function deleteMeshConnector(env: Env, id: string): Promise<boolean> {
  if (!id) return true;
  const body = await meshJSON<{ success?: boolean }>(env as MeshEnv, `/warp_connector/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  return body?.success === true;
}

export async function meshConnectorToken(env: Env, id: string): Promise<string | undefined> {
  if (!id) return undefined;
  const body = await meshJSON<{ success?: boolean; result?: string | { token?: string } }>(
    env as MeshEnv,
    `/warp_connector/${encodeURIComponent(id)}/token`,
  );
  if (!body?.success) return undefined;
  if (typeof body.result === "string" && body.result) return body.result;
  if (body.result && typeof body.result === "object" && body.result.token) return body.result.token;
  return undefined;
}
