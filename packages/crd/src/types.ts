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

/** Configuration object for handleResourceCRUD. */
export interface CRDConfig {
  prefix: string;
  resource: string;
  kind: string;
  listKind: string;
  apiVersion: string;
  validate: (spec: any) => string | null;
  applyDefaults: (obj: any) => void;
  _subresource?: string;
}

/** Generic Custom Resource with metadata, spec, and status. */
export interface CustomResource {
  apiVersion?: string;
  kind?: string;
  metadata: ObjectMeta;
  spec: any;
  status: any;
}

/** Result of a single CR get operation. */
export interface CRGetResult {
  obj: any;
  modRevision: number;
}

/** Parsed path components from a custom API group URL. */
export interface ParsedPath {
  namespace: string;
  resource: string;
  name: string;
  subresource: string;
}

/** API resource descriptor used in discovery responses. */
export interface APIResourceDescriptor {
  name: string;
  singularName: string;
  namespaced: boolean;
  kind: string;
  verbs: string[];
}
