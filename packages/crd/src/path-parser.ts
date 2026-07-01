import type { ParsedPath } from "./types.ts";

/**
 * Parse a custom API group path into its components.
 *
 * Supported patterns:
 *   namespaces/{ns}/{resource}[/{name}[/{sub}]]
 *   {resource}  (cluster-wide list)
 *
 * Returns null if the path does not match any known pattern.
 */
export function parseCustomGroupPath(path: string): ParsedPath | null {
  const p = path.replace(/\/$/, "");
  if (!p) return null;
  const parts = p.split("/");
  // namespaces/{ns}/{resource}[/{name}[/{sub}]]
  if (parts[0] === "namespaces" && parts.length >= 3) {
    return {
      namespace: parts[1],
      resource: parts[2],
      name: parts[3] || "",
      subresource: parts[4] || "",
    };
  }
  // {resource} — all-namespace list
  if (parts.length === 1) {
    return { namespace: "", resource: parts[0], name: "", subresource: "" };
  }
  return null;
}
