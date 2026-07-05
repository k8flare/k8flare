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

    // HTTP ingress into a Pod-on-Containers Pod:
    // /podproxy/{namespace}/{pod}/{path...} -> workers/nodes ->
    // VirtualNode DO -> the Pod's backing Container (port 8080).
    // Token-gated here (same dwAuth as watch/kubelet-proxy); the nodes
    // Worker re-checks the same Authorization header on its side too.
    if (url.pathname.startsWith("/podproxy/")) {
      if (!dwAuth(req, env)) {
        return new Response("unauthorized", { status: 401 });
      }
      if (!env.NODES) {
        return new Response("podproxy: workers/nodes is not deployed on this cluster", {
          status: 503,
        });
      }
      return env.NODES.fetch(req);
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
    const apiResp = await env.APISERVER.fetch(req);
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
