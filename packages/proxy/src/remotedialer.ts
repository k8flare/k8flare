/**
 * WebSocket stub for k3s agent remotedialer.
 *
 * The k3s agent expects this WebSocket endpoint to exist. In a standard k3s
 * deployment, the server uses this tunnel to route traffic back to the kubelet.
 * Since we use Workers VPC for kubelet communication, this stub simply accepts
 * the WebSocket upgrade and keeps the connection alive so the agent's bootstrap
 * process can complete.
 *
 * No auth check here -- the k3s remotedialer authenticates via mTLS client
 * certificate, which Cloudflare strips during TLS termination. Since this is
 * a stub endpoint (we use Workers VPC, not remotedialer, for kubelet
 * communication), skipping auth is safe. The agent already authenticated
 * with the cluster token during bootstrap (/v1-k3s/config, cert signing).
 */
export async function handleRemotedialConnect(req: Request, _env: any): Promise<Response> {
  // Must be a WebSocket upgrade request
  const upgradeHeader = (req.headers.get("Upgrade") || "").toLowerCase();
  if (upgradeHeader !== "websocket") {
    return new Response("WebSocket upgrade required", { status: 426 });
  }

  const [clientWs, serverWs] = Object.values(new WebSocketPair());
  serverWs.accept();

  const nodeName = req.headers.get("X-Cattle-NodeId") || "unknown";
  console.log(`[remotedialer] Agent connected: ${nodeName}`);

  // Keep-alive: respond to any message from the agent (remotedialer heartbeats)
  serverWs.addEventListener("message", (_event) => {
    // remotedialer sends periodic heartbeat frames; acknowledge silently
  });

  serverWs.addEventListener("close", (event) => {
    console.log(`[remotedialer] Agent disconnected: ${nodeName} (code=${event.code})`);
  });

  serverWs.addEventListener("error", () => {
    console.log(`[remotedialer] Agent error: ${nodeName}`);
  });

  return new Response(null, { status: 101, webSocket: clientWs });
}
