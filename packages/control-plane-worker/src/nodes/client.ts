// Thin REST client for pkg/apiserver, used by the nodes subtree to read
// Nodes/Pods (NodeVM scheduling, pod proxy target resolution).
// Mirrors pkg/cacert/flannel.go's getNodePodCIDR pattern (plain fetch +
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
import { apiserverFetch } from "../loader.ts";

function getToken(env: Env): string {
  return env.ADMIN_TOKEN;
}

// Post-consolidation this goes straight to the apiserver dynamic worker
// (loader/apiserver.ts), not through the public routing.
async function apiFetch(env: Env, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers);
  headers.set("Authorization", `Bearer ${getToken(env)}`);
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  return apiserverFetch(env, new Request(`http://apiserver.internal${path}`, { ...init, headers }));
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

export interface PodObject {
  apiVersion: "v1";
  kind: "Pod";
  metadata: {
    name: string;
    namespace: string;
    uid?: string;
    deletionTimestamp?: string;
    // Read by listPendingContainersPods to learn the size tier
    // pkg/apiserver/computeclass.go's AssignContainersNode already
    // picked at admission (NodeVMTierAnnotation there).
    annotations?: Record<string, string>;
  };
  spec?: {
    nodeName?: string;
    schedulerName?: string;
    // Read by listPendingContainersPods: AssignContainersNode pins the
    // Pod's dedicated (not-yet-existing) Node via the standard
    // kubernetes.io/hostname key here, alongside the static
    // k8flare.com/backend=containers marker MutatePodForComputeClass
    // sets on every Pod-on-Containers Pod.
    nodeSelector?: Record<string, string>;
    restartPolicy?: "Always" | "OnFailure" | "Never";
    containers?: Array<{
      name: string;
      image: string;
      resources?: { requests?: Record<string, string>; limits?: Record<string, string> };
      // Task #13 (podproxy.ts): the default target port for pods/proxy
      // when the caller doesn't specify one via pods/{name}:{port}/proxy.
      ports?: Array<{ containerPort: number; name?: string; protocol?: string }>;
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

// The static marker pkg/apiserver/computeclass.go's MutatePodForComputeClass
// stamps on every Pod-on-Containers Pod (containersBackendLabel there),
// and the standard nodeSelector key AssignContainersNode pins each such
// Pod's dedicated Node name to (kubernetes.io/hostname). Both need to
// match exactly what admission wrote for listPendingContainersPods below
// to find the right Pods.
const CONTAINERS_BACKEND_LABEL = "k8flare.com/backend";
const HOSTNAME_SELECTOR_KEY = "kubernetes.io/hostname";

/**
 * Lists Pods admission has already pinned to a dedicated (not-yet-
 * existing) Node -- pkg/apiserver/computeclass.go's AssignContainersNode
 * -- but this DO hasn't started a NodeVM for yet (this repo never
 * touches spec.nodeName itself; the real kube-scheduler does, once that
 * Node exists and is Ready, so `spec.nodeName=""` here just narrows to
 * "not yet bound", same field selector the old binder used, kept for
 * the same reason: pkg/apiserver/store.go serves it as a real field
 * selector). The nodeSelector/annotation check happens client-side --
 * this apiserver doesn't support filtering on nodeSelector map keys via
 * fieldSelector.
 */
export async function listPendingContainersPods(env: Env): Promise<PodObject[]> {
  const resp = await apiFetch(
    env,
    `/api/v1/pods?fieldSelector=${encodeURIComponent("spec.nodeName=")}`,
  );
  if (!resp.ok) throw new Error(`listPendingContainersPods: ${resp.status} ${await resp.text()}`);
  const data = (await resp.json()) as { items?: PodObject[] };
  return (data.items ?? []).filter(
    (p) =>
      p.spec?.nodeSelector?.[CONTAINERS_BACKEND_LABEL] === "containers" &&
      !!p.spec?.nodeSelector?.[HOSTNAME_SELECTOR_KEY] &&
      !p.metadata.deletionTimestamp,
  );
}

export async function getPod(env: Env, namespace: string, name: string): Promise<PodObject | null> {
  const resp = await apiFetch(env, `/api/v1/namespaces/${namespace}/pods/${name}`);
  if (resp.status === 404) return null;
  if (!resp.ok) throw new Error(`getPod ${namespace}/${name}: ${resp.status} ${await resp.text()}`);
  return (await resp.json()) as PodObject;
}

// ---- Task #13: virtual kube-proxy (ClusterIP -> backing Pod resolution) ----
//
// The ClusterIP -> Service -> EndpointSlice resolution itself moved into
// Go (pkg/apiserver/vkubeproxy.go's ResolveVKubeProxyTarget), which runs
// in-process against this apiserver's own ResourceStore -- one HTTP hop
// from here instead of the two separate round trips (a Service lookup,
// then an EndpointSlice lookup) this file used to make. This module just
// carries the result back to podproxy.ts's handleVKubeProxy, which still
// owns the final hop (forwarding to the resolved Pod's NodeVM Durable
// Object -- something only the shell Worker can do, per forwardToPod's
// doc comment).

export interface VKubeProxyTarget {
  podUID: string;
  containerPort: number;
}

/**
 * GET /internal/vkubeproxy-resolve?ip=&port= (pkg/apiserver/vkubeproxy.go).
 * Returns null if no Service/ready-endpoint match was found (a stale/
 * unknown ClusterIP, or no ready backing Pod yet -- the caller's proxied
 * connection should fail, not synthesize a fake target).
 */
export async function resolveVKubeProxyTarget(
  env: Env,
  clusterIP: string,
  port: number,
): Promise<VKubeProxyTarget | null> {
  const resp = await apiFetch(
    env,
    `/internal/vkubeproxy-resolve?ip=${encodeURIComponent(clusterIP)}&port=${port}`,
  );
  if (!resp.ok) {
    if (resp.status === 502) return null; // no Service/ready-endpoint match
    throw new Error(
      `resolveVKubeProxyTarget ${clusterIP}:${port}: ${resp.status} ${await resp.text()}`,
    );
  }
  return (await resp.json()) as VKubeProxyTarget;
}

export async function planNodeVMs(env: Env, body: unknown): Promise<{
  boots: Array<{ uid: string; namespace: string; podName: string; podUID: string; nodeName: string; tier: "small" | "medium" | "large"; claimed: boolean; meshConnectorId?: string }>;
  teardowns: Array<{ namespace: string; podName: string; podUID: string; nodeName: string; tier: "small" | "medium" | "large"; bound: boolean; started: boolean; bootedAt: number; meshConnectorId?: string }>;
  markBound: string[];
  hasWork: boolean;
}> {
  const resp = await apiFetch(env, "/internal/nodes/vm-plan", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!resp.ok) throw new Error(`planNodeVMs: ${resp.status} ${await resp.text()}`);
  return resp.json();
}

export async function deleteNode(env: Env, name: string): Promise<void> {
  const resp = await apiFetch(env, `/api/v1/nodes/${name}`, { method: "DELETE" });
  if (!resp.ok && resp.status !== 404) {
    throw new Error(`deleteNode ${name}: ${resp.status} ${await resp.text()}`);
  }
}

export async function mergePodEnv(env: Env, namespace: string, name: string, vars: Record<string, string>): Promise<void> {
  if (!Object.keys(vars).length) return;
  const resp = await apiFetch(env, `/api/v1/namespaces/${namespace}/pods/${name}`);
  if (resp.status === 404) return;
  if (!resp.ok) throw new Error(`mergePodEnv get ${namespace}/${name}: ${resp.status} ${await resp.text()}`);
  const pod = await resp.json();
  const merged = await apiFetch(env, "/internal/pods/merge-env", {
    method: "POST",
    body: JSON.stringify({ pod, vars }),
  });
  if (!merged.ok) throw new Error(`mergePodEnv merge ${namespace}/${name}: ${merged.status} ${await merged.text()}`);
  const put = await apiFetch(env, `/api/v1/namespaces/${namespace}/pods/${name}`, {
    method: "PUT",
    body: await merged.text(),
  });
  if (!put.ok) throw new Error(`mergePodEnv put ${namespace}/${name}: ${put.status} ${await put.text()}`);
}
