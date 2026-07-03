import type { PodContainerLarge, PodContainerMedium, PodContainerSmall } from "./podcontainer.ts";
import type { VirtualNode } from "./virtualnode.ts";

export interface Env {
  VIRTUAL_NODE: DurableObjectNamespace<VirtualNode>;
  POD_CONTAINER_SMALL: DurableObjectNamespace<PodContainerSmall>;
  POD_CONTAINER_MEDIUM: DurableObjectNamespace<PodContainerMedium>;
  POD_CONTAINER_LARGE: DurableObjectNamespace<PodContainerLarge>;
  APISERVER: Fetcher;
  K3S_TOKEN?: string;
  // Name suffix for the virtual Node this Worker registers, so more than one
  // pool can be deployed side by side (e.g. distinct size/region policies) --
  // the registered Node is named `cf-containers-${NODE_POOL}`. Defaults to
  // "default" (see virtualnode.ts's DEFAULT_POOL).
  NODE_POOL?: string;
}
