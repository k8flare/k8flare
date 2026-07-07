import { dwError } from "@k8flare/k8s";
import { resolveKubeletTarget } from "./target.ts";

/**
 * Handle kubectl exec/attach via WebSocket upgrade.
 *
 * Protocol overview (Kubernetes v5 WebSocket subprotocol):
 *   - kubectl sends Upgrade: websocket with Sec-WebSocket-Protocol: v4.channel.k8s.io
 *   - Each WebSocket message is prefixed with a single-byte channel identifier:
 *     0 = stdin, 1 = stdout, 2 = stderr, 3 = error, 255 = resize
 *   - The server upgrades and bridges bidirectional streams to the kubelet.
 *
 * Cloudflare Workers limitations:
 *   - Workers can accept a client WebSocket upgrade via WebSocketPair.
 *   - VPC Service binding fetch() can request a WebSocket upgrade to kubelet.
 *   - If VPC Service binding does not support WebSocket upgrade, the proxy
 *     returns 501 with a descriptive message.
 *
 * @param goFetch - function that sends a request through the Go WASM worker
 */
export async function handleExecAttach(
  req: Request,
  env: any,
  url: URL,
  namespace: string,
  podName: string,
  container: string,
  subresource: string,
  goFetch: (req: Request) => Promise<Response>,
): Promise<Response> {
  // Verify that the client is requesting a WebSocket upgrade.
  const upgradeHeader = req.headers.get("Upgrade") || "";
  if (upgradeHeader.toLowerCase() !== "websocket") {
    return dwError(
      400,
      `${subresource} requires a WebSocket upgrade (Upgrade: websocket header missing)`,
    );
  }

  // Build kubelet URL for exec/attach.
  // Kubelet endpoint: /exec/{namespace}/{pod}/{container}?command=...&stdin=...&stdout=...&stderr=...&tty=...
  const kubeletParams = new URLSearchParams();
  for (const [k, v] of url.searchParams) {
    if (k !== "container") kubeletParams.append(k, v);
  }
  const kubeletPath = `/${subresource}/${namespace}/${podName}/${container}?${kubeletParams.toString()}`;

  // Extract subprotocol requested by kubectl (e.g. "v4.channel.k8s.io").
  const clientSubprotocol = req.headers.get("Sec-WebSocket-Protocol") || "";

  // Get the node name from the pod to target the correct kubelet.
  const token: string = env.K3S_TOKEN || "k8flare-dev-token";
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

  // Reach the node's kubelet via Mesh (preferred) or the legacy
  // Tunnel+VPC Service binding -- see target.ts.
  const kubeletTarget = await resolveKubeletTarget(env, goFetch, token, nodeName);
  if (!kubeletTarget) {
    return dwError(
      501,
      "kubelet proxy not available (neither MESH nor KUBELET_VPC binding configured)",
    );
  }

  // Attempt WebSocket upgrade to kubelet via the resolved binding.
  let kubeletResp: Response;
  try {
    const kubeletWsHeaders = new Headers();
    kubeletWsHeaders.set("Upgrade", "websocket");
    if (clientSubprotocol) {
      kubeletWsHeaders.set("Sec-WebSocket-Protocol", clientSubprotocol);
    }

    kubeletResp = await kubeletTarget.fetcher.fetch(
      new Request(`http://${kubeletTarget.host}:10255${kubeletPath}`, {
        headers: kubeletWsHeaders,
      }),
    );
  } catch (err: any) {
    return dwError(502, `failed to connect to kubelet for ${subresource}: ${err.message}`);
  }

  const kubeletWs: WebSocket | null = kubeletResp.webSocket;
  if (!kubeletWs) {
    // Binding did not return a WebSocket -- not supported.
    return dwError(
      501,
      `${subresource} WebSocket proxy failed: kubelet did not upgrade to WebSocket (binding may not support WebSocket passthrough)`,
    );
  }

  kubeletWs.accept();

  // Create a WebSocketPair for the client (kubectl) side.
  const [clientWs, serverWs] = Object.values(new WebSocketPair());

  // Bridge: forward messages from kubectl -> kubelet
  serverWs.accept();
  serverWs.addEventListener("message", (event) => {
    try {
      kubeletWs.send(event.data);
    } catch {
      // kubelet side closed
    }
  });
  serverWs.addEventListener("close", (event) => {
    try {
      kubeletWs.close(event.code, event.reason);
    } catch {
      // ignore
    }
  });
  serverWs.addEventListener("error", () => {
    try {
      kubeletWs.close(1011, "client error");
    } catch {
      // ignore
    }
  });

  // Bridge: forward messages from kubelet -> kubectl
  kubeletWs.addEventListener("message", (event) => {
    try {
      serverWs.send(event.data);
    } catch {
      // client side closed
    }
  });
  kubeletWs.addEventListener("close", (event) => {
    try {
      serverWs.close(event.code, event.reason);
    } catch {
      // ignore
    }
  });
  kubeletWs.addEventListener("error", () => {
    try {
      serverWs.close(1011, "kubelet error");
    } catch {
      // ignore
    }
  });

  // Return the WebSocket upgrade response to the client.
  return new Response(null, { status: 101, webSocket: clientWs });
}
