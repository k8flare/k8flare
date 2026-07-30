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
import { handleClustersInternalAPI } from "../clusters/internalapi.ts";
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
    isHealthPath(p) ||
    p === "/version" ||
    p === "/cacerts" ||
    p === "/api" ||
    p.startsWith("/openapi/") ||
    /^\/api\/v1$/.test(p) ||
    /^\/apis(\/[^/]+(\/[^/]+)?)?$/.test(p)
  );
}

// The health probes, which an uptime monitor must be able to reach without
// a cluster token. Answered here in the shell rather than passed through to
// the Go apiserver's own /healthz (discovery.go): reaching that one costs a
// dynamic-worker load of a 65MB module, and an unauthenticated path on a
// public *.workers.dev URL is exactly where you do not want that to be
// driveable by anyone.
//
// Be precise about what a 200 here asserts: the Worker is routable and its
// script loaded. It does NOT assert that storage is reachable, that the
// cluster exists, or that any control-plane component is healthy -- those
// need a token, because probing them costs real work.
function isHealthPath(p: string): boolean {
  return p === "/healthz" || p === "/livez" || p === "/readyz";
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
// The /openapi/v2 media types, mirroring upstream's own table
// (k8s.io/kube-openapi pkg/handler/handler.go RegisterOpenAPIVersionedService).
// Two subtleties there are load-bearing and were both found the hard way:
//
//   - The subtype client-go asks for (DiscoveryClient.OpenAPISchema sends
//     the "@" one) is NOT the subtype the reply is labelled with. "@" is
//     not a legal mime token, so echoing it back makes client-go's
//     transformResponse fail at mime.ParseMediaType with "unexpected
//     content after media subtype" -- a different failure, not a fix.
//   - Vary: Accept, because one URL now has two bodies and this response
//     is edge-cacheable.
const OPENAPI_V2_ACCEPTED = [
  { subtype: "json", asset: "/openapi/v2", contentType: "application/json" },
  {
    subtype: "com.github.proto-openapi.spec.v2@v1.0+protobuf",
    asset: "/openapi/v2.pb",
    contentType: "application/com.github.proto-openapi.spec.v2.v1.0+protobuf",
  },
  {
    subtype: "com.github.proto-openapi.spec.v2.v1.0+protobuf",
    asset: "/openapi/v2.pb",
    contentType: "application/com.github.proto-openapi.spec.v2.v1.0+protobuf",
  },
];

// Serves the two encodings of the same generated document (cmd/k8flare-gen/
// openapi.go): assets "v2" (JSON) and "v2.pb" (protobuf). Both are fetched
// through the ASSETS binding, which does not re-enter this Worker, so the
// .pb file is never reachable at its own URL.
//
// Clauses are matched in the order the client listed them rather than by
// q-value like upstream's goautoneg. Every real client here (kubectl,
// client-go, browsers) sends its clauses in preference order already, so
// the extra sort would not change any outcome.
async function serveOpenAPIV2(req: Request, env: Env, url: URL): Promise<Response> {
  if (url.pathname !== "/openapi/v2") {
    return new Response("not found", { status: 404 });
  }
  const accept = req.headers.get("Accept") || "*/*";
  for (const clause of accept.split(",")) {
    const [type, subtype] = clause.split(";")[0].trim().split("/");
    const match = OPENAPI_V2_ACCEPTED.find(
      (a) => (type === "application" || type === "*") && (subtype === a.subtype || subtype === "*"),
    );
    if (!match) continue;
    const resp = await env.ASSETS.fetch(new Request(new URL(match.asset, url.origin)));
    if (!resp.ok) return resp;
    const headers = new Headers(resp.headers);
    headers.set("Content-Type", match.contentType);
    headers.set("Vary", "Accept");
    return new Response(resp.body, { status: resp.status, headers });
  }
  return new Response(null, { status: 406 });
}

export async function handleGateway(
  req: Request,
  outerEnv: Env,
  ctx: ExecutionContext,
): Promise<Response> {
  let url = new URL(req.url);

  // Bootstrap kubeconfig, plus 410 Gone for the retired management API
  // (clusters/api.ts). Never cluster-prefixed; doesn't collide with any
  // k8s API path.
  if (url.pathname === "/clusters" || url.pathname.startsWith("/clusters/")) {
    return handleClustersAPI(req, outerEnv);
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

  // EVERY cluster (default included, since the K3S_TOKEN secret was
  // abolished) authenticates AT THE DOOR against the cluster's own token
  // vault. This is what prevents cross-cluster token reuse: downstream
  // dwAuth-style checks compare against env.K3S_TOKEN, so the derived
  // env carries the verified presented token -- and an unverified
  // request never reaches them.
  let env: Env;
  {
    const presented = await verifyClusterToken(req, outerEnv, cluster.doName);
    // ServiceAccount JWTs are not vault tokens: they pass the door and
    // are authenticated by the Go apiserver's own SA-JWT authenticator
    // (signature/audience/expiry) with RBAC applied -- rejects still
    // 401 there. The derived env gets a random placeholder token in
    // that case, NOT the JWT, so the dwAuth-compared extras (kubelet
    // proxy, /nodes/*) never treat an SA identity as the cluster token.
    const auth = req.headers.get("Authorization") || "";
    const bearer = auth.startsWith("Bearer ") ? auth.slice(7) : "";
    const isJWT = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(bearer);
    if (!presented && !isJWT && !isUnauthenticatedPath(url)) {
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
    // The cluster operator's platform-operations bridge
    // (clusters/internalapi.ts). Restricted to the MANAGEMENT cluster:
    // these routes mint into arbitrary clusters' token vaults and tear
    // their DO trees down, so a tenant's own token -- which is a perfectly
    // valid cluster token, just not this cluster's -- must never reach
    // them. outerEnv, not env: the derived env's DO namespaces are
    // retargeted at one cluster's tree, and this handler addresses many.
    if (url.pathname.startsWith("/internal/clusters/")) {
      if (cluster.id !== "default") {
        return new Response("not found", { status: 404 });
      }
      return handleClustersInternalAPI(req, outerEnv);
    }
    return apiserverFetch(env, req);
  }
  // Answered before any apiserver dispatch: see isHealthPath.
  if (isHealthPath(url.pathname)) {
    return new Response("ok", {
      status: 200,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }

  // The wasm chunk supply channel (run_worker_first) is Loader-only.
  if (url.pathname.startsWith("/wasm/")) {
    return new Response("not found", { status: 404 });
  }

  // /openapi/v2 is the one asset needing content negotiation, which the
  // asset store cannot do on its own -- hence its run_worker_first entry.
  if (url.pathname.startsWith("/openapi/v2")) {
    return serveOpenAPIV2(req, env, url);
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
  // nodes -- the standard path metrics scrapers use. Port 10999 is
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
        target.pathname = `/kubelet/${nodeName}/10999${rest}`;
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
