import type { APIResourceDescriptor } from "./types.ts";

/**
 * Headers safe to carry over from an upstream Response being re-serialized
 * with a modified body. `Content-Length` (and `Content-Encoding`, in case
 * the upstream response was compressed) must NOT be copied verbatim --
 * the re-serialized body's byte length differs from the original, and a
 * stale Content-Length causes a body/length mismatch that some HTTP
 * clients (observed: a local TLS-terminating reverse proxy) reject
 * outright rather than silently tolerate. `Response.json()` recomputes
 * both correctly on its own when they're absent from the headers passed in.
 */
function headersWithoutLength(h: Headers): Headers {
  const out = new Headers(h);
  out.delete("Content-Length");
  out.delete("Content-Encoding");
  return out;
}

/**
 * Build an APIResourceList discovery response for a custom API group.
 */
export function buildDiscoveryResources(
  group: string,
  version: string,
  resources: APIResourceDescriptor[],
): Response {
  return Response.json({
    kind: "APIResourceList",
    apiVersion: "v1",
    groupVersion: `${group}/${version}`,
    resources,
  });
}

/**
 * Build an APIGroup discovery response for a custom API group.
 */
export function buildDiscoveryGroup(group: string, version: string): Response {
  return Response.json({
    kind: "APIGroup",
    apiVersion: "v1",
    name: group,
    versions: [{ groupVersion: `${group}/${version}`, version }],
    preferredVersion: { groupVersion: `${group}/${version}`, version },
  });
}

/**
 * Inject a custom API group into an existing /apis discovery response.
 *
 * Clones the upstream response, appends the group to the APIGroupList,
 * and returns a new Response. Falls back to the original response if
 * parsing fails.
 */
export async function injectCustomAPIGroup(
  goResp: Response,
  group: string,
  version: string,
): Promise<Response> {
  try {
    const body: any = await goResp.json();
    if (body.kind === "APIGroupList" && Array.isArray(body.groups)) {
      body.groups.push({
        name: group,
        versions: [{ groupVersion: `${group}/${version}`, version }],
        preferredVersion: { groupVersion: `${group}/${version}`, version },
      });
    }
    return Response.json(body, {
      status: goResp.status,
      headers: headersWithoutLength(goResp.headers),
    });
  } catch {
    return goResp;
  }
}

/**
 * Inject a custom API group's OpenAPI v3 document entry into an existing
 * /openapi/v3 index response (see pkg/apiserver/discovery.go's
 * RegisterOpenAPIDiscovery -- that Go handler only knows about the
 * per-group-version documents baked into workers/apiserver/assets/openapi/
 * at build time, so it can't list a TypeScript-only custom group itself).
 *
 * `serverRelativeURL` should point at wherever the actual document is
 * served from (see workers/runtime's /openapi/v3/apis/{group}/{version}
 * route) -- the `?hash=...` query kubectl appends to the real entries is
 * just a cache-busting key, not verified against content, so a static
 * placeholder hash is fine here.
 */
export async function injectOpenAPIV3Path(
  goResp: Response,
  group: string,
  version: string,
  serverRelativeURL: string,
): Promise<Response> {
  try {
    const body: any = await goResp.json();
    if (body && typeof body.paths === "object") {
      body.paths[`apis/${group}/${version}`] = { serverRelativeURL };
    }
    return Response.json(body, {
      status: goResp.status,
      headers: headersWithoutLength(goResp.headers),
    });
  } catch {
    return goResp;
  }
}
