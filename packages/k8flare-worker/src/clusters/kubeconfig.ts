// kubeconfig for a cluster: server URL carries the /c/<id> path prefix
// (client-go/kubectl fully support path-prefixed servers -- the Rancher
// /k8s/clusters/<id> proxy precedent). TLS is Cloudflare's public edge
// cert, so no CA data is embedded -- same shape as
// cmd/controller-manager's writeKubeconfig.
export function buildKubeconfig(origin: string, clusterId: string, token: string): string {
  const server = clusterId === "default" ? origin : `${origin}/c/${clusterId}`;
  const name = `k8flare-${clusterId}`;
  return `apiVersion: v1
kind: Config
clusters:
  - name: ${name}
    cluster:
      server: ${server}
users:
  - name: ${name}
    user:
      token: ${token}
contexts:
  - name: ${name}
    context:
      cluster: ${name}
      user: ${name}
current-context: ${name}
`;
}
