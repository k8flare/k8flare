import {
  isKubeletProxyRequest,
  handleKubeletProxy,
  handleRemotedialConnect,
} from "./proxy/index.ts";
import { handleWatch } from "@k8flare/k8s";
import { injectCustomAPIGroup } from "@k8flare/crd";
import { DW_GROUP, DW_VERSION } from "@k8flare/dynamic-worker";
import type { Env } from "./env.ts";

export default {
  async fetch(req: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(req.url);

    // Kubelet proxy requests (pods/log, pods/exec, etc.)
    if (isKubeletProxyRequest(url.pathname)) {
      return handleKubeletProxy(req, env, url, (r) => env.APISERVER.fetch(r));
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

    // Custom API group: DynamicWorker + WorkerTrigger CRUD, discovery, and
    // dispatch all live in the runtime Worker.
    const dwGroupPrefix = `/apis/${DW_GROUP}/${DW_VERSION}/`;
    if (
      url.pathname.startsWith(dwGroupPrefix) ||
      url.pathname === `/apis/${DW_GROUP}/${DW_VERSION}` ||
      url.pathname === `/apis/${DW_GROUP}/${DW_VERSION}/` ||
      url.pathname === `/apis/${DW_GROUP}` ||
      url.pathname === `/apis/${DW_GROUP}/`
    ) {
      return env.RUNTIME.fetch(req);
    }

    // HTTP trigger dispatch, also handled by runtime.
    if (url.pathname.startsWith("/trigger/")) {
      return env.RUNTIME.fetch(req);
    }

    // All other requests go to the Go apiserver Worker.
    // For /apis, inject our custom group into the response.
    const apiResp = await env.APISERVER.fetch(req);
    if (url.pathname === "/apis" || url.pathname === "/apis/") {
      return injectCustomAPIGroup(apiResp, DW_GROUP, DW_VERSION);
    }
    return apiResp;
  },
};
