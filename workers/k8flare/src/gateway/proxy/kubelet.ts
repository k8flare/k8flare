import { dwAuth, dwError } from "../../k8s/index.ts";
import { handleExecAttach } from "./exec.ts";
import { resolveKubeletTarget } from "./target.ts";
import { handleNodes } from "../../nodes/index.ts";

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

  // Per-Pod Containers-backed node (workers/nodes microVM): the compute-
  // class admission stamps this nodeSelector, so it reliably marks pods
  // whose kubelet lives inside a NodeVM. Reach it through the nodes
  // handler's kubelet bridge (port 10999 = cmd/agent's plain-HTTP front
  // for the authenticated kubelet API) instead of the VPC binding, which
  // only dials BYO VMs. Post-consolidation (S19) this is a direct
  // function call, not a service binding -- there is no `env.NODES`
  // anymore (single-Worker `env.ts` never declared it; found stale here
  // while implementing task #13, which touches this same branch -- this
  // silently 503'd every logs/metrics request to a Containers-backed pod
  // since the consolidation, not something this pass could leave broken
  // while adding more code beside it).
  if (pod.spec?.nodeSelector?.["k8flare.com/backend"] === "containers") {
    if (!kubeletPath) {
      return dwError(501, `${subresource} is not yet supported on the per-Pod node backend`);
    }
    const target = new URL(url);
    target.pathname = `/kubelet/${pod.metadata.uid}/10999${kubeletPath.split("?")[0]}`;
    target.search = kubeletPath.includes("?") ? `?${kubeletPath.split("?")[1]}` : "";
    return handleNodes(
      new Request(target.toString(), {
        method: req.method,
        headers: { Authorization: `Bearer ${token}`, Accept: req.headers.get("Accept") || "*/*" },
      }),
      env,
    );
  }

  // Reach the node's kubelet via Mesh (preferred) or the legacy
  // Tunnel+VPC Service binding -- see target.ts.
  const kubeletTarget = await resolveKubeletTarget(env, goFetch, token, nodeName);
  if (!kubeletTarget) {
    return dwError(
      501,
      "kubelet proxy not available (neither MESH nor KUBELET_VPC binding configured)",
    );
  }
  try {
    // Strip auth headers -- kubelet uses its own auth, not the API token
    const proxyHeaders = new Headers();
    proxyHeaders.set("Accept", req.headers.get("Accept") || "*/*");
    return await kubeletTarget.fetcher.fetch(
      new Request(`http://${kubeletTarget.host}:10255${kubeletPath}`, {
        method: req.method,
        headers: proxyHeaders,
      }),
    );
  } catch (err: any) {
    return dwError(502, `failed to proxy to kubelet: ${err.message}`);
  }
}
