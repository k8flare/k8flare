/** Kubernetes Condition — status condition on a resource. */
export interface Condition {
  type: string;
  status: string;
  reason: string;
  message: string;
  lastTransitionTime: string;
}

/** Reference to a Secret or ConfigMap for environment variable injection. */
export interface EnvFromRef {
  secretRef?: { name: string; prefix?: string };
  configMapRef?: { name: string; prefix?: string };
  prefix?: string;
  optional?: boolean;
}

/** DynamicWorker spec — defines the worker code, modules, and runtime config. */
export interface DynamicWorkerSpec {
  modules: Record<string, string | Record<string, string>>;
  mainModule: string;
  compatibilityDate: string;
  compatibilityFlags?: string[];
  networkAccess: "none" | "inherit";
  env?: Record<string, string>;
  envFrom?: EnvFromRef[];
  entrypoint?: string;
}

/** DynamicWorker status — observed state of the worker resource. */
export interface DynamicWorkerStatus {
  observedGeneration?: number;
  lastRunTime?: string;
  conditions?: Condition[];
}
