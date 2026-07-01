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
