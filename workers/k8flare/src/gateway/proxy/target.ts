// Resolves which binding (Mesh, preferred; Tunnel+VPC Service, legacy
// fallback) and host:port to reach a BYO VM node's kubelet through, per
// env.ts's MESH/KUBELET_VPC doc comments. Mesh needs the Node's
// ExternalIP (whatever cmd/agent's --mesh-connector-token flow
// discovered, pkg/meshconnector); the Tunnel path addresses by node
// name, unchanged from before Mesh existed.

export interface KubeletTarget {
  fetcher: Fetcher;
  host: string;
}

export async function resolveKubeletTarget(
  env: any,
  goFetch: (req: Request) => Promise<Response>,
  token: string,
  nodeName: string,
): Promise<KubeletTarget | null> {
  if (env.MESH) {
    const nodeResp = await goFetch(
      new Request(`http://internal/api/v1/nodes/${nodeName}`, {
        headers: { Authorization: `Bearer ${token}` },
      }),
    );
    if (nodeResp.ok) {
      const node: any = await nodeResp.json();
      const externalIP = node.status?.addresses?.find((a: any) => a.type === "ExternalIP")?.address;
      if (externalIP) {
        return { fetcher: env.MESH, host: externalIP };
      }
      // No ExternalIP on this Node -- not Mesh-joined, fall through.
    }
  }
  if (env.KUBELET_VPC) {
    return { fetcher: env.KUBELET_VPC, host: nodeName };
  }
  return null;
}
