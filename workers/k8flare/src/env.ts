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
  // Multi-cluster metadata (clusters/registry.ts): {id -> uid/state} and
  // the cluster list ONLY -- no alarms/WS, idle cost is storage alone.
  REGISTRY: DurableObjectNamespace;
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
  // Legacy: Cloudflare Tunnel + VPC Service (scripts/setup-tunnel.sh),
  // being phased out in favor of MESH below (user decision 2026-07-07)
  // -- kept as a fallback for existing deployments, not removed.
  KUBELET_VPC?: Fetcher;
  // Cloudflare Mesh (spikes/s17-mesh-nodevm/FINDINGS.md): a
  // vpc_networks binding to the account-wide Mesh ("cf1:network").
  // Reaches a BYO VM node directly at its Mesh IP -- no per-node Tunnel
  // config, no VPC Service resource. Preferred over KUBELET_VPC when
  // both are present (gateway/proxy/kubelet.ts).
  MESH?: Fetcher;

  K3S_TOKEN?: string;
  // Management-API auth (clusters/adminauth.ts): comma-separated
  // rotatable admin secrets, and/or Cloudflare Access JWT verification
  // (team domain + application AUD). Neither set = dev fallback token,
  // same posture as K3S_TOKEN's dev fallback.
  ADMIN_TOKENS?: string;
  ACCESS_TEAM_DOMAIN?: string;
  ACCESS_AUD?: string;
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
  // Multi-cluster: set ONLY on derived envs (clusters/clusterenv.ts, DO
  // apiEnv helpers), never in wrangler.jsonc -- names the Cluster DO
  // instance downstream storage traffic targets. loader/apiserver.ts
  // stamps it as the X-K8flare-Cluster header and keys the per-cluster
  // dynamic-worker isolate with it. Absent = "default".
  CLUSTER_DO_NAME?: string;
  // Multi-cluster: the public URL path prefix ("/c/<id>", "" for
  // default) the supervisor advertises to joining agents.
  CLUSTER_BASE_PATH?: string;
  // Multi-cluster: the PRISTINE wrangler-level K3S_TOKEN, preserved by
  // clusters/clusterenv.ts when it overwrites K3S_TOKEN with the
  // caller's presented token -- loader/apiserver.ts must bake the env
  // token (a stable fallback), never whichever presented token happened
  // to arrive first on a cold isolate.
  ENV_K3S_TOKEN?: string;
}
