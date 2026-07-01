export interface Env {
  ETCD: DurableObjectNamespace;
  LOADER: any;
  KUBELET_VPC?: Fetcher;
  K3S_TOKEN?: string;
}
