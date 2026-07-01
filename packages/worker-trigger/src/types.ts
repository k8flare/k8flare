import type { Condition } from "@k8flare/k8s";

/** Reference to a DynamicWorker resource. */
export interface WorkerRef {
  name: string;
  namespace?: string;
}

/** HTTP trigger configuration. */
export interface HTTPTriggerConfig {
  path: string;
  methods?: string[];
}

/** Cron trigger configuration. */
export interface CronTriggerConfig {
  schedule: string;
}

/** WorkerTrigger spec — defines which worker to invoke and the trigger type. */
export interface WorkerTriggerSpec {
  workerRef: WorkerRef;
  type: "http" | "cron";
  http?: HTTPTriggerConfig;
  cron?: CronTriggerConfig;
}

/** WorkerTrigger status — observed state of the trigger resource. */
export interface WorkerTriggerStatus {
  observedGeneration?: number;
  conditions?: Condition[];
}
