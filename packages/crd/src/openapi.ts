/** One Kind this custom group's OpenAPI v3 document should describe. */
export interface OpenAPIKindSpec {
  kind: string;
  /** Plural resource name, e.g. "dynamicworkers" (used in the `paths` keys). */
  resource: string;
  /** Schema name, e.g. "com.k8flare.v1alpha1.DynamicWorker". */
  schemaName: string;
}

const FIELD_VALIDATION_PARAM = {
  description: "fieldValidation instructs the server on how to handle unknown/duplicate fields.",
  in: "query",
  name: "fieldValidation",
  schema: { type: "string", uniqueItems: true },
};

/**
 * Build a minimal, permissive OpenAPI v3 document for a custom API group.
 *
 * Real per-group-version documents (workers/apiserver/assets/openapi/v3/)
 * carry a full `paths` map plus exact field schemas generated from Go
 * types. This repo's custom resources (DynamicWorker/WorkerTrigger) have
 * no such generated Go schema, so this instead emits just enough for two
 * separate kubectl checks to pass without `--validate=false`:
 *
 * 1. components.schemas gets one entry per Kind (+ its List), tagged with
 *    the `x-kubernetes-group-version-kind` extension kubectl's openapi3
 *    client scans for (see k8s.io/kubectl/pkg/cmd/util/openapi), each
 *    permissive via `x-kubernetes-preserve-unknown-fields` so arbitrary
 *    spec/status shapes are accepted rather than pruned or rejected.
 * 2. `paths` gets a namespaced *individual-resource* entry (PATCH) per
 *    Kind with a `fieldValidation` query parameter and a requestBody
 *    $ref to that Kind's schema. This is NOT for kubectl's schema/pruning
 *    logic (that only reads components.schemas) -- it's because
 *    k8s.io/cli-runtime's queryParamVerifierV3.HasSupport (see
 *    query_param_verifier_v3.go) specifically scans `doc.Paths` for a
 *    **PATCH** operation whose own `x-kubernetes-group-version-kind`
 *    extension (a single object here, unlike the *array* used on
 *    component schemas above -- the real per-group-version documents use
 *    both shapes, one per spot) matches the GVK, then checks that PATCH
 *    operation's parameters for `fieldValidation`. Without a matching
 *    PATCH entry it logs "Path not found for GVK ... falling back to
 *    legacy" and falls through to the /openapi/v2 (protobuf) endpoint,
 *    which errors for this repo independent of anything CRD-related. An
 *    empty `paths: {}`, or a path with only a POST operation (this file's
 *    first two versions), satisfies the schema-lookup check but not this
 *    one.
 */
export function buildGroupOpenAPIDocument(
  group: string,
  version: string,
  kinds: OpenAPIKindSpec[],
): object {
  const schemas: Record<string, object> = {};
  const paths: Record<string, object> = {};

  const permissiveObjectSchema = (kind: string) => ({
    description: `${kind} (permissive schema; see packages/crd/src/openapi.ts)`,
    type: "object",
    properties: {
      apiVersion: { type: "string" },
      kind: { type: "string" },
      metadata: { type: "object", "x-kubernetes-preserve-unknown-fields": true },
      spec: { type: "object", "x-kubernetes-preserve-unknown-fields": true },
      status: { type: "object", "x-kubernetes-preserve-unknown-fields": true },
    },
    "x-kubernetes-preserve-unknown-fields": true,
    "x-kubernetes-group-version-kind": [{ group, version, kind }],
  });

  for (const { kind, resource, schemaName } of kinds) {
    schemas[schemaName] = permissiveObjectSchema(kind);
    schemas[`${schemaName}List`] = {
      description: `${kind}List (permissive schema; see packages/crd/src/openapi.ts)`,
      type: "object",
      properties: {
        apiVersion: { type: "string" },
        kind: { type: "string" },
        metadata: { type: "object", "x-kubernetes-preserve-unknown-fields": true },
        items: { type: "array", items: { $ref: `#/components/schemas/${schemaName}` } },
      },
      "x-kubernetes-group-version-kind": [{ group, version, kind: `${kind}List` }],
    };

    const schemaRef = { $ref: `#/components/schemas/${schemaName}` };
    paths[`/apis/${group}/${version}/namespaces/{namespace}/${resource}/{name}`] = {
      patch: {
        description: `partially update the specified ${kind}`,
        operationId: `patch${kind}`,
        parameters: [FIELD_VALIDATION_PARAM],
        requestBody: {
          content: {
            "application/json-patch+json": { schema: schemaRef },
            "application/merge-patch+json": { schema: schemaRef },
          },
          required: true,
        },
        responses: {
          "200": { content: { "application/json": { schema: schemaRef } }, description: "OK" },
        },
        "x-kubernetes-group-version-kind": { group, version, kind },
      },
    };
  }

  return {
    openapi: "3.0.0",
    info: { title: `${group}/${version}`, version: "v1" },
    paths,
    components: { schemas },
  };
}
