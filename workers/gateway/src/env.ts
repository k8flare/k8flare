export interface Env {
  WATCHHUB: DurableObjectNamespace;
  APISERVER: Fetcher;
  RUNTIME: Fetcher;
  KUBELET_VPC?: Fetcher;
  K3S_TOKEN?: string;
}
