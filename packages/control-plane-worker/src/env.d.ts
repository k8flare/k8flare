type Entrypoints = typeof import("./index.ts");

declare module "*.wasm" {
  const module: WebAssembly.Module;
  export default module;
}

declare namespace Cloudflare {
  interface Env {
    PRINTERS: Fetcher<InstanceType<Entrypoints["Printers"]>>;
    OPENAPI: Fetcher<InstanceType<Entrypoints["OpenAPI"]>>;
    CUSTOMRESOURCES: Fetcher<InstanceType<Entrypoints["CustomResources"]>>;
    APIGROUPS: Fetcher<InstanceType<Entrypoints["APIGroups"]>>;
    SCHEDULER: Fetcher<InstanceType<Entrypoints["Scheduler"]>>;
    WORKLOADS: Fetcher<InstanceType<Entrypoints["Workloads"]>>;
    ATTACHDETACH: Fetcher<InstanceType<Entrypoints["AttachDetach"]>>;
    HPA: Fetcher<InstanceType<Entrypoints["HorizontalPodAutoscaler"]>>;
    GC: Fetcher<InstanceType<Entrypoints["GarbageCollector"]>>;
    STORAGE_SVC: Fetcher<InstanceType<Entrypoints["Storage"]>>;
    TUNNEL: Fetcher<InstanceType<Entrypoints["NodeTunnels"]>>;
    ADMISSION: Fetcher<InstanceType<Entrypoints["Admission"]>>;
    HOOKS: Fetcher<InstanceType<Entrypoints["Hooks"]>>;
    OUTBOUND: Fetcher<InstanceType<Entrypoints["Outbound"]>>;
    METRICS: Fetcher<InstanceType<Entrypoints["Metrics"]>>;
  }
}
