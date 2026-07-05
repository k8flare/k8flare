import type { NodeVMLarge, NodeVMMedium, NodeVMSmall } from "./nodevm.ts";
import type { CFContainersScheduler } from "./scheduler.ts";

export interface Env {
  SCHEDULER: DurableObjectNamespace<CFContainersScheduler>;
  NODE_VM_SMALL: DurableObjectNamespace<NodeVMSmall>;
  NODE_VM_MEDIUM: DurableObjectNamespace<NodeVMMedium>;
  NODE_VM_LARGE: DurableObjectNamespace<NodeVMLarge>;
  APISERVER: Fetcher;
  K3S_TOKEN?: string;
  // Public gateway URL the in-VM k3s agent joins through (the microVM
  // dials out over the internet; service bindings don't reach into it).
  GATEWAY_URL?: string;
}
