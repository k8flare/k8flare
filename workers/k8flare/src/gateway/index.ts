import {
  isKubeletProxyRequest,
  handleKubeletProxy,
  handleRemotedialConnect,
} from "./proxy/index.ts";
import { dwAuth, handleWatch } from "../k8s/index.ts";
import type { Env } from "../env.ts";
import { apiserverFetch } from "../loader/apiserver.ts";
import { handleNodes } from "../nodes/index.ts";
import { handleClustersAPI } from "../clusters/api.ts";
import { clusterEnv } from "../clusters/clusterenv.ts";
import { resolveCluster } from "../clusters/resolve.ts";
import { verifyClusterToken } from "../clusters/tokens.ts";

// Paths a cluster serves WITHOUT a token, matching what the default
// cluster has always exposed (version, the agent bootstrap trust hint,
// and the discovery/OpenAPI documents that are edge-served assets on
// the un-prefixed path).
function isUnauthenticatedPath(url: URL): boolean {
  const p = url.pathname;
  return (
    p === "/version" ||
    p === "/cacerts" ||
    p === "/api" ||
    p.startsWith("/openapi/") ||
    /^\/api\/v1$/.test(p) ||
    /^\/apis(\/[^/]+(\/[^/]+)?)?$/.test(p)
  );
}

// Watch streams are served in TS (Go WASM cannot stream), which means
// they bypass the Go apiserver's AuthzMiddleware -- so derived
// identities (X-Remote-User; the cluster token itself is
// system:masters and bypasses RBAC in Go too) are authorized here by
// asking the Go authorizer the same question via SubjectAccessReview.
// One extra apiserver round-trip per watch OPEN (not per event), and
// only for derived identities -- kubectl-as-admin, the KCM, and the
// kubelet pay nothing.
function watchResourceAttributes(
  pathname: string,
): { group: string; resource: string; namespace: string; name: string } | null {
  const parts = pathname.split("/").filter(Boolean);
  let group = "";
  let rest: string[];
  if (parts[0] === "api" && parts[1] === "v1") {
    rest = parts.slice(2);
  } else if (parts[0] === "apis" && parts.length >= 3) {
    group = parts[1];
    rest = parts.slice(3);
  } else {
    return null;
  }
  let namespace = "";
  if (rest[0] === "namespaces" && rest.length >= 3) {
    namespace = rest[1];
    rest = rest.slice(2);
  }
  if (!rest[0]) return null;
  return { group, resource: rest[0], namespace, name: rest[1] ?? "" };
}

async function authorizeWatchRBAC(req: Request, env: Env, url: URL): Promise<Response | null> {
  const remoteUser = req.headers.get("X-Remote-User");
  if (!remoteUser) return null;
  const forbidden = (message: string) =>
    Response.json(
      {
        kind: "Status",
        apiVersion: "v1",
        status: "Failure",
        message,
        reason: "Forbidden",
        code: 403,
      },
      { status: 403 },
    );
  const attrs = watchResourceAttributes(url.pathname);
  if (!attrs) return forbidden(`forbidden: cannot resolve watch path ${url.pathname}`);
  const groups = ["system:authenticated"];
  const remoteGroups = req.headers.get("X-Remote-Group");
  if (remoteGroups) groups.unshift(...remoteGroups.split(","));
  const sar = {
    apiVersion: "authorization.k8s.io/v1",
    kind: "SubjectAccessReview",
    spec: {
      user: remoteUser,
      groups,
      resourceAttributes: {
        verb: "watch",
        group: attrs.group,
        resource: attrs.resource,
        namespace: attrs.namespace,
        name: attrs.name,
      },
    },
  };
  const resp = await apiserverFetch(
    env,
    new Request("http://internal/apis/authorization.k8s.io/v1/subjectaccessreviews", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${env.K3S_TOKEN || "k8flare-dev-token"}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(sar),
    }),
  );
  const body = (await resp.json().catch(() => null)) as { status?: { allowed?: boolean } } | null;
  if (resp.ok && body?.status?.allowed) return null;
  return forbidden(`forbidden: User "${remoteUser}" cannot watch resource "${attrs.resource}"`);
}

// The consolidated Worker's public routing -- the former gateway Worker's
// fetch handler, with the cross-Worker service bindings replaced:
// APISERVER -> apiserverFetch (Loader dynamic worker), RUNTIME/NODES ->
// direct function calls, WATCHHUB -> the now-local DO binding.
export async function handleGateway(
  req: Request,
  outerEnv: Env,
  ctx: ExecutionContext,
): Promise<Response> {
  let url = new URL(req.url);

  // Cluster management API (admin-authenticated, clusters/adminauth.ts).
  // Never cluster-prefixed; doesn't collide with any k8s API path.
  if (url.pathname === "/clusters" || url.pathname.startsWith("/clusters/")) {
    return handleClustersAPI(req, outerEnv, ctx);
  }

  // Multi-cluster resolution: parse+strip /c/<id> (unknown id 404s
  // before any DO is touched); no prefix = the zero-config "default"
  // cluster. Everything below runs against a derived env whose DO
  // namespaces transparently retarget to this cluster's tree
  // (clusters/clusterenv.ts).
  const resolved = await resolveCluster(req, outerEnv, url);
  if (resolved instanceof Response) return resolved;
  const { cluster } = resolved;
  req = resolved.req;
  url = resolved.url;

  let env: Env;
  if (cluster.doName === "default") {
    // Default keeps its exact pre-multi-cluster auth semantics: the env
    // token, enforced downstream (Go AuthMiddleware, dwAuth, handleNodes).
    env = clusterEnv(outerEnv, cluster);
  } else {
    // Provisioned clusters authenticate AT THE DOOR against the
    // cluster's own token vault. This is what prevents cross-cluster
    // token reuse: downstream dwAuth-style checks compare against
    // env.K3S_TOKEN, so the derived env carries the verified presented
    // token -- and an unverified request never reaches them.
    const presented = await verifyClusterToken(req, outerEnv, cluster.doName);
    if (!presented && !isUnauthenticatedPath(url)) {
      return Response.json(
        {
          kind: "Status",
          apiVersion: "v1",
          status: "Failure",
          message: "Unauthorized",
          reason: "Unauthorized",
          code: 401,
        },
        { status: 401 },
      );
    }
    env = clusterEnv(outerEnv, cluster, presented ?? crypto.randomUUID());
  }

  // /internal/* was implicitly private pre-consolidation (reachable only
  // over service bindings to unrouted Workers; the Go handlers themselves
  // are unauthenticated). Now that everything shares the one public fetch
  // handler, the boundary is explicit: in-Worker callers (the Cluster
  // DO's node-lifecycle ping) go through apiserverFetch directly and
  // never enter this handler; from the outside these routes exist only
  // behind the cluster token (the same trust level as every other API
  // path), and without it they 404 rather than advertise themselves.
  if (url.pathname.startsWith("/internal/")) {
    if (!dwAuth(req, env)) {
      return new Response("not found", { status: 404 });
    }
    return apiserverFetch(env, req);
  }
  // The wasm chunk supply channel (run_worker_first) is Loader-only.
  if (url.pathname.startsWith("/wasm/")) {
    return new Response("not found", { status: 404 });
  }

  // Kubelet proxy requests (pods/log, pods/exec, etc.)
  if (isKubeletProxyRequest(url.pathname)) {
    return handleKubeletProxy(req, env, url, (r) => apiserverFetch(env, r));
  }

  // Standard Kubernetes pods/proxy subresource, backed by the
  // Pod-on-Containers backend: GET/... /api/v1/namespaces/{ns}/pods/
  // {pod}/proxy/{path} -> nodes handler -> NodeVM DO. Using the upstream
  // API surface (what `kubectl proxy` / `kubectl get --raw .../proxy/...`
  // speak) keeps the route inside the API's URL namespace and inherits
  // the pods/proxy verb semantics the moment a real RBAC authorizer
  // lands. Token-gated like every other API path; handleNodes re-checks
  // the same Authorization header on its side too.
  const podProxyMatch = url.pathname.match(
    /^\/api\/v1\/namespaces\/([^/]+)\/pods\/([^/]+)\/proxy(\/.*)?$/,
  );
  if (podProxyMatch) {
    if (!dwAuth(req, env)) {
      return new Response("unauthorized", { status: 401 });
    }
    const [, ns, pod, rest] = podProxyMatch;
    const target = new URL(req.url);
    target.pathname = `/podproxy/${ns}/${pod}${rest ?? "/"}`;
    return handleNodes(new Request(target.toString(), req), env);
  }

  // nodes/{name}/proxy/{path}: kubelet endpoints (stats/summary,
  // metrics, metrics/resource, pods) for Containers-backed per-Pod
  // nodes -- the standard path metrics scrapers use. Port 10256 is
  // cmd/agent's plain-HTTP front for the authenticated kubelet API
  // (k3s pins --read-only-port=0 as a CLI flag, which beats any
  // kubelet config drop-in, so 10255 never listens on these nodes).
  // The node object's backend label (stamped at kubelet registration)
  // decides the branch; BYO nodes fall through to the apiserver.
  const nodeProxyMatch = url.pathname.match(/^\/api\/v1\/nodes\/([^/]+)\/proxy(\/.*)$/);
  if (nodeProxyMatch) {
    if (!dwAuth(req, env)) {
      return new Response("unauthorized", { status: 401 });
    }
    const [, nodeName, rest] = nodeProxyMatch;
    const token = env.K3S_TOKEN || "k8flare-dev-token";
    const nodeResp = await apiserverFetch(
      env,
      new Request(`http://internal/api/v1/nodes/${nodeName}`, {
        headers: { Authorization: `Bearer ${token}` },
      }),
    );
    if (nodeResp.ok) {
      const node: any = await nodeResp.json();
      if (node.metadata?.labels?.["k8flare.com/backend"] === "containers") {
        const target = new URL(req.url);
        target.pathname = `/kubelet/${nodeName}/10256${rest}`;
        return handleNodes(
          new Request(target.toString(), {
            method: req.method,
            headers: { Authorization: `Bearer ${token}` },
          }),
          env,
        );
      }
    }
  }

  // Handle watch requests in JS (Go WASM cannot do streaming)
  if (url.searchParams.get("watch") === "true") {
    const denied = await authorizeWatchRBAC(req, env, url);
    if (denied) return denied;
    return handleWatch(req, env, url, ctx);
  }

  // remotedialer tunnel endpoint — k3s agent connects here via WebSocket.
  // We use Workers VPC for kubelet communication, so this is a stub that
  // keeps the connection alive so the agent completes its bootstrap.
  if (url.pathname === "/v1-k3s/connect") {
    return handleRemotedialConnect(req, env);
  }

  // Operator-facing nodes surface (bootstrap/health pokes; token-gated
  // inside handleNodes) -- the former nodes Worker's workers.dev root,
  // absorbed under /nodes/* (the same convention smoke-nodes.yml's CI
  // proxy already used, so the workflow drops its proxy, not its paths).
  if (url.pathname === "/nodes" || url.pathname.startsWith("/nodes/")) {
    const target = new URL(req.url);
    target.pathname = url.pathname.slice("/nodes".length) || "/";
    return handleNodes(new Request(target.toString(), req), env);
  }
  if (url.pathname.startsWith("/kubelet/") || url.pathname.startsWith("/podproxy/")) {
    return handleNodes(req, env);
  }

  // All other requests go to the Go apiserver dynamic worker (cold-start
  // retry absorber lives inside apiserverFetch).
  return apiserverFetch(env, req);
}
