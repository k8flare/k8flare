import { API_HOST } from "./spec.ts";

export interface EgressDeps {
  apiFetch(request: Request): Promise<Response>;
  serviceAccountToken(): Promise<string | null>;
  clusterTarget?(host: string, port: number): Promise<Fetcher | null>;
}

const hopByHop = ["connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade"];

function isAPIServer(host: string): boolean {
  const name = host.split(":")[0].toLowerCase();
  return name === API_HOST || name === "kubernetes.default" || name === "kubernetes.default.svc.cluster.local" || name === "kubernetes";
}

export async function routeEgress(request: Request, deps: EgressDeps): Promise<Response> {
  const url = new URL(request.url);
  if (isAPIServer(url.host)) {
    const token = await deps.serviceAccountToken();
    const headers = new Headers(request.headers);
    for (const h of hopByHop) headers.delete(h);
    if (token) headers.set("Authorization", `Bearer ${token}`);
    else headers.delete("Authorization");
    headers.set("Host", API_HOST);
    return deps.apiFetch(new Request(`https://${API_HOST}${url.pathname}${url.search}`, { method: request.method, headers, body: request.body, redirect: "manual" }));
  }
  const port = Number(url.port || (url.protocol === "https:" ? 443 : 80));
  const target = deps.clusterTarget ? await deps.clusterTarget(url.hostname, port) : null;
  if (target) return target.fetch(request);
  return fetch(request);
}
