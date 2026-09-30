import { apiserverFetch } from "../loader.ts";
import { componentToken } from "../componenttoken.ts";
import type { Pod, PodStatus } from "./spec.ts";

export interface NodeObject {
  metadata: { name: string; uid?: string };
  spec?: { podCIDR?: string; podCIDRs?: string[] };
  status?: { addresses?: Array<{ type: string; address: string }> };
}

export class APIError extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
  }
}

export class PodAPI {
  constructor(private readonly env: Env) {}

  async fetch(path: string, init?: RequestInit): Promise<Response> {
    const headers = new Headers(init?.headers);
    headers.set("Authorization", `Bearer ${await componentToken(this.env, "podkubelet")}`);
    if (init?.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    return apiserverFetch(this.env, new Request(`https://apiserver.internal${path}`, { ...init, headers }));
  }

  private async json<T>(path: string, init?: RequestInit): Promise<T | null> {
    const resp = await this.fetch(path, init);
    if (resp.status === 404) return null;
    if (!resp.ok) throw new APIError(resp.status, `${init?.method ?? "GET"} ${path}: ${resp.status} ${(await resp.text()).slice(0, 300)}`);
    return (await resp.json()) as T;
  }

  getPod(namespace: string, name: string): Promise<Pod | null> {
    return this.json<Pod>(`/api/v1/namespaces/${namespace}/pods/${name}`);
  }

  getNode(name: string): Promise<NodeObject | null> {
    return this.json<NodeObject>(`/api/v1/nodes/${name}`);
  }

  async configMapData(namespace: string, name: string): Promise<Record<string, string> | null> {
    const cm = await this.json<{ data?: Record<string, string>; binaryData?: Record<string, string> }>(`/api/v1/namespaces/${namespace}/configmaps/${name}`);
    if (!cm) return null;
    return { ...cm.data };
  }

  async secretData(namespace: string, name: string): Promise<Record<string, string> | null> {
    const secret = await this.json<{ data?: Record<string, string>; stringData?: Record<string, string> }>(`/api/v1/namespaces/${namespace}/secrets/${name}`);
    if (!secret) return null;
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(secret.data ?? {})) out[k] = new TextDecoder().decode(Uint8Array.from(atob(v), (c) => c.charCodeAt(0)));
    return out;
  }

  async putStatus(pod: Pod, status: PodStatus): Promise<Pod | null> {
    const body = { ...pod, apiVersion: "v1", kind: "Pod", status };
    const resp = await this.fetch(`/api/v1/namespaces/${pod.metadata.namespace}/pods/${pod.metadata.name}/status`, { method: "PUT", body: JSON.stringify(body) });
    if (resp.status === 404) return null;
    if (resp.status === 409) {
      const fresh = await this.getPod(pod.metadata.namespace, pod.metadata.name);
      if (!fresh || fresh.metadata.uid !== pod.metadata.uid) return null;
      return this.putStatus(fresh, status);
    }
    if (!resp.ok) throw new APIError(resp.status, `pods/status ${pod.metadata.namespace}/${pod.metadata.name}: ${resp.status} ${(await resp.text()).slice(0, 300)}`);
    return (await resp.json()) as Pod;
  }

  async deletePod(namespace: string, name: string, uid: string): Promise<void> {
    const resp = await this.fetch(`/api/v1/namespaces/${namespace}/pods/${name}`, {
      method: "DELETE",
      body: JSON.stringify({ apiVersion: "v1", kind: "DeleteOptions", gracePeriodSeconds: 0, preconditions: { uid } }),
    });
    if (resp.ok || resp.status === 404 || resp.status === 409) return;
    throw new APIError(resp.status, `delete pod ${namespace}/${name}: ${resp.status} ${(await resp.text()).slice(0, 300)}`);
  }

  async event(pod: Pod, type: "Normal" | "Warning", reason: string, message: string, fieldPath?: string): Promise<void> {
    const now = new Date().toISOString();
    const body = {
      apiVersion: "v1",
      kind: "Event",
      metadata: { name: `${pod.metadata.name}.${Date.now().toString(16)}${Math.random().toString(16).slice(2, 8)}`, namespace: pod.metadata.namespace },
      involvedObject: { apiVersion: "v1", kind: "Pod", namespace: pod.metadata.namespace, name: pod.metadata.name, uid: pod.metadata.uid, fieldPath },
      reason,
      message: message.slice(0, 1024),
      type,
      source: { component: "kubelet", host: "cloudflare" },
      firstTimestamp: now,
      lastTimestamp: now,
      count: 1,
      reportingComponent: "kubelet",
      reportingInstance: "cloudflare",
    };
    const resp = await this.fetch(`/api/v1/namespaces/${pod.metadata.namespace}/events`, { method: "POST", body: JSON.stringify(body) });
    if (!resp.ok) console.log(`podkubelet: event ${reason} for ${pod.metadata.namespace}/${pod.metadata.name}: ${resp.status}`);
  }

  async serviceAccountToken(pod: Pod, expirationSeconds: number): Promise<{ token: string; expiresAt: number } | null> {
    const name = pod.spec.serviceAccountName ?? "default";
    const body = {
      apiVersion: "authentication.k8s.io/v1",
      kind: "TokenRequest",
      spec: {
        expirationSeconds,
        boundObjectRef: { apiVersion: "v1", kind: "Pod", name: pod.metadata.name, uid: pod.metadata.uid },
      },
    };
    const out = await this.json<{ status?: { token?: string; expirationTimestamp?: string } }>(`/api/v1/namespaces/${pod.metadata.namespace}/serviceaccounts/${name}/token`, { method: "POST", body: JSON.stringify(body) });
    if (!out?.status?.token) return null;
    return { token: out.status.token, expiresAt: out.status.expirationTimestamp ? Date.parse(out.status.expirationTimestamp) : Date.now() + expirationSeconds * 1000 };
  }
}
