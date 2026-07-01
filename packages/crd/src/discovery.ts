import type { APIResourceDescriptor } from "./types.ts";

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
      headers: goResp.headers,
    });
  } catch {
    return goResp;
  }
}
