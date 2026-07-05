// Thin REST client for pkg/apiserver, used by virtualnode.ts/podcontainer.ts
// to register the virtual Node, renew its Lease, and read/write Pods bound to
// it. Mirrors pkg/cacert/flannel.go's getNodePodCIDR pattern (plain fetch +
// Bearer token + JSON decode) rather than pulling in a generated client --
// workers/nodes is a TypeScript Worker, so there is no client-go available,
// and the REST surface it needs is small (a handful of fixed paths).
//
// Every request needs "kind"/"apiVersion" in its body (pkg/apiserver/
// handler.go's decodeBody resolves the Go type purely from those fields, no
// default GVK) and an Authorization header (every route except
// pkg/apiserver's /internal/* requires it) -- see docs/control-plane-
// architecture.md and the apidef/table.go research this file's callers were
// designed against.
import type { Env } from "./env.ts";

const DEV_TOKEN = "k8flare-dev-token"; // fallback, matches every other Worker in this repo (see CLAUDE.md)

function getToken(env: Env): string {
  return env.K3S_TOKEN || DEV_TOKEN;
}

async function apiFetch(env: Env, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers);
  headers.set("Authorization", `Bearer ${getToken(env)}`);
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  return env.APISERVER.fetch(`http://apiserver.internal${path}`, { ...init, headers });
}

/** Minimal corev1.Node shape this Worker needs to read/write. Fields beyond these pass through untouched when read-modify-write is used. */
export interface NodeObject {
  apiVersion: "v1";
  kind: "Node";
  metadata: { name: string; labels?: Record<string, string>; annotations?: Record<string, string> };
  spec?: { podCIDR?: string; podCIDRs?: string[]; taints?: unknown[]; unschedulable?: boolean };
  status?: {
    capacity?: Record<string, string>;
    allocatable?: Record<string, string>;
    conditions?: Array<{
      type: string;
      status: string;
      reason?: string;
      message?: string;
      lastHeartbeatTime?: string;
      lastTransitionTime?: string;
    }>;
    addresses?: Array<{ type: string; address: string }>;
    nodeInfo?: Record<string, string>;
  };
}

export interface LeaseObject {
  apiVersion: "coordination.k8s.io/v1";
  kind: "Lease";
  metadata: { name: string };
  spec: { holderIdentity?: string; leaseDurationSeconds?: number; renewTime?: string };
}

export interface PodObject {
  apiVersion: "v1";
  kind: "Pod";
  metadata: { name: string; namespace: string; uid?: string; deletionTimestamp?: string };
  spec?: {
    nodeName?: string;
    schedulerName?: string;
    restartPolicy?: "Always" | "OnFailure" | "Never";
    containers?: Array<{
      name: string;
      image: string;
      resources?: { requests?: Record<string, string>; limits?: Record<string, string> };
      // Phase 8 (R2 PV/PVC backend): only volumeMounts[].name is needed, to
      // cross-reference against spec.volumes[].name below and find which
      // (if any) of this Pod's volumes is actually mounted by its one
      // supported container -- mountPath itself is unused (R2 access is
      // injected as env vars for direct S3-SDK use, not a real filesystem
      // mount -- see README.md's "Volumes (R2 PV/PVC)" section).
      volumeMounts?: Array<{ name: string; mountPath: string }>;
    }>;
    // Phase 8 (R2 PV/PVC backend). Only the persistentVolumeClaim volume
    // source is understood; any other volume type (emptyDir, configMap,
    // etc.) is simply not matched by podPersistentVolumeClaimRef (client.ts)
    // and has no effect here -- workers/nodes v1 does not provide real
    // filesystem-mounted volumes of any kind (see README.md).
    volumes?: Array<{
      name: string;
      persistentVolumeClaim?: { claimName: string };
    }>;
  };
  status?: {
    phase?: string;
    podIP?: string;
    podIPs?: Array<{ ip: string }>;
    hostIP?: string;
    conditions?: Array<{ type: string; status: string; lastTransitionTime?: string }>;
    containerStatuses?: Array<{
      name: string;
      image: string;
      imageID: string;
      containerID: string;
      ready: boolean;
      started: boolean;
      restartCount: number;
      state?: Record<string, unknown>;
    }>;
  };
}

export interface PodList {
  items: PodObject[];
}

/** GET /api/v1/nodes/{name}. Returns null on 404. */
export async function getNode(env: Env, name: string): Promise<NodeObject | null> {
  const resp = await apiFetch(env, `/api/v1/nodes/${name}`);
  if (resp.status === 404) return null;
  if (!resp.ok) throw new Error(`getNode ${name}: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

/** POST /api/v1/nodes. Ignores 409 (already exists) -- idempotent create, same shape as pkg/apiserver/bootstrap.go's BootstrapCluster. */
export async function createNode(env: Env, node: NodeObject): Promise<void> {
  const resp = await apiFetch(env, "/api/v1/nodes", { method: "POST", body: JSON.stringify(node) });
  if (!resp.ok && resp.status !== 409) {
    throw new Error(`createNode ${node.metadata.name}: ${resp.status} ${await resp.text()}`);
  }
}

/** PUT /api/v1/nodes/{name}/status. */
export async function updateNodeStatus(env: Env, node: NodeObject): Promise<void> {
  const resp = await apiFetch(env, `/api/v1/nodes/${node.metadata.name}/status`, {
    method: "PUT",
    body: JSON.stringify(node),
  });
  if (!resp.ok)
    throw new Error(`updateNodeStatus ${node.metadata.name}: ${resp.status} ${await resp.text()}`);
}

const LEASE_NS = "kube-node-lease";

// coordinationv1.LeaseSpec.RenewTime is a *metav1.MicroTime, whose
// UnmarshalJSON requires exactly microsecond (6-digit) fractional-second
// precision (Go's fixed-width time.RFC3339Micro layout) -- unlike
// metav1.Time (used for every other timestamp this file sends, e.g. Node/Pod
// .status.conditions), which parses any valid RFC3339 string regardless of
// fractional digit count. JavaScript's `Date.toISOString()` only produces
// millisecond (3-digit) precision, so passing it straight through fails
// server-side with "cannot parse \".839Z\" as \".000000\"" -- found by
// actually running this against wrangler dev (CLAUDE.md rule 2: this was NOT
// caught by reading either side's code first), not a hypothetical. Padding
// with 3 zeros produces a valid 6-digit fraction.
function toMicroTime(date: Date): string {
  return date.toISOString().replace("Z", "000Z");
}

/** GET .../namespaces/kube-node-lease/leases/{name}. Returns null on 404. */
export async function getLease(env: Env, name: string): Promise<LeaseObject | null> {
  const resp = await apiFetch(
    env,
    `/apis/coordination.k8s.io/v1/namespaces/${LEASE_NS}/leases/${name}`,
  );
  if (resp.status === 404) return null;
  if (!resp.ok) throw new Error(`getLease ${name}: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

/** POST .../leases (create). Ignores 409 -- caller falls back to renewLease. */
export async function createLease(env: Env, name: string, renewTime: Date): Promise<void> {
  const lease: LeaseObject = {
    apiVersion: "coordination.k8s.io/v1",
    kind: "Lease",
    metadata: { name },
    spec: {
      holderIdentity: name,
      leaseDurationSeconds: NODE_LEASE_DURATION_SECONDS,
      renewTime: toMicroTime(renewTime),
    },
  };
  const resp = await apiFetch(env, `/apis/coordination.k8s.io/v1/namespaces/${LEASE_NS}/leases`, {
    method: "POST",
    body: JSON.stringify(lease),
  });
  if (!resp.ok && resp.status !== 409) {
    throw new Error(`createLease ${name}: ${resp.status} ${await resp.text()}`);
  }
}

/** PUT .../leases/{name}: whole-object renew (matches nodelifecycle_test.go's mustCreateLease shape). */
export async function renewLease(env: Env, name: string, renewTime: Date): Promise<void> {
  const lease: LeaseObject = {
    apiVersion: "coordination.k8s.io/v1",
    kind: "Lease",
    metadata: { name },
    spec: {
      holderIdentity: name,
      leaseDurationSeconds: NODE_LEASE_DURATION_SECONDS,
      renewTime: toMicroTime(renewTime),
    },
  };
  const resp = await apiFetch(
    env,
    `/apis/coordination.k8s.io/v1/namespaces/${LEASE_NS}/leases/${name}`,
    {
      method: "PUT",
      body: JSON.stringify(lease),
    },
  );
  if (!resp.ok) throw new Error(`renewLease ${name}: ${resp.status} ${await resp.text()}`);
}

// Matches real kubelet's default (pkg/kubelet/kubelet.go's
// NodeLeaseDurationSeconds default = 40s, renew every 1/4 of that = 10s --
// see nodeLeaseRenewIntervalFraction). Comfortably under
// pkg/apiserver/nodelifecycle.go's real-upstream-sourced
// NodeMonitorGracePeriod (measured 50s), same margin a real kubelet gets.
export const NODE_LEASE_DURATION_SECONDS = 40;
export const NODE_LEASE_RENEW_INTERVAL_MS = 10_000;

/** GET /api/v1/pods?fieldSelector=spec.nodeName=<name> (all namespaces -- pkg/apiserver/store.go supports spec.nodeName as a real field selector). */
export async function listPodsForNode(env: Env, nodeName: string): Promise<PodObject[]> {
  const resp = await apiFetch(
    env,
    `/api/v1/pods?fieldSelector=${encodeURIComponent(`spec.nodeName=${nodeName}`)}`,
  );
  if (!resp.ok) throw new Error(`listPodsForNode ${nodeName}: ${resp.status} ${await resp.text()}`);
  const list: PodList = await resp.json();
  return list.items ?? [];
}

/** PUT /api/v1/namespaces/{ns}/pods/{name}/status. */
export async function updatePodStatus(env: Env, pod: PodObject): Promise<void> {
  const resp = await apiFetch(
    env,
    `/api/v1/namespaces/${pod.metadata.namespace}/pods/${pod.metadata.name}/status`,
    {
      method: "PUT",
      body: JSON.stringify(pod),
    },
  );
  if (!resp.ok)
    throw new Error(
      `updatePodStatus ${pod.metadata.namespace}/${pod.metadata.name}: ${resp.status} ${await resp.text()}`,
    );
}

/** DELETE /api/v1/namespaces/{ns}/pods/{name}. Ignores 404 (already gone). */
export async function deletePod(env: Env, namespace: string, name: string): Promise<void> {
  const resp = await apiFetch(env, `/api/v1/namespaces/${namespace}/pods/${name}`, {
    method: "DELETE",
  });
  if (!resp.ok && resp.status !== 404) {
    throw new Error(`deletePod ${namespace}/${name}: ${resp.status} ${await resp.text()}`);
  }
}

// ---- Phase 8: R2 PV/PVC backend ----
//
// workers/nodes never talks to pkg/apiserver/pvcbind.go's
// PersistentVolume/StorageClass logic directly, and never parses a PV's
// spec.csi.volumeAttributes itself -- it only needs to know "does this Pod
// reference a PVC, and if so, by what claim name," then hands that name to
// /internal/mint-r2-credentials (pkg/apiserver/r2handlers.go), which alone
// owns the PVC -> PV -> bucket/prefix lookup. This keeps the CSI attribute
// schema a private contract between pvcbind.go and r2handlers.go.

/** A Pod's PersistentVolumeClaim reference, if it has one mounted by its (one supported) container. */
export interface PodPVCRef {
  volumeName: string;
  claimName: string;
}

/**
 * Finds the (at most one, per workers/nodes v1's single-container support)
 * persistentVolumeClaim-backed volume actually mounted by pod's container,
 * cross-referencing spec.containers[0].volumeMounts against spec.volumes by
 * name -- a volume merely listed in spec.volumes but never mounted is not
 * returned, matching how a real kubelet only wires up volumes a container
 * actually references.
 */
export function podPersistentVolumeClaimRef(pod: PodObject): PodPVCRef | undefined {
  const mountedNames = new Set((pod.spec?.containers?.[0]?.volumeMounts ?? []).map((m) => m.name));
  for (const vol of pod.spec?.volumes ?? []) {
    if (vol.persistentVolumeClaim && mountedNames.has(vol.name)) {
      return { volumeName: vol.name, claimName: vol.persistentVolumeClaim.claimName };
    }
  }
  return undefined;
}

/** Response shape of POST /internal/mint-r2-credentials (pkg/apiserver/r2handlers.go's mintR2CredentialsResponse). */
export interface R2Credential {
  accessKeyId: string;
  secretAccessKey: string;
  sessionToken: string;
  bucket: string;
  endpoint: string;
  prefix: string;
  expiresAt: string; // RFC3339, from Go's encoding/json marshaling of time.Time
}

/**
 * POST /internal/mint-r2-credentials: mints a fresh, prefix-scoped R2
 * Temporary Access Credential for claimName in namespace. Returns undefined
 * (rather than throwing) on any non-2xx response -- the claim not existing,
 * not yet bound, or R2 not being configured are all routine "this Pod can't
 * get R2 access right now" outcomes the caller (virtualnode.ts) turns into
 * a failed Pod with a clear reason, not an unhandled exception.
 */
export async function mintR2Credentials(
  env: Env,
  namespace: string,
  claimName: string,
  ttlSeconds?: number,
): Promise<R2Credential | undefined> {
  const resp = await apiFetch(env, "/internal/mint-r2-credentials", {
    method: "POST",
    body: JSON.stringify({ namespace, claimName, ttlSeconds }),
  });
  if (!resp.ok) return undefined;
  return resp.json();
}

/** Lists pods that have no node assigned yet (the binder's work queue). */
export async function listUnscheduledPods(env: Env): Promise<PodObject[]> {
  const resp = await apiFetch(
    env,
    `/api/v1/pods?fieldSelector=${encodeURIComponent("spec.nodeName=")}`,
  );
  if (!resp.ok) throw new Error(`listUnscheduledPods: ${resp.status} ${await resp.text()}`);
  const data = (await resp.json()) as { items?: PodObject[] };
  return data.items ?? [];
}

export async function getPod(env: Env, namespace: string, name: string): Promise<PodObject | null> {
  const resp = await apiFetch(env, `/api/v1/namespaces/${namespace}/pods/${name}`);
  if (resp.status === 404) return null;
  if (!resp.ok) throw new Error(`getPod ${namespace}/${name}: ${resp.status} ${await resp.text()}`);
  return (await resp.json()) as PodObject;
}

/**
 * Binds a pod to a node via the official Binding subresource -- the same
 * API the real kube-scheduler uses (POST pods/{name}/binding), no
 * spec.nodeName back-door writes.
 */
export async function bindPod(
  env: Env,
  namespace: string,
  name: string,
  nodeName: string,
): Promise<void> {
  const resp = await apiFetch(env, `/api/v1/namespaces/${namespace}/pods/${name}/binding`, {
    method: "POST",
    body: JSON.stringify({
      apiVersion: "v1",
      kind: "Binding",
      metadata: { name, namespace },
      target: { apiVersion: "v1", kind: "Node", name: nodeName },
    }),
  });
  if (!resp.ok)
    throw new Error(
      `bindPod ${namespace}/${name} -> ${nodeName}: ${resp.status} ${await resp.text()}`,
    );
}

/** Records a scheduling Event with the standard shape kubectl describe shows. */
export async function createSchedulingEvent(
  env: Env,
  pod: PodObject,
  reason: "Scheduled" | "FailedScheduling",
  message: string,
): Promise<void> {
  const ns = pod.metadata.namespace;
  const now = new Date().toISOString();
  const resp = await apiFetch(env, `/api/v1/namespaces/${ns}/events`, {
    method: "POST",
    body: JSON.stringify({
      apiVersion: "v1",
      kind: "Event",
      metadata: { generateName: `${pod.metadata.name}.`, namespace: ns },
      involvedObject: {
        apiVersion: "v1",
        kind: "Pod",
        namespace: ns,
        name: pod.metadata.name,
        uid: pod.metadata.uid,
      },
      reason,
      message,
      type: reason === "Scheduled" ? "Normal" : "Warning",
      source: { component: "cf-containers-scheduler" },
      firstTimestamp: now,
      lastTimestamp: now,
      count: 1,
    }),
  });
  if (!resp.ok) console.log(`createSchedulingEvent: ${resp.status} ${await resp.text()}`);
}

export async function deleteNode(env: Env, name: string): Promise<void> {
  const resp = await apiFetch(env, `/api/v1/nodes/${name}`, { method: "DELETE" });
  if (!resp.ok && resp.status !== 404) {
    throw new Error(`deleteNode ${name}: ${resp.status} ${await resp.text()}`);
  }
}
