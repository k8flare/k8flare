export function refuseUpgrade(): Response {
  return Response.json(
    {
      kind: "Status",
      apiVersion: "v1",
      metadata: {},
      status: "Failure",
      message: "only WebSocket upgrades are served; retry with a WebSocket client (v5.channel.k8s.io)",
      reason: "Invalid",
      code: 426,
    },
    { status: 426, headers: { Upgrade: "websocket", Connection: "Upgrade" } },
  );
}

export async function refuseUpgradeAfterAdmission(locate: () => Promise<Response>): Promise<Response> {
  const located = await locate();
  return located.ok ? refuseUpgrade() : located;
}
