import { DurableObject } from "cloudflare:workers";
import type { Env } from "../env.ts";

// ClusterRegistry: the single metadata DO behind the management API
// (idFromName("registry")). Holds ONLY {id -> {uid, createdAt, state}}
// and the list -- tokens/CA/data live in each cluster's own DO tree
// (auth deliberately does NOT funnel through this DO's single thread).
// No alarms, no WebSockets: idle cost is storage alone (cost invariant
// #1). DO instance names are "<id>@<uid>" -- the uid indirection means a
// recreated cluster id NEVER reuses a DO/facet name (the facet
// name-reuse wedge, packages/k8flare-worker/src/storage/index.ts handleDelete),
// and deleting the record makes the old tree unreachable immediately.

export interface ClusterRecord {
  id: string;
  uid: string;
  createdAt: string;
  state: "active" | "deleting";
}

export function clusterDOName(rec: ClusterRecord): string {
  return `${rec.id}@${rec.uid}`;
}

export class ClusterRegistry extends DurableObject<Env> {
  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const m = url.pathname.match(/^\/clusters(?:\/([a-z0-9][a-z0-9-]*))?$/);
    if (!m) return new Response("not found", { status: 404 });
    const id = m[1];

    if (request.method === "GET" && !id) {
      const list = await this.ctx.storage.list({ prefix: "cluster:" });
      return Response.json({ items: [...list.values()] });
    }
    if (!id) return new Response("not found", { status: 404 });

    const key = `cluster:${id}`;
    if (request.method === "GET") {
      const rec = (await this.ctx.storage.get(key)) as ClusterRecord | undefined;
      if (!rec) return new Response("not found", { status: 404 });
      return Response.json(rec);
    }
    if (request.method === "POST") {
      const existing = await this.ctx.storage.get(key);
      if (existing) return Response.json({ error: "already exists" }, { status: 409 });
      const rec: ClusterRecord = {
        id,
        uid: crypto.randomUUID(),
        createdAt: new Date().toISOString(),
        state: "active",
      };
      await this.ctx.storage.put(key, rec);
      return Response.json(rec, { status: 201 });
    }
    if (request.method === "PATCH") {
      const rec = (await this.ctx.storage.get(key)) as ClusterRecord | undefined;
      if (!rec) return new Response("not found", { status: 404 });
      const body = (await request.json()) as { state?: ClusterRecord["state"] };
      if (body.state) rec.state = body.state;
      await this.ctx.storage.put(key, rec);
      return Response.json(rec);
    }
    if (request.method === "DELETE") {
      await this.ctx.storage.delete(key);
      return new Response(null, { status: 204 });
    }
    return new Response("method not allowed", { status: 405 });
  }
}

export function registryStub(env: Env) {
  const ns = env.REGISTRY;
  return ns.get(ns.idFromName("registry"));
}
