import { loadWasmWorker } from "@k8flare/loader-kit";

function forwardedHost(request: Request): string {
  const fromURL = new URL(request.url).host;
  if (fromURL.includes(":")) return fromURL;
  const fromHeader = request.headers.get("Host") ?? "";
  if (fromHeader.includes(":")) return fromHeader;
  return fromURL;
}

export async function apiserverFetch(env: Env, request: Request): Promise<Response> {
  const headers = new Headers(request.headers);
  const auth = request.headers.get("Authorization");
  if (auth) headers.set("Authorization", auth);
  const host = forwardedHost(request);
  if (host) headers.set("X-Forwarded-Host", host);
  request = new Request(request, { headers });
  const worker = await loadWasmWorker(env.LOADER, env.ASSETS, "apiserver", {
    STORAGE: env.STORAGE_SVC,
    APIGROUPS: env.APIGROUPS,
    OPENAPI: env.OPENAPI,
    CUSTOMRESOURCES: env.CUSTOMRESOURCES,
    ADMIN_TOKEN: env.ADMIN_TOKEN,
    READONLY_TOKEN: env.READONLY_TOKEN,
    JOIN_TOKEN: env.JOIN_TOKEN,
    SECRETS_ENCRYPTION_KEYS: env.SECRETS_ENCRYPTION_KEYS,
    ACCESS_TEAM_DOMAIN: env.ACCESS_TEAM_DOMAIN,
    ACCESS_AUD: env.ACCESS_AUD,
    ACCESS_GROUPS_CLAIM: env.ACCESS_GROUPS_CLAIM,
    ACCESS_GROUPS_PREFIX: env.ACCESS_GROUPS_PREFIX,
    CLUSTER_UID: env.CLUSTER_UID,
    OUTBOUND: env.OUTBOUND,
    TUNNEL: env.TUNNEL,
    ADMISSION: env.ADMISSION,
    HOOKS: env.HOOKS,
  }, env.APISERVER);
  return worker.fetch(request);
}
