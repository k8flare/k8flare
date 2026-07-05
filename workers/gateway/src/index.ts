import {
  isKubeletProxyRequest,
  handleKubeletProxy,
  handleRemotedialConnect,
} from "./proxy/index.ts";
import { dwAuth, handleWatch } from "@k8flare/k8s";
import { injectCustomAPIGroup, injectOpenAPIV3Path } from "@k8flare/crd";
import { DW_GROUP, DW_VERSION } from "@k8flare/dynamic-worker";
import type { Env } from "./env.ts";

export default {
  async fetch(req: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(req.url);

    // Kubelet proxy requests (pods/log, pods/exec, etc.)
    if (isKubeletProxyRequest(url.pathname)) {
      return handleKubeletProxy(req, env, url, (r) => env.APISERVER.fetch(r));
    }

    // Standard Kubernetes pods/proxy subresource, backed by the
    // Pod-on-Containers backend: GET/... /api/v1/namespaces/{ns}/pods/
    // {pod}/proxy/{path} -> workers/nodes -> VirtualNode DO -> the Pod's
    // backing Container (port 8080). Using the upstream API surface (what
    // `kubectl proxy` / `kubectl get --raw .../proxy/...` speak) instead
    // of an invented path keeps the route inside the API's URL namespace
    // and inherits the pods/proxy verb semantics the moment a real RBAC
    // authorizer lands. Token-gated like every other API path; the nodes
    // Worker re-checks the same Authorization header on its side too.
    const podProxyMatch = url.pathname.match(
      /^\/api\/v1\/namespaces\/([^/]+)\/pods\/([^/]+)\/proxy(\/.*)?$/,
    );
    if (podProxyMatch) {
      if (!dwAuth(req, env)) {
        return new Response("unauthorized", { status: 401 });
      }
      if (!env.NODES) {
        return new Response("pods/proxy: workers/nodes is not deployed on this cluster", {
          status: 503,
        });
      }
      const [, ns, pod, rest] = podProxyMatch;
      const target = new URL(req.url);
      target.pathname = `/podproxy/${ns}/${pod}${rest ?? "/"}`;
      return env.NODES.fetch(new Request(target.toString(), req));
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
    // OpenAPI v3 document, and dispatch all live in the runtime Worker.
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
      return env.RUNTIME.fetch(req);
    }

    // HTTP trigger dispatch, also handled by runtime.
    if (url.pathname.startsWith("/trigger/")) {
      return env.RUNTIME.fetch(req);
    }

    // All other requests go to the Go apiserver Worker.
    // For /apis and /openapi/v3, inject our custom group into the response
    // (the Go apiserver only knows about its own build-time-baked
    // per-group-version documents -- see pkg/apiserver/discovery.go).
    //
    // Cold-start absorber (read-only requests only): instantiating the
    // 43MB apiserver WASM under a parallel burst (kubectl's discovery
    // fans out ~30 concurrent group requests, each of which can land on
    // its own cold isolate) intermittently blows the isolate's startup
    // CPU budget -- the binding fetch then throws, which kubectl surfaces
    // as `couldn't get resource list ... ("unknown")`. Retrying after a
    // short pause lands on a now-warm isolate and succeeds (verified
    // live: the same URL 200s immediately after a failure). Retries are
    // strictly limited to GET/HEAD so no mutation is ever replayed.
    const attempts = req.method === "GET" || req.method === "HEAD" ? 3 : 1;
    let apiResp: Response | undefined;
    let lastErr: unknown;
    for (let i = 0; i < attempts; i++) {
      if (i > 0) await new Promise((r) => setTimeout(r, 250 * 2 ** (i - 1)));
      try {
        apiResp = await env.APISERVER.fetch(req.clone() as Request);
        if (apiResp.status < 500 || i === attempts - 1) break;
      } catch (err) {
        lastErr = err;
        apiResp = undefined;
      }
    }
    if (!apiResp) {
      throw lastErr instanceof Error ? lastErr : new Error(String(lastErr));
    }
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
  },
};
