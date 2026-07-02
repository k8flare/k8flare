import { RESOURCE_KINDS } from "./gen/resource-kinds.gen.ts";

/**
 * Map a Kubernetes API URL pathname to a kine storage prefix.
 *
 * Examples:
 *   /api/v1/namespaces                                        -> /registry/namespaces/
 *   /api/v1/namespaces/{ns}/pods                              -> /registry/pods/{ns}/
 *   /api/v1/pods                                              -> /registry/pods/
 *   /apis/coordination.k8s.io/v1/namespaces/{ns}/leases       -> /registry/leases/{ns}/
 */
export function urlToStoragePrefix(pathname: string): string | null {
  let path: string;
  if (pathname.startsWith("/api/v1/")) {
    path = pathname.slice(8); // strip "/api/v1/"
  } else if (pathname.startsWith("/apis/")) {
    const parts = pathname.split("/");
    // /apis/{group}/{version}/...  ->  parts[4..]
    path = parts.slice(4).join("/");
  } else {
    return null;
  }

  // Remove trailing slash
  path = path.replace(/\/$/, "");

  const parts = path.split("/");

  // namespaces/{ns}/{resource} -> /registry/{resource}/{ns}/
  if (parts[0] === "namespaces" && parts.length >= 3) {
    return "/registry/" + parts[2] + "/" + parts[1] + "/";
  }

  // {resource} -> /registry/{resource}/
  if (parts.length === 1 && parts[0] !== "") {
    return "/registry/" + parts[0] + "/";
  }

  return null;
}

// RESOURCE_KINDS (resource's plural name -> Kind, covering every resource
// the Go apiserver serves plus the TypeScript-only CRDs) is generated from
// pkg/apiserver/apidef.Table by cmd/k8flare-gen -- see
// packages/k8s/src/gen/resource-kinds.gen.ts. It used to be hand-written
// here, which had already caused two separate watch-bookmark bugs (a
// resource added to the Go apiserver but forgotten in this map) before the
// generator existed.

/**
 * Determine the Kind and apiVersion for a resource URL, e.g. for constructing
 * a synthetic object (such as a watch bookmark) that must decode as the
 * correct concrete type. Returns null for unrecognized resources.
 */
export function resourceKindForPath(pathname: string): { kind: string; apiVersion: string } | null {
  let group = "";
  let version: string;
  let rest: string;

  if (pathname.startsWith("/api/v1/")) {
    version = "v1";
    rest = pathname.slice(8);
  } else if (pathname.startsWith("/apis/")) {
    const parts = pathname.split("/");
    group = parts[2];
    version = parts[3];
    rest = parts.slice(4).join("/");
  } else {
    return null;
  }

  rest = rest.replace(/\/$/, "");
  const parts = rest.split("/");
  const resource = parts[0] === "namespaces" && parts.length >= 3 ? parts[2] : parts[0];

  const kind = RESOURCE_KINDS[resource];
  if (!kind) return null;

  return { kind, apiVersion: group ? `${group}/${version}` : version };
}
