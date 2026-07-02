export interface Env {
  CLUSTER: DurableObjectNamespace;
  APISERVER: Fetcher;
  RUNTIME: Fetcher;
  KUBELET_VPC?: Fetcher;
  K3S_TOKEN?: string;
}
