export interface Env {
  CONTROLLERS: DurableObjectNamespace;
  CLUSTER: DurableObjectNamespace;
  GATEWAY: Fetcher;
  K3S_TOKEN?: string;
}
