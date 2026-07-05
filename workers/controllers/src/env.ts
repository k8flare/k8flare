export interface Env {
  CONTROLLERS: DurableObjectNamespace;
  CLUSTER: DurableObjectNamespace;
  GATEWAY: Fetcher;
  LOADER: WorkerLoader;
  ASSETS: Fetcher;
  K3S_TOKEN?: string;
}
