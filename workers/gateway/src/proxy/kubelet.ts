import { dwAuth, dwError } from "@k8flare/k8s";
import { handleExecAttach } from "./exec.ts";

/**
 * Detect if a request needs kubelet proxying.
 * Patterns: /api/v1/namespaces/{ns}/pods/{name}/log|exec|attach|portforward
 */
export function isKubeletProxyRequest(pathname: string): boolean {
  return /\/api\/v1\/namespaces\/[^/]+\/pods\/[^/]+\/(log|exec|attach|portforward)/.test(pathname);
}

/**
 * Proxy a request to kubelet via VPC Service binding.
 *
 * @param goFetch - function that sends a request through the Go WASM worker
 */
export async function handleKubeletProxy(
  req: Request,
  env: any,
  url: URL,
  goFetch: (req: Request) => Promise<Response>,
): Promise<Response> {
  // Auth check
  if (!dwAuth(req, env)) {
    return new Response("Unauthorized", { status: 401 });
  }

  const token: string = env.K3S_TOKEN || "k8flare-dev-token";

  // Parse path: /api/v1/namespaces/{ns}/pods/{name}/{subresource}
  const match = url.pathname.match(
    /\/api\/v1\/namespaces\/([^/]+)\/pods\/([^/]+)\/(log|exec|attach|portforward)/,
  );
  if (!match) {
    return new Response("Bad Request", { status: 400 });
  }
  const [, namespace, podName, subresource] = match;

  // Get pod to find node name -- call Go WASM to get pod info
  const podResp = await goFetch(
    new Request(`http://internal/api/v1/namespaces/${namespace}/pods/${podName}`, {
      headers: { Authorization: `Bearer ${token}` },
    }),
  );

  if (!podResp.ok) {
    return podResp;
  }

  const pod: any = await podResp.json();
  const nodeName: string | undefined = pod.spec?.nodeName;
  if (!nodeName) {
    return dwError(400, `pod ${podName} is not assigned to a node`);
  }

  // Get container name (first container or from query param)
  const container: string =
    url.searchParams.get("container") || pod.spec.containers?.[0]?.name || "";

  // Build kubelet URL
  let kubeletPath: string | undefined;
  switch (subresource) {
    case "log": {
      kubeletPath = `/containerLogs/${namespace}/${podName}/${container}`;
      // Pass through query params like follow, tailLines, etc.
      const params = new URLSearchParams();
      for (const [k, v] of url.searchParams) {
        if (k !== "container") params.set(k, v);
      }
      if (params.toString()) kubeletPath += `?${params.toString()}`;
      break;
    }
    case "exec":
    case "attach":
      return handleExecAttach(req, env, url, namespace, podName, container, subresource, goFetch);
    case "portforward":
      // portforward requires a distinct multiplexing protocol not yet implemented.
      return dwError(501, `${subresource} is not yet supported`);
  }

  // Proxy via VPC Service binding if available
  if (env.KUBELET_VPC) {
    try {
      // Strip auth headers -- kubelet uses its own auth, not the API token
      const proxyHeaders = new Headers();
      proxyHeaders.set("Accept", req.headers.get("Accept") || "*/*");
      const kubeletResp: Response = await env.KUBELET_VPC.fetch(
        new Request(`http://${nodeName}:10255${kubeletPath}`, {
          method: req.method,
          headers: proxyHeaders,
        }),
      );
      return kubeletResp;
    } catch (err: any) {
      return dwError(502, `failed to proxy to kubelet: ${err.message}`);
    }
  }

  // Fallback: VPC not configured
  return dwError(501, "kubelet proxy not available (KUBELET_VPC binding not configured)");
}
