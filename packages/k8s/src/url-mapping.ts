import { RESOURCE_KINDS } from "./gen/resource-kinds.gen.ts";

// Real Kubernetes stores true CustomResourceDefinition-backed types in etcd
// as "/registry/<plural>.<group>/...", not "/registry/<plural>/..." --
// the group suffix is what lets a CRD's plural name collide with an
// unrelated built-in resource without colliding in storage. This repo's Go
// apiserver's built-in groups (apps, batch, ...) don't use that suffix
// (matching upstream, which only suffixes genuine CRDs), so the group
// alone can't tell us which convention applies. CRD_GROUPS is therefore a
// small hand-maintained list of this repo's groups that DO use the
// suffixed convention -- mirrors keyspace.ts's CLUSTER_SCOPED_RESOURCES
// denylist pattern. See packages/dynamic-worker/src/constants.ts's
// DW_PREFIX / packages/worker-trigger/src/constants.ts's WT_PREFIX, which
// this list must stay in sync with (duplicated here rather than imported
// to avoid packages/k8s depending on packages/dynamic-worker+worker-trigger,
// which already depend on packages/k8s).
const CRD_GROUPS = new Set(["k8flare.com"]);

/**
 * Map a Kubernetes API URL pathname to a kine storage prefix.
 *
 * Examples:
 *   /api/v1/namespaces                                        -> /registry/namespaces/
 *   /api/v1/namespaces/{ns}/pods                              -> /registry/pods/{ns}/
 *   /api/v1/pods                                              -> /registry/pods/
 *   /apis/coordination.k8s.io/v1/namespaces/{ns}/leases       -> /registry/leases/{ns}/
 *   /apis/k8flare.com/v1alpha1/namespaces/{ns}/dynamicworkers -> /registry/dynamicworkers.k8flare.com/{ns}/
 */
export function urlToStoragePrefix(pathname: string): string | null {
  let path: string;
  let group = "";
  if (pathname.startsWith("/api/v1/")) {
    path = pathname.slice(8); // strip "/api/v1/"
  } else if (pathname.startsWith("/apis/")) {
    const parts = pathname.split("/");
    // /apis/{group}/{version}/...  ->  parts[4..]
    group = parts[2];
    path = parts.slice(4).join("/");
  } else {
    return null;
  }

  // Remove trailing slash
  path = path.replace(/\/$/, "");

  const parts = path.split("/");
  const resourceSuffix = CRD_GROUPS.has(group) ? "." + group : "";

  // namespaces/{ns}/{resource} -> /registry/{resource}/{ns}/
  if (parts[0] === "namespaces" && parts.length >= 3) {
    return "/registry/" + parts[2] + resourceSuffix + "/" + parts[1] + "/";
  }

  // {resource} -> /registry/{resource}/
  if (parts.length === 1 && parts[0] !== "") {
    return "/registry/" + parts[0] + resourceSuffix + "/";
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
