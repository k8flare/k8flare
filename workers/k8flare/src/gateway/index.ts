import {
  isKubeletProxyRequest,
  handleKubeletProxy,
  handleRemotedialConnect,
} from "./proxy/index.ts";
import { dwAuth, handleWatch } from "@k8flare/k8s";
import { injectCustomAPIGroup, injectOpenAPIV3Path } from "@k8flare/crd";
import { DW_GROUP, DW_VERSION } from "@k8flare/dynamic-worker";
import type { Env } from "../env.ts";
import { apiserverFetch } from "../loader/apiserver.ts";
import { handleRuntime } from "../runtime/index.ts";
import { handleNodes } from "../nodes/index.ts";

// The consolidated Worker's public routing -- the former gateway Worker's
// fetch handler, with the cross-Worker service bindings replaced:
// APISERVER -> apiserverFetch (Loader dynamic worker), RUNTIME/NODES ->
// direct function calls, WATCHHUB -> the now-local DO binding.
export async function handleGateway(
  req: Request,
  env: Env,
  ctx: ExecutionContext,
): Promise<Response> {
  const url = new URL(req.url);

  // /internal/* was implicitly private pre-consolidation (reachable only
  // over service bindings to unrouted Workers; the Go handlers themselves
  // are unauthenticated). Now that everything shares the one public fetch
  // handler, the boundary is explicit: in-Worker callers (the Cluster
  // DO's node-lifecycle ping, nodes' mint-r2-credentials) go through
  // apiserverFetch directly and never enter this handler; from the
  // outside these routes exist only behind the cluster token (the same
  // trust level as every other API path -- pkg/apiserver's own test
  // suite drives /internal/mint-r2-credentials this way), and without it
  // they 404 rather than advertise themselves.
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
    return handleWatch(req, env, url, ctx);
  }

  // remotedialer tunnel endpoint — k3s agent connects here via WebSocket.
  // We use Workers VPC for kubelet communication, so this is a stub that
  // keeps the connection alive so the agent completes its bootstrap.
  if (url.pathname === "/v1-k3s/connect") {
    return handleRemotedialConnect(req, env);
  }

  // Custom API group: DynamicWorker + WorkerTrigger CRUD, discovery,
  // OpenAPI v3 document, and dispatch (the former runtime Worker).
  const dwGroupPrefix = `/apis/${DW_GROUP}/${DW_VERSION}/`;
  const dwOpenAPIPath = `/openapi/v3/apis/${DW_GROUP}/${DW_VERSION}`;
  if (
    url.pathname.startsWith(dwGroupPrefix) ||
    url.pathname === `/apis/${DW_GROUP}/${DW_VERSION}` ||
    url.pathname === `/apis/${DW_GROUP}/${DW_VERSION}/` ||
    url.pathname === `/apis/${DW_GROUP}` ||
    url.pathname === `/apis/${DW_GROUP}/` ||
    url.pathname === dwOpenAPIPath
  ) {
    return handleRuntime(req, env, ctx);
  }

  // HTTP trigger dispatch, also handled by runtime.
  if (url.pathname.startsWith("/trigger/")) {
    return handleRuntime(req, env, ctx);
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
  // retry absorber lives inside apiserverFetch). For /apis and
  // /openapi/v3, inject our custom group into the response (the Go
  // apiserver only knows about its own build-time-baked per-group-version
  // documents -- see pkg/apiserver/discovery.go).
  const apiResp = await apiserverFetch(env, req);
  if (url.pathname === "/apis" || url.pathname === "/apis/") {
    return injectCustomAPIGroup(apiResp, DW_GROUP, DW_VERSION);
  }
  if (url.pathname === "/openapi/v3") {
    return injectOpenAPIV3Path(
      apiResp,
      DW_GROUP,
      DW_VERSION,
      `${dwOpenAPIPath}?hash=k8flare-custom`,
    );
  }
  return apiResp;
}
