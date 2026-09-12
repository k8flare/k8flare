// Classifies kine keys/prefixes into the facet (or parent) that owns them.
//
// Resource-type scoping (namespaced vs cluster-scoped) mirrors real
// Kubernetes API discovery, which this apiserver doesn't yet expose as a
// programmatically shared table between Go and TS (see CLAUDE.md Go-first
// note; pkg/apiserver is out of scope for this phase). CLUSTER_SCOPED_RESOURCES
// is therefore a small, hand-maintained denylist: unknown resource types
// default to "namespaced" (the common case in Kubernetes), so a new
// namespaced kind registered elsewhere works correctly without this file
// being updated. Only newly-registered *cluster-scoped* kinds need adding
// here.
const CLUSTER_SCOPED_RESOURCES = new Set([
  "namespaces",
  "nodes",
  "runtimeclasses",
  "csidrivers",
  "csinodes",
  "resourceslices",
  "deviceclasses",
  "servicecidrs",
  "_internal",
  // k8flare.com/v1alpha1 Clusters (pkg/apis/k8flare/v1alpha1) -- k8flare's
  // own built-in group, cluster-scoped like every other kind here. Omitting
  // it was not benign: a key like "/registry/clusters/team-a" fell through
  // to the namespaced branch below, which read "team-a" as the namespace
  // and wrote the object into an ns/team-a facet, while a LIST of
  // "/registry/clusters/" fanned out across the REAL namespaces and so
  // returned only clusters whose name happened to match one (measured
  // 2026-07-27: GET returned the object, LIST did not). This is exactly the
  // drift this file's doc comment warns about; a table-driven generator
  // (cmd/k8flare-gen, from apidef.Table's Namespaced field) would remove
  // the failure mode entirely and is worth doing if more cluster-scoped
  // kinds are added.
  "clusters",
  // Standard Kubernetes cluster-scoped kinds not yet registered by this
  // apiserver (see url-mapping.ts's RESOURCE_KINDS) but included defensively
  // per CLAUDE.md rule 3 (upstream's real scoping, not a repo-specific guess)
  // so a future addition doesn't silently misroute into a namespace facet.
  "persistentvolumes",
  "storageclasses",
  "clusterroles",
  "clusterrolebindings",
  "certificatesigningrequests",
  "priorityclasses",
  "mutatingwebhookconfigurations",
  "validatingwebhookconfigurations",
  "apiservices",
  "customresourcedefinitions",
  "volumeattachments",
  "ingressclasses",
]);

const REGISTRY_PREFIX = "/registry/";

/** Facet name for the events-log facet (shared across all namespaces). */
export const EVENTS_FACET = "events-log";
/** Facet name for the ca-vault facet (CA keys + node-password hashes). */
export const CA_VAULT_FACET = "ca-vault";

/** Facet name for a given namespace's facet. */
export function namespaceFacet(namespace: string): string {
  return `ns/${namespace}`;
}

export type KeyClass =
  | { kind: "cluster" }
  | { kind: "namespace"; namespace: string; facet: string }
  | { kind: "events"; facet: string }
  | { kind: "ca-vault"; facet: string };

/** Classify a single fully-qualified key (e.g. "/registry/pods/default/foo" or "/ca/client-ca.crt"). */
export function classifyKey(key: string): KeyClass {
  if (key.startsWith("/ca/") || key.startsWith("/nodepasswords/")) {
    return { kind: "ca-vault", facet: CA_VAULT_FACET };
  }
  if (!key.startsWith(REGISTRY_PREFIX)) return { kind: "cluster" };

  const rest = key.slice(REGISTRY_PREFIX.length); // "pods/default/foo" | "nodes/foo"
  const firstSlash = rest.indexOf("/");
  const resource = firstSlash === -1 ? rest : rest.slice(0, firstSlash);

  if (resource === "events") return { kind: "events", facet: EVENTS_FACET };
  if (CLUSTER_SCOPED_RESOURCES.has(resource)) return { kind: "cluster" };
  if (firstSlash === -1) return { kind: "cluster" }; // malformed/no name segment at all -- keep local defensively

  const afterResource = rest.slice(firstSlash + 1); // "default/foo"
  const nsSlash = afterResource.indexOf("/");
  const namespace = nsSlash === -1 ? afterResource : afterResource.slice(0, nsSlash);
  if (!namespace) return { kind: "cluster" }; // malformed -- keep local defensively
  return { kind: "namespace", namespace, facet: namespaceFacet(namespace) };
}
