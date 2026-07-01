/** Kubernetes ObjectMeta — core metadata for every K8s object. */
export interface ObjectMeta {
  name?: string;
  namespace?: string;
  uid?: string;
  creationTimestamp?: string;
  generation?: number;
  resourceVersion?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

/** Kubernetes ListMeta — metadata for list responses. */
export interface ListMeta {
  resourceVersion?: string;
  continue?: string;
}

/** Kubernetes TypeMeta — identifies the type of an API object. */
export interface TypeMeta {
  apiVersion?: string;
  kind?: string;
}

/** Kubernetes Status — returned for errors and operation results. */
export interface Status {
  kind: string;
  apiVersion: string;
  metadata: ListMeta;
  status: string;
  message: string;
  reason?: string;
  code: number;
}

/** Kubernetes WatchEvent — streamed during watch operations. */
export interface WatchEvent {
  type: "ADDED" | "MODIFIED" | "DELETED";
  object: Record<string, unknown>;
}

/** Kubernetes Condition — status condition on a resource. */
export interface Condition {
  type: string;
  status: string;
  reason: string;
  message: string;
  lastTransitionTime: string;
}

/** Kine key-value entry. */
export interface KineKV {
  key: string;
  modRevision: number;
  createRevision: number;
  value: string;
  lease?: number;
}

/** Kine event — emitted by the KineStore DO over WebSocket. */
export interface KineEvent {
  kv: KineKV;
  create?: boolean;
  delete?: boolean;
  prevKV?: KineKV;
}
