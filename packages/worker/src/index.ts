import goWorker from "#go-worker";
export { Etcd } from "@k8flare/etcd";

import { isKubeletProxyRequest, handleKubeletProxy, handleRemotedialConnect } from "@k8flare/proxy";
import { handleWatch, dwAuth } from "@k8flare/k8s";
import {
  parseCustomGroupPath,
  buildDiscoveryResources,
  buildDiscoveryGroup,
  injectCustomAPIGroup,
  handleResourceCRUD,
} from "@k8flare/crd";
import {
  DW_GROUP,
  DW_VERSION,
  DW_RESOURCE,
  DW_KIND,
  DW_API_VERSION,
  handleDynamicWorkerCRUD,
  handleDynamicWorkerRun,
} from "@k8flare/dynamic-worker";
import {
  WT_RESOURCE,
  WT_KIND,
  handleWorkerTriggerCRUD,
  handleHTTPTrigger,
} from "@k8flare/worker-trigger";
import type { Env } from "./env.ts";

/**
 * Handle custom API group requests (DynamicWorker + WorkerTrigger).
 *
 * Authenticates via dwAuth and dispatches to the appropriate resource handler
 * based on the parsed path.
 */
async function handleCustomGroupAPI(
  req: Request,
  env: Env,
  ctx: ExecutionContext,
  path: string,
): Promise<Response> {
  if (!dwAuth(req, env)) {
    return Response.json(
      { kind: "Status", apiVersion: "v1", status: "Failure", message: "Unauthorized", code: 401 },
      { status: 401 },
    );
  }

  const parsed = parseCustomGroupPath(path);
  if (!parsed) {
    return Response.json(
      {
        kind: "Status",
        apiVersion: "v1",
        status: "Failure",
        message: `invalid path "${path}"`,
        code: 404,
      },
      { status: 404 },
    );
  }

  const { namespace, resource, name, subresource } = parsed;

  if (resource === DW_RESOURCE) {
    return handleDynamicWorkerCRUD(req, env, ctx, namespace, name, subresource, handleResourceCRUD);
  }
  if (resource === WT_RESOURCE) {
    return handleWorkerTriggerCRUD(req, env, ctx, namespace, name, subresource);
  }

  return Response.json(
    {
      kind: "Status",
      apiVersion: "v1",
      status: "Failure",
      message: `unknown resource "${resource}"`,
      code: 404,
    },
    { status: 404 },
  );
}

/**
 * Build the APIResourceList discovery response for the custom API group.
 *
 * Lists DynamicWorker (with run and status subresources) and WorkerTrigger
 * (with status subresource).
 */
function dwDiscoveryResources(): Response {
  return buildDiscoveryResources(DW_GROUP, DW_VERSION, [
    {
      name: DW_RESOURCE,
      singularName: "dynamicworker",
      namespaced: true,
      kind: DW_KIND,
      verbs: ["create", "delete", "get", "list", "update"],
    },
    {
      name: `${DW_RESOURCE}/run`,
      singularName: "",
      namespaced: true,
      kind: DW_KIND,
      verbs: ["create"],
    },
    {
      name: `${DW_RESOURCE}/status`,
      singularName: "",
      namespaced: true,
      kind: DW_KIND,
      verbs: ["get", "update"],
    },
    {
      name: WT_RESOURCE,
      singularName: "workertrigger",
      namespaced: true,
      kind: WT_KIND,
      verbs: ["create", "delete", "get", "list", "update"],
    },
    {
      name: `${WT_RESOURCE}/status`,
      singularName: "",
      namespaced: true,
      kind: WT_KIND,
      verbs: ["get", "update"],
    },
  ]);
}

/**
 * Build the APIGroup discovery response for the custom API group.
 */
function dwDiscoveryGroup(): Response {
  return buildDiscoveryGroup(DW_GROUP, DW_VERSION);
}

export default {
  async fetch(req: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(req.url);

    // Kubelet proxy requests (pods/log, pods/exec, etc.)
    if (isKubeletProxyRequest(url.pathname)) {
      return handleKubeletProxy(req, env, url, (r) => goWorker.fetch(r, env, ctx));
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

    // Custom API group: DynamicWorker + WorkerTrigger
    const dwGroupPrefix = `/apis/${DW_GROUP}/${DW_VERSION}/`;
    if (url.pathname.startsWith(dwGroupPrefix)) {
      return handleCustomGroupAPI(req, env, ctx, url.pathname.slice(dwGroupPrefix.length));
    }

    // API discovery for the custom group
    if (
      url.pathname === `/apis/${DW_GROUP}/${DW_VERSION}` ||
      url.pathname === `/apis/${DW_GROUP}/${DW_VERSION}/`
    ) {
      return dwDiscoveryResources();
    }
    if (url.pathname === `/apis/${DW_GROUP}` || url.pathname === `/apis/${DW_GROUP}/`) {
      return dwDiscoveryGroup();
    }

    // HTTP trigger dispatch
    if (url.pathname.startsWith("/trigger/")) {
      return handleHTTPTrigger(req, env, ctx, url, handleDynamicWorkerRun);
    }

    // All other requests go to Go WASM handler.
    // For /apis, inject our custom group into the response.
    const goResp = await goWorker.fetch(req, env, ctx);
    if (url.pathname === "/apis" || url.pathname === "/apis/") {
      return injectCustomAPIGroup(goResp, DW_GROUP, DW_VERSION);
    }
    return goResp;
  },
};
