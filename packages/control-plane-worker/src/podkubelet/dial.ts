import { VIRTUAL_NODE } from "./spec.ts";
import { podKubeletStub, podLedgerStub } from "./wake.ts";

export const LOGS_UNAVAILABLE = `container logs are not available for Pods on node ${VIRTUAL_NODE}: the container's stdout and stderr belong to the platform and cannot be read by the Pod's Durable Object`;

function text(status: number, message: string): Response {
  return new Response(`${message}\n`, { status, headers: { "Content-Type": "text/plain; charset=utf-8" } });
}

export async function servePodDial(env: Env, request: Request): Promise<Response | null> {
  const url = new URL(request.url);
  const parts = url.pathname.split("/").filter(Boolean);
  if ((parts[0] !== "dial" && parts[0] !== "node") || parts[1] !== VIRTUAL_NODE) return null;
  if (parts[0] === "node") {
    if (parts[2] === "containerLogs") return text(400, LOGS_UNAVAILABLE);
    return text(502, `node ${VIRTUAL_NODE} has no kubelet to proxy to`);
  }
  const host = parts[2] ?? "";
  const port = Number(parts[3]);
  if (!host || !Number.isInteger(port) || port <= 0) return text(400, "invalid dial target");
  if (request.headers.get("X-Dial-TLS") === "1") return text(502, `TLS to a Pod on ${VIRTUAL_NODE} is not supported; use http`);
  const entry = await podLedgerStub(env).byIP(host);
  if (!entry) return text(502, `no Pod on ${VIRTUAL_NODE} has IP ${host}`);
  const prefix = `/${parts.slice(0, 4).join("/")}`;
  const rest = url.pathname.slice(prefix.length) || "/";
  const forwarded = new Request(`http://${host}:${port}${rest}${url.search}`, request);
  return podKubeletStub(env, entry.uid).ingress(port, forwarded);
}
