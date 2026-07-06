import type { NodeVMLarge, NodeVMMedium, NodeVMSmall } from "./nodes/nodevm.ts";
import type { CFContainersScheduler } from "./nodes/scheduler.ts";

// The single Worker's merged environment (union of the former six
// Workers' envs). Every DO class is local now -- the script_name
// indirections and cross-Worker service bindings are gone.
export interface Env {
  // Durable Objects (all exported from src/index.ts)
  CLUSTER: DurableObjectNamespace;
  WATCHHUB: DurableObjectNamespace;
  CONTROLLERS: DurableObjectNamespace;
  SCHEDULER: DurableObjectNamespace<CFContainersScheduler>;
  NODE_VM_SMALL: DurableObjectNamespace<NodeVMSmall>;
  NODE_VM_MEDIUM: DurableObjectNamespace<NodeVMMedium>;
  NODE_VM_LARGE: DurableObjectNamespace<NodeVMLarge>;

  // Self service bindings (S19 G2): SELF = the public fetch handler
  // (the KCM dynamic worker's "GATEWAY"); STORAGE = the ClusterLoopback
  // named entrypoint (the apiserver dynamic worker's route to the
  // Cluster DO -- DO namespaces cannot cross the Loader env clone).
  SELF: Fetcher;
  STORAGE: Fetcher;

  ASSETS: Fetcher;
  LOADER: WorkerLoader;

  // Optional Workers VPC binding for BYO-node kubelet access
  // (logs/exec); attached per deployment, absent in dev.
  KUBELET_VPC?: Fetcher;

  K3S_TOKEN?: string;
  // Public URL in-VM k3s agents join through (microVMs dial out over
  // the internet; bindings don't reach them).
  GATEWAY_URL?: string;
  // R2 PV/PVC backend configuration (see pkg/apiserver/r2.go); passed
  // through to the apiserver dynamic worker's env as plain values.
  R2_ACCOUNT_ID?: string;
  R2_ACCESS_KEY_ID?: string;
  R2_SECRET_ACCESS_KEY?: string;
  R2_BUCKET?: string;
  // Test kill switch: "1" disables KCM pokes/loads so pkg/apiserver's
  // go test suite (whose Pods must not be touched by controllers) can
  // run against the consolidated single config. See CLAUDE.md's
  // local-dev pitfalls.
  KCM_DISABLED?: string;
}
