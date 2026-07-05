import type { CRDConfig } from "./types.ts";
import { crGet, crList, crPut, crDelete } from "./storage.ts";
import { applyMergePatch, applyJSONPatch } from "./patch.ts";

/** Return a Response containing a Kubernetes Status error JSON body. */
function crdError(status: number, message: string): Response {
  return Response.json(
    { kind: "Status", apiVersion: "v1", status: "Failure", message, code: status },
    { status },
  );
}

/**
 * Generic CRUD handler for a custom resource.
 *
 * Dispatches GET/POST/PUT/DELETE based on the HTTP method and config.
 * Supports the "status" subresource via config._subresource.
 */
export async function handleResourceCRUD(
  req: Request,
  env: any,
  namespace: string,
  name: string,
  config: CRDConfig,
): Promise<Response> {
  // PUT/GET .../resource/{name}/status
  if (config._subresource === "status" && name) {
    if (req.method === "PUT") {
      return handleStatusUpdate(req, env, config.prefix, config.resource, namespace, name);
    }
    if (req.method === "GET") {
      const result = await crGet(env, config.prefix, namespace, name);
      if (!result) return crdError(404, `${config.resource} "${name}" not found`);
      return Response.json(result.obj);
    }
    return crdError(405, `method ${req.method} not allowed on status`);
  }

  if (config._subresource) {
    return crdError(404, `unknown subresource "${config._subresource}"`);
  }

  switch (req.method) {
    case "GET": {
      if (name) {
        const result = await crGet(env, config.prefix, namespace, name);
        if (!result) return crdError(404, `${config.resource} "${name}" not found`);
        return Response.json(result.obj);
      }
      // List -- namespace="" means all-namespace list
      const items = await crList(env, config.prefix, namespace);
      return Response.json({
        apiVersion: config.apiVersion,
        kind: config.listKind,
        metadata: {},
        items,
      });
    }

    case "POST": {
      if (name) return crdError(405, "POST with name not allowed; use PUT to update");
      let obj: any;
      try {
        obj = await req.json();
      } catch {
        return crdError(400, "invalid JSON");
      }
      if (!obj.metadata?.name) return crdError(400, "metadata.name is required");
      if (namespace) obj.metadata.namespace = namespace;
      config.applyDefaults(obj);
      const err = config.validate(obj.spec);
      if (err) return crdError(400, err);
      const resp = await crPut(env, config.prefix, obj);
      if (!resp.ok) {
        const r: any = await resp.json().catch(() => ({}));
        if (resp.status === 409) {
          return crdError(409, `${config.resource} "${obj.metadata.name}" already exists`);
        }
        return crdError(500, r.error || "storage error");
      }
      const r: any = await resp.json();
      obj.metadata.resourceVersion = String(r.revision);
      return Response.json(obj, { status: 201 });
    }

    case "PUT": {
      if (!name) return crdError(400, "name is required for update");
      let obj: any;
      try {
        obj = await req.json();
      } catch {
        return crdError(400, "invalid JSON");
      }
      const existing = await crGet(env, config.prefix, namespace, name);
      if (!existing) return crdError(404, `${config.resource} "${name}" not found`);
      obj.metadata = {
        ...existing.obj.metadata,
        ...obj.metadata,
        name,
        namespace: namespace || existing.obj.metadata.namespace,
      };
      // Bump generation if spec changed
      if (JSON.stringify(obj.spec) !== JSON.stringify(existing.obj.spec)) {
        obj.metadata.generation = (existing.obj.metadata.generation || 0) + 1;
      }
      // Preserve status from existing (spec-only update via PUT)
      obj.status = existing.obj.status;
      config.applyDefaults(obj);
      const err = config.validate(obj.spec);
      if (err) return crdError(400, err);
      const resp = await crPut(env, config.prefix, obj, existing.modRevision);
      if (!resp.ok) return crdError(409, "conflict: resource was modified");
      const r: any = await resp.json();
      obj.metadata.resourceVersion = String(r.revision);
      return Response.json(obj);
    }

    case "DELETE": {
      if (!name) return crdError(400, "name is required for delete");
      const existing = await crGet(env, config.prefix, namespace, name);
      if (!existing) return crdError(404, `${config.resource} "${name}" not found`);
      await crDelete(env, config.prefix, namespace, name);
      return Response.json({
        kind: "Status",
        apiVersion: "v1",
        status: "Success",
        message: `${config.resource} "${name}" deleted`,
      });
    }

    case "PATCH": {
      if (!name) return crdError(400, "name is required for patch");
      const existing = await crGet(env, config.prefix, namespace, name);
      if (!existing) return crdError(404, `${config.resource} "${name}" not found`);

      const contentType = req.headers.get("Content-Type") || "";
      let body: any;
      try {
        body = await req.json();
      } catch {
        return crdError(400, "invalid JSON");
      }

      let obj: any;
      try {
        if (contentType.includes("json-patch")) {
          obj = applyJSONPatch(existing.obj, body);
        } else if (
          contentType.includes("merge-patch") ||
          contentType.includes("strategic-merge-patch") ||
          contentType === ""
        ) {
          // kubectl (label/annotate/client-side apply) sends merge-patch+json
          // for custom resources that have no OpenAPI schema to compute a
          // strategic-merge 3-way diff against -- see spikes/s14 gap report.
          // Strategic-merge-patch bodies aren't distinguished from plain
          // merge-patch here since we have no per-field merge-key metadata;
          // treating them as a merge-patch is correct for the object-level
          // (non-list-field) patches kubectl actually sends against a CRD.
          obj = applyMergePatch(existing.obj, body);
        } else {
          return crdError(415, `unsupported patch content-type "${contentType}"`);
        }
      } catch (e: any) {
        return crdError(400, `patch failed: ${e?.message || e}`);
      }

      obj.metadata = {
        ...obj.metadata,
        name,
        namespace: namespace || existing.obj.metadata.namespace,
      };
      if (JSON.stringify(obj.spec) !== JSON.stringify(existing.obj.spec)) {
        obj.metadata.generation = (existing.obj.metadata.generation || 0) + 1;
      }
      // Same as PUT: the top-level resource endpoint doesn't touch status.
      obj.status = existing.obj.status;
      config.applyDefaults(obj);
      const err = config.validate(obj.spec);
      if (err) return crdError(400, err);
      const resp = await crPut(env, config.prefix, obj, existing.modRevision);
      if (!resp.ok) return crdError(409, "conflict: resource was modified");
      const r: any = await resp.json();
      obj.metadata.resourceVersion = String(r.revision);
      return Response.json(obj);
    }

    default:
      return crdError(405, `method ${req.method} not allowed`);
  }
}

/**
 * Update the status subresource of a custom resource.
 *
 * Preserves the existing spec and only replaces the status field.
 * Sets observedGeneration if the caller did not provide it.
 */
export async function handleStatusUpdate(
  req: Request,
  env: any,
  prefix: string,
  resource: string,
  namespace: string,
  name: string,
): Promise<Response> {
  let incoming: any;
  try {
    incoming = await req.json();
  } catch {
    return crdError(400, "invalid JSON");
  }

  const existing = await crGet(env, prefix, namespace, name);
  if (!existing) return crdError(404, `${resource} "${name}" not found`);

  // Status subresource: only update status, preserve spec
  const updated = { ...existing.obj };
  updated.status = incoming.status || {};
  // Record observedGeneration if caller did not set it
  if (updated.status.observedGeneration === undefined) {
    updated.status.observedGeneration = updated.metadata.generation || 1;
  }

  const resp = await crPut(env, prefix, updated, existing.modRevision);
  if (!resp.ok) return crdError(409, "conflict: resource was modified");
  const r: any = await resp.json();
  updated.metadata.resourceVersion = String(r.revision);
  return Response.json(updated);
}
