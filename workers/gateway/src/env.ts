export interface Env {
  WATCHHUB: DurableObjectNamespace;
  APISERVER: Fetcher;
  RUNTIME: Fetcher;
  KUBELET_VPC?: Fetcher;
  // Optional: workers/nodes (Pod-on-Containers backend), for the
  // /podproxy HTTP ingress route. Optional so clusters that never deploy
  // workers/nodes keep working (see index.ts's /podproxy branch).
  NODES?: Fetcher;
  K3S_TOKEN?: string;
}
