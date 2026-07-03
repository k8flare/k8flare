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
    restartPolicy?: "Always" | "OnFailure" | "Never";
    containers?: Array<{
      name: string;
      image: string;
      resources?: { requests?: Record<string, string> };
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
export async function createLease(env: Env, name: string, renewTime: string): Promise<void> {
  const lease: LeaseObject = {
    apiVersion: "coordination.k8s.io/v1",
    kind: "Lease",
    metadata: { name },
    spec: { holderIdentity: name, leaseDurationSeconds: NODE_LEASE_DURATION_SECONDS, renewTime },
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
export async function renewLease(env: Env, name: string, renewTime: string): Promise<void> {
  const lease: LeaseObject = {
    apiVersion: "coordination.k8s.io/v1",
    kind: "Lease",
    metadata: { name },
    spec: { holderIdentity: name, leaseDurationSeconds: NODE_LEASE_DURATION_SECONDS, renewTime },
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
